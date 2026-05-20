package storage

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

	"github.com/huwenlong92/sdkit/pkg/storage/chunk"
	"github.com/huwenlong92/sdkit/pkg/storage/core"

	"github.com/google/uuid"
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

type UploadResult struct {
	File     core.FileInfo
	Uploaded bool
	Error    error
}

func (r UploadResult) OK() bool {
	return r.Error == nil
}

func (r UploadResult) AfterFailed() bool {
	return r.Uploaded && r.Error != nil
}

type DownloadResult struct {
	File       core.FileInfo
	Downloaded bool
	Error      error
}

func (r DownloadResult) OK() bool {
	return r.Error == nil
}

func (r DownloadResult) AfterFailed() bool {
	return r.Downloaded && r.Error != nil
}

type GetResult struct {
	File   core.FileInfo
	Reader io.ReadCloser
	Opened bool
	Error  error
}

type fileWithInfo struct {
	core.FileHeader
	info core.FileInfo
}

func (f fileWithInfo) Info() core.FileInfo { return f.info }

func (fs *FileSystem) Upload(ctx context.Context, file core.FileHeader) UploadResult {
	return fs.UploadWithHook(ctx, file)
}

func (fs *FileSystem) UploadWithHook(ctx context.Context, file core.FileHeader, opts ...HookOption) UploadResult {
	return fs.upload(ctx, file, collectHookOptions(opts...))
}

func (fs *FileSystem) upload(ctx context.Context, file core.FileHeader, hooks operationHooks) UploadResult {
	if ctx == nil {
		ctx = context.Background()
	}
	if fs == nil || fs.handler == nil {
		return UploadResult{Error: core.ErrUnknownDriver}
	}
	info := fs.prepareInfo(file.Info())
	wrapped := fileWithInfo{FileHeader: file, info: info}

	if err := fs.Trigger(ctx, HookBeforeUpload, hooks.event(fs, OperationUpload, HookBeforeUpload, info, nil), hooks.beforeUpload...); err != nil {
		return UploadResult{File: info, Error: err}
	}
	if err := fs.handler.Put(wrapped); err != nil {
		_ = fs.Trigger(ctx, HookAfterUploadFailed, hooks.event(fs, OperationUpload, HookAfterUploadFailed, info, err), hooks.afterUploadFailed...)
		return UploadResult{File: info, Error: err}
	}
	if err := fs.Trigger(ctx, HookAfterUpload, hooks.event(fs, OperationUpload, HookAfterUpload, info, nil), hooks.afterUpload...); err != nil {
		return UploadResult{File: info, Uploaded: true, Error: err}
	}
	return UploadResult{File: info, Uploaded: true}
}

func (fs *FileSystem) UploadStream(ctx context.Context, reader io.Reader, info core.FileInfo) UploadResult {
	return fs.UploadStreamWithHook(ctx, reader, info)
}

func (fs *FileSystem) UploadStreamWithHook(ctx context.Context, reader io.Reader, info core.FileInfo, opts ...HookOption) UploadResult {
	return fs.uploadStream(ctx, reader, info, collectHookOptions(opts...))
}

func (fs *FileSystem) uploadStream(ctx context.Context, reader io.Reader, info core.FileInfo, hooks operationHooks) UploadResult {
	info = fs.prepareInfo(info)
	return fs.upload(ctx, core.NewFileStream(reader, info), hooks)
}

func (fs *FileSystem) UploadFromURL(ctx context.Context, rawURL string, info core.FileInfo) UploadResult {
	return fs.UploadFromURLWithHook(ctx, rawURL, info)
}

func (fs *FileSystem) UploadFromURLWithHook(ctx context.Context, rawURL string, info core.FileInfo, opts ...HookOption) UploadResult {
	return fs.uploadFromURL(ctx, rawURL, info, collectHookOptions(opts...))
}

