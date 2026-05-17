package filesystem

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/huwenlong92/sdkit/pkg/filesystem/chunk"
	"github.com/huwenlong92/sdkit/pkg/filesystem/core"

	"github.com/google/uuid"
)

const (
	HookBeforeDownload      = "BeforeDownload"
	HookAfterDownload       = "AfterDownload"
	HookAfterDownloadFailed = "AfterDownloadFailed"
)

type UploadInitRequest struct {
	FileName  string
	Path      string
	TotalSize int64
	MIMEType  string
	Metadata  map[string]string
}

type UploadChunkResult struct {
	UploadID       string `json:"upload_id"`
	ChunkIndex     int    `json:"chunk_index"`
	Received       int    `json:"received"`
	ReceivedText   string `json:"received_text"`
	ChunkNum       int    `json:"chunk_num"`
	Done           bool   `json:"done"`
	FilePath       string `json:"file_path"`
	UploadComplete bool   `json:"upload_complete"`
}

type UploadStatus struct {
	UploadID  string `json:"upload_id"`
	FileName  string `json:"file_name"`
	FilePath  string `json:"file_path"`
	ChunkNum  int    `json:"chunk_num"`
	Received  []int  `json:"received"`
	TotalSize int64  `json:"total_size"`
}

type UploadSession struct {
	mu        sync.Mutex
	UploadID  string
	FileName  string
	FilePath  string
	TotalSize int64
	MIMEType  string
	Metadata  map[string]string
	Chunks    *chunk.ChunkUpload
}

type fileWithInfo struct {
	core.FileHeader
	info core.FileInfo
}

func (f fileWithInfo) Info() core.FileInfo { return f.info }

func (fs *FileSystem) Upload(ctx context.Context, file core.FileHeader) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if fs == nil || fs.handler == nil {
		return core.ErrUnknownDriver
	}
	info := fs.prepareInfo(file.Info())
	wrapped := fileWithInfo{FileHeader: file, info: info}

	if err := fs.Trigger(ctx, HookBeforeUpload, info); err != nil {
		_ = fs.Trigger(ctx, HookAfterValidateFailed, info)
		return err
	}
	if err := fs.handler.Put(wrapped); err != nil {
		_ = fs.Trigger(ctx, HookAfterUploadFailed, info)
		return err
	}
	if err := fs.Trigger(ctx, HookAfterUpload, info); err != nil {
		_ = fs.Trigger(ctx, HookAfterValidateFailed, info)
		return err
	}
	return nil
}

func (fs *FileSystem) UploadStream(ctx context.Context, reader io.Reader, info core.FileInfo) (core.FileInfo, error) {
	info = fs.prepareInfo(info)
	return info, fs.Upload(ctx, core.NewFileStream(reader, info))
}

func (fs *FileSystem) UploadFromURL(ctx context.Context, rawURL string, info core.FileInfo) (core.FileInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return core.FileInfo{}, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return core.FileInfo{}, fmt.Errorf("unsupported url scheme: %s", parsed.Scheme)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return core.FileInfo{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return core.FileInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return core.FileInfo{}, fmt.Errorf("download url failed: status %d", resp.StatusCode)
	}
	if info.Name == "" {
		info.Name = filepath.Base(parsed.Path)
	}
	if info.Size <= 0 {
		info.Size = resp.ContentLength
	}
	if info.Size < 0 {
		info.Size = 0
	}
	if info.MIMEType == "" {
		info.MIMEType = resp.Header.Get("Content-Type")
	}
	return fs.UploadStream(ctx, resp.Body, info)
}

func (fs *FileSystem) InitUpload(ctx context.Context, req UploadInitRequest) (*core.UploadCredential, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	info := fs.prepareInfo(core.FileInfo{
		Name:     req.FileName,
		Path:     req.Path,
		Size:     req.TotalSize,
		MIMEType: req.MIMEType,
		Metadata: req.Metadata,
	})
	if err := fs.Trigger(ctx, HookBeforeUpload, info); err != nil {
		_ = fs.Trigger(ctx, HookAfterValidateFailed, info)
		return nil, err
	}

	cred, err := fs.handler.Token(info, fs.cfg.TokenTTL)
	if err != nil {
		_ = fs.Trigger(ctx, HookAfterUploadFailed, info)
		return nil, err
	}
	cred.Path = info.Path
	if cred.ChunkSize <= 0 {
		cred.ChunkSize = fs.cfg.ChunkSize
	}
	if cred.ChunkNum <= 0 && info.Size > 0 && cred.ChunkSize > 0 {
		cred.ChunkNum = chunkNum(info.Size, cred.ChunkSize)
	}

	if cred.Gateway == "local" {
		tempDir := fs.cfg.TempDir
		if tempDir == "" {
			if provider, ok := fs.handler.(interface{ TempDir() string }); ok {
				tempDir = provider.TempDir()
			}
		}
		if tempDir == "" {
			return nil, fmt.Errorf("filesystem temp dir is required for local chunk upload")
		}
		cred.ChunkSize = fs.cfg.ChunkSize
		cred.ChunkNum = chunkNum(info.Size, cred.ChunkSize)
		uploadID := strings.ReplaceAll(uuid.New().String(), "-", "")
		cred.UploadID = uploadID
		cu := chunk.NewChunkUpload(uploadID, info.Name, info.Path, tempDir, info.Size, cred.ChunkSize)
		cred.ChunkNum = cu.ChunkNum
		fs.sessionMu.Lock()
		fs.sessions[uploadID] = &UploadSession{
			UploadID:  uploadID,
			FileName:  info.Name,
			FilePath:  info.Path,
			TotalSize: info.Size,
			MIMEType:  info.MIMEType,
			Metadata:  info.Metadata,
			Chunks:    cu,
		}
		fs.sessionMu.Unlock()
	}
	return cred, nil
}