func (fs *FileSystem) uploadFromURL(ctx context.Context, rawURL string, info core.FileInfo, hooks operationHooks) UploadResult {
	if ctx == nil {
		ctx = context.Background()
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return UploadResult{Error: err}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return UploadResult{Error: fmt.Errorf("unsupported url scheme: %s", parsed.Scheme)}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return UploadResult{Error: err}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return UploadResult{Error: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return UploadResult{Error: fmt.Errorf("download url failed: status %d", resp.StatusCode)}
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
	return fs.uploadStream(ctx, resp.Body, info, hooks)
}

func (fs *FileSystem) InitUpload(ctx context.Context, req UploadInitRequest) (*core.UploadCredential, error) {
	return fs.InitUploadWithHook(ctx, req)
}

func (fs *FileSystem) InitUploadWithHook(ctx context.Context, req UploadInitRequest, opts ...HookOption) (*core.UploadCredential, error) {
	return fs.initUpload(ctx, req, collectHookOptions(opts...))
}

func (fs *FileSystem) initUpload(ctx context.Context, req UploadInitRequest, hooks operationHooks) (*core.UploadCredential, error) {
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
	if err := fs.Trigger(ctx, HookBeforeUpload, hooks.event(fs, OperationUpload, HookBeforeUpload, info, nil), hooks.beforeUpload...); err != nil {
		return nil, err
	}

	cred, err := fs.handler.Token(info, fs.cfg.TokenTTL)
	if err != nil {
		_ = fs.Trigger(ctx, HookAfterUploadFailed, hooks.event(fs, OperationUpload, HookAfterUploadFailed, info, err), hooks.afterUploadFailed...)
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
			return nil, fmt.Errorf("storage temp dir is required for local chunk upload")
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
	return fs.UploadChunkWithHook(ctx, uploadID, index, reader)
}

func (fs *FileSystem) UploadChunkWithHook(ctx context.Context, uploadID string, index int, reader io.Reader, opts ...HookOption) (*UploadChunkResult, error) {
	return fs.uploadChunk(ctx, uploadID, index, reader, collectHookOptions(opts...))
}

func (fs *FileSystem) uploadChunk(ctx context.Context, uploadID string, index int, reader io.Reader, hooks operationHooks) (*UploadChunkResult, error) {
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
		_ = fs.Trigger(ctx, HookAfterUploadFailed, hooks.event(fs, OperationUpload, HookAfterUploadFailed, fs.sessionInfo(session), err), hooks.afterUploadFailed...)
		return nil, err
	}
	merged, err := os.Open(session.Chunks.MergePath())
	if err != nil {
		_ = fs.Trigger(ctx, HookAfterUploadFailed, hooks.event(fs, OperationUpload, HookAfterUploadFailed, fs.sessionInfo(session), err), hooks.afterUploadFailed...)
		return nil, err
	}
	defer merged.Close()

	upload := fs.uploadStream(ctx, merged, fs.sessionInfo(session), hooks)
	if upload.Error != nil {
		_ = session.Chunks.Cleanup()
		if upload.Uploaded {
			result.FilePath = upload.File.Path
			result.UploadComplete = true
			fs.deleteSession(uploadID)
			return result, upload.Error
		}
		return nil, upload.Error
	}
	result.FilePath = upload.File.Path
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

func (fs *FileSystem) Download(ctx context.Context, path string, writer io.Writer, progress func(downloaded, total int64)) DownloadResult {
	return fs.DownloadWithHook(ctx, path, writer, progress)
}

func (fs *FileSystem) DownloadWithHook(ctx context.Context, path string, writer io.Writer, progress func(downloaded, total int64), opts ...HookOption) DownloadResult {
	return fs.download(ctx, path, writer, progress, collectHookOptions(opts...))
}

func (fs *FileSystem) download(ctx context.Context, path string, writer io.Writer, progress func(downloaded, total int64), hooks operationHooks) DownloadResult {
	if ctx == nil {
		ctx = context.Background()
	}
	info := core.FileInfo{Name: filepath.Base(path), Path: path}
	if err := fs.Trigger(ctx, HookBeforeDownload, hooks.event(fs, OperationDownload, HookBeforeDownload, info, nil), hooks.beforeDownload...); err != nil {
		return DownloadResult{File: info, Error: err}
	}
	reader, err := fs.get(path)
	if err != nil {
		_ = fs.Trigger(ctx, HookAfterDownloadFailed, hooks.event(fs, OperationDownload, HookAfterDownloadFailed, info, err), hooks.afterDownloadFailed...)
		return DownloadResult{File: info, Error: err}
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
		_ = fs.Trigger(ctx, HookAfterDownloadFailed, hooks.event(fs, OperationDownload, HookAfterDownloadFailed, info, err), hooks.afterDownloadFailed...)
		return DownloadResult{File: info, Error: err}
	}
	if err := fs.Trigger(ctx, HookAfterDownload, hooks.event(fs, OperationDownload, HookAfterDownload, info, nil), hooks.afterDownload...); err != nil {
		return DownloadResult{File: info, Downloaded: true, Error: err}
	}
	return DownloadResult{File: info, Downloaded: true}
}

func (fs *FileSystem) getWithHook(ctx context.Context, path string, hooks operationHooks) GetResult {
	if ctx == nil {
		ctx = context.Background()
	}
	info := core.FileInfo{Name: filepath.Base(path), Path: path}
	if err := fs.Trigger(ctx, HookBeforeGet, hooks.eventWith(fs, OperationGet, HookBeforeGet, info, nil, func(event *Event) {
		event.Path = path
	}), hooks.beforeGet...); err != nil {
		return GetResult{File: info, Error: err}
	}
	reader, err := fs.get(path)
	if err != nil {
		_ = fs.Trigger(ctx, HookAfterGetFailed, hooks.eventWith(fs, OperationGet, HookAfterGetFailed, info, err, func(event *Event) {
			event.Path = path
		}), hooks.afterGetFailed...)
		return GetResult{File: info, Error: err}
	}
	if statReader, ok := reader.(interface{ Stat() (os.FileInfo, error) }); ok {
		if stat, err := statReader.Stat(); err == nil {
			info.Size = stat.Size()
		}
	}
	if err := fs.Trigger(ctx, HookAfterGet, hooks.eventWith(fs, OperationGet, HookAfterGet, info, nil, func(event *Event) {
		event.Path = path
		event.Reader = reader
	}), hooks.afterGet...); err != nil {
		return GetResult{File: info, Reader: reader, Opened: true, Error: err}
	}
	return GetResult{File: info, Reader: reader, Opened: true}
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