func (fs *FileSystem) UploadChunk(ctx context.Context, uploadID string, index int, reader io.Reader) (*UploadChunkResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	session, ok := fs.getSession(uploadID)
	if !ok {
		return nil, core.ErrFileNotFound
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if index < 0 || index >= session.Chunks.ChunkNum {
		return nil, fmt.Errorf("chunk index out of range")
	}
	if err := session.Chunks.SaveChunk(index, reader); err != nil {
		return nil, err
	}

	received := session.Chunks.ReceivedChunks()
	done := len(received) == session.Chunks.ChunkNum
	result := &UploadChunkResult{
		UploadID:     uploadID,
		ChunkIndex:   index,
		Received:     len(received),
		ReceivedText: fmt.Sprintf("%d/%d", len(received), session.Chunks.ChunkNum),
		ChunkNum:     session.Chunks.ChunkNum,
		Done:         done,
	}
	if !done {
		return result, nil
	}

	if err := session.Chunks.Merge(); err != nil {
		_ = fs.Trigger(ctx, HookAfterUploadFailed, fs.sessionInfo(session))
		return nil, err
	}
	merged, err := os.Open(session.Chunks.MergePath())
	if err != nil {
		_ = fs.Trigger(ctx, HookAfterUploadFailed, fs.sessionInfo(session))
		return nil, err
	}
	defer merged.Close()

	info, err := fs.UploadStream(ctx, merged, fs.sessionInfo(session))
	if err != nil {
		_ = session.Chunks.Cleanup()
		return nil, err
	}
	result.FilePath = info.Path
	result.UploadComplete = true
	_ = session.Chunks.Cleanup()
	fs.deleteSession(uploadID)
	return result, nil
}

func (fs *FileSystem) UploadStatus(uploadID string) (*UploadStatus, error) {
	session, ok := fs.getSession(uploadID)
	if !ok {
		return nil, core.ErrFileNotFound
	}
	return &UploadStatus{
		UploadID:  uploadID,
		FileName:  session.FileName,
		FilePath:  session.FilePath,
		ChunkNum:  session.Chunks.ChunkNum,
		Received:  session.Chunks.ReceivedChunks(),
		TotalSize: session.TotalSize,
	}, nil
}

func (fs *FileSystem) Download(ctx context.Context, path string, writer io.Writer, progress func(downloaded, total int64)) error {
	if ctx == nil {
		ctx = context.Background()
	}
	info := core.FileInfo{Name: filepath.Base(path), Path: path}
	if err := fs.Trigger(ctx, HookBeforeDownload, info); err != nil {
		return err
	}
	reader, err := fs.Get(path)
	if err != nil {
		_ = fs.Trigger(ctx, HookAfterDownloadFailed, info)
		return err
	}
	defer reader.Close()
	if statReader, ok := reader.(interface{ Stat() (os.FileInfo, error) }); ok {
		if stat, err := statReader.Stat(); err == nil {
			info.Size = stat.Size()
		}
	}

	dst := writer
	if progress != nil {
		dst = &progressWriter{w: writer, total: info.Size, fn: progress}
	}
	if _, err := io.Copy(dst, reader); err != nil {
		_ = fs.Trigger(ctx, HookAfterDownloadFailed, info)
		return err
	}
	return fs.Trigger(ctx, HookAfterDownload, info)
}

func (fs *FileSystem) prepareInfo(info core.FileInfo) core.FileInfo {
	if info.Name == "" && info.Path != "" {
		info.Name = filepath.Base(info.Path)
	}
	if info.Path == "" {
		info.Path = filepath.ToSlash(filepath.Join(fs.cfg.UploadDir, fs.namer.Generate(info.Name)))
	}
	info.Path = filepath.ToSlash(strings.TrimLeft(info.Path, "/"))
	return info
}

func (fs *FileSystem) sessionInfo(session *UploadSession) core.FileInfo {
	return core.FileInfo{
		Name:     session.FileName,
		Path:     session.FilePath,
		Size:     session.TotalSize,
		MIMEType: session.MIMEType,
		Metadata: session.Metadata,
	}
}

func (fs *FileSystem) getSession(uploadID string) (*UploadSession, bool) {
	if fs == nil || fs.sessionMu == nil || fs.sessions == nil {
		return nil, false
	}
	fs.sessionMu.RLock()
	defer fs.sessionMu.RUnlock()
	session, ok := fs.sessions[uploadID]
	return session, ok
}

func (fs *FileSystem) deleteSession(uploadID string) {
	if fs == nil || fs.sessionMu == nil || fs.sessions == nil {
		return
	}
	fs.sessionMu.Lock()
	defer fs.sessionMu.Unlock()
	delete(fs.sessions, uploadID)
}

func chunkNum(size, chunkSize int64) int {
	if size <= 0 || chunkSize <= 0 {
		return 0
	}
	n := int(size / chunkSize)
	if size%chunkSize != 0 {
		n++
	}
	return n
}

type progressWriter struct {
	w     io.Writer
	total int64
	curr  int64
	fn    func(downloaded, total int64)
}

func (w *progressWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if n > 0 {
		w.curr += int64(n)
		w.fn(w.curr, w.total)
	}
	return n, err
}
