// Package local implements RemoteFS over a confined local directory. It is
// suitable for NAS volumes already mounted through NFS, SMB, or WebDAV.
package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/huwenlong92/sdkit/pkg/remotefs"
	"github.com/huwenlong92/sdkit/pkg/remotefs/internal/offsetcursor"
)

const DriverName = "local"

const (
	defaultPageSize = 100
	maxPageSize     = 1000
	copyBufferSize  = 256 * 1024
)

type Config struct {
	Root        string `mapstructure:"root" yaml:"root"`
	AllowRemove bool   `mapstructure:"allow_remove" yaml:"allow_remove"`
}

type FileSystem struct {
	rootPath string
	root     *os.Root
	closed   atomic.Bool
}

type removableFileSystem struct {
	*FileSystem
}

func New(cfg Config) (remotefs.FileSystem, error) {
	fs, err := newFileSystem(cfg)
	if err != nil {
		return nil, err
	}
	return withOptionalRemove(fs, cfg.AllowRemove), nil
}

func newFileSystem(cfg Config) (*FileSystem, error) {
	root := strings.TrimSpace(cfg.Root)
	if root == "" {
		return nil, remotefs.WrapError("open", DriverName, remotefs.ErrInvalidReference, "root is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, remotefs.WrapError("open", DriverName, remotefs.ErrInvalidReference, "root cannot be resolved")
	}
	info, err := os.Lstat(absRoot)
	if err != nil {
		return nil, mapFileError("open", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, remotefs.WrapError("open", DriverName, remotefs.ErrInvalidReference, "root must be a directory")
	}
	rootHandle, err := os.OpenRoot(absRoot)
	if err != nil {
		return nil, mapFileError("open", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		_ = rootHandle.Close()
		return nil, mapFileError("open", err)
	}
	return &FileSystem{rootPath: filepath.Clean(canonicalRoot), root: rootHandle}, nil
}

func (f *FileSystem) Driver() string { return DriverName }

func (f *FileSystem) Stat(ctx context.Context, ref remotefs.Reference) (remotefs.Entry, error) {
	if err := f.ready(ctx); err != nil {
		return remotefs.Entry{}, err
	}
	_, remotePath, info, err := f.inspect(ref, false)
	if err != nil {
		return remotefs.Entry{}, err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return remotefs.Entry{}, remotefs.WrapError("stat", DriverName, remotefs.ErrUnsupported, "reference is not a regular file or directory")
	}
	return f.entry(remotePath, info), nil
}

func (f *FileSystem) List(ctx context.Context, dir remotefs.Reference, opts remotefs.ListOptions) (remotefs.ListPage, error) {
	if err := f.ready(ctx); err != nil {
		return remotefs.ListPage{}, err
	}
	if err := validateListOptions(opts); err != nil {
		return remotefs.ListPage{}, err
	}
	name, remotePath, info, err := f.inspect(dir, false)
	if err != nil {
		return remotefs.ListPage{}, err
	}
	if !info.IsDir() {
		return remotefs.ListPage{}, remotefs.WrapError("list", DriverName, remotefs.ErrNotDirectory, "reference is not a directory")
	}
	directory, err := f.root.Open(name)
	if err != nil {
		return remotefs.ListPage{}, mapFileError("list", err)
	}
	defer directory.Close()
	openedInfo, err := directory.Stat()
	if err != nil {
		return remotefs.ListPage{}, mapFileError("list", err)
	}
	if !os.SameFile(info, openedInfo) || !openedInfo.IsDir() {
		return remotefs.ListPage{}, remotefs.WrapError("list", DriverName, remotefs.ErrSourceChanged, "directory changed while it was being opened")
	}
	dirEntries, err := directory.ReadDir(-1)
	if err != nil {
		return remotefs.ListPage{}, mapFileError("list", err)
	}
	entries := make([]remotefs.Entry, 0, len(dirEntries))
	for _, item := range dirEntries {
		if err := ctx.Err(); err != nil {
			return remotefs.ListPage{}, err
		}
		childRemotePath := path.Join(remotePath, item.Name())
		_, _, childInfo, inspectErr := f.inspect(remotefs.Reference{Path: childRemotePath}, false)
		if inspectErr != nil {
			return remotefs.ListPage{}, inspectErr
		}
		if !childInfo.IsDir() && !childInfo.Mode().IsRegular() {
			return remotefs.ListPage{}, remotefs.WrapError("list", DriverName, remotefs.ErrUnsupported, "directory contains an unsupported file type")
		}
		entries = append(entries, f.entry(childRemotePath, childInfo))
	}
	sortEntries(entries, opts)
	cursorScope := listCursorScope(remotePath, opts)
	start, err := decodeCursor(opts.Cursor, cursorScope, len(entries))
	if err != nil {
		return remotefs.ListPage{}, err
	}
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	end := start + pageSize
	if end > len(entries) {
		end = len(entries)
	}
	page := remotefs.ListPage{Entries: append([]remotefs.Entry(nil), entries[start:end]...)}
	if end < len(entries) {
		page.NextCursor = offsetcursor.Encode(cursorScope, end)
	}
	return page, nil
}

func (f *FileSystem) Open(ctx context.Context, ref remotefs.Reference, opts remotefs.OpenOptions) (io.ReadCloser, error) {
	if err := f.ready(ctx); err != nil {
		return nil, err
	}
	if opts.Offset < 0 {
		return nil, remotefs.WrapError("open", DriverName, remotefs.ErrInvalidOption, "offset cannot be negative")
	}
	name, _, info, err := f.inspect(ref, false)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, remotefs.WrapError("open", DriverName, remotefs.ErrNotRegularFile, "reference is not a regular file")
	}
	file, err := f.root.Open(name)
	if err != nil {
		return nil, mapFileError("open", err)
	}
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) || !openedInfo.Mode().IsRegular() {
		_ = file.Close()
		if err != nil {
			return nil, mapFileError("open", err)
		}
		return nil, remotefs.WrapError("open", DriverName, remotefs.ErrSourceChanged, "reference changed while it was being opened")
	}
	if opts.Offset > 0 {
		if _, err := file.Seek(opts.Offset, io.SeekStart); err != nil {
			_ = file.Close()
			return nil, mapFileError("open", err)
		}
	}
	return file, nil
}

func (f *FileSystem) Download(ctx context.Context, req remotefs.DownloadRequest, sink remotefs.ProgressSink) (result remotefs.DownloadResult, resultErr error) {
	if err := f.ready(ctx); err != nil {
		return result, err
	}
	name, remotePath, inspectedInfo, err := f.inspect(req.Reference, false)
	if err != nil {
		return result, err
	}
	if !inspectedInfo.Mode().IsRegular() {
		return result, remotefs.WrapError("download", DriverName, remotefs.ErrNotRegularFile, "reference is not a regular file")
	}
	source, err := f.root.Open(name)
	if err != nil {
		return result, mapFileError("download", err)
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return result, mapFileError("download", err)
	}
	if !info.Mode().IsRegular() {
		return result, remotefs.WrapError("download", DriverName, remotefs.ErrNotRegularFile, "reference is not a regular file")
	}
	if !os.SameFile(inspectedInfo, info) {
		return result, remotefs.WrapError("download", DriverName, remotefs.ErrSourceChanged, "reference changed while it was being opened")
	}
	snapshot := f.entry(remotePath, info)
	if err := validateSnapshot(req.Reference, req.ExpectedVersion, snapshot); err != nil {
		return result, err
	}
	destination := strings.TrimSpace(req.Destination)
	if destination == "" {
		return result, remotefs.WrapError("download", DriverName, remotefs.ErrInvalidReference, "destination is required")
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return result, remotefs.WrapError("download", DriverName, remotefs.ErrInvalidReference, "destination cannot be resolved")
	}
	if destinationWithinRoot(f.rootPath, destination) {
		return result, remotefs.WrapError("download", DriverName, remotefs.ErrPermissionDenied, "destination must be outside the source root")
	}
	if req.Overwrite == "" {
		req.Overwrite = remotefs.OverwriteDeny
	}
	if req.PartialFile == "" {
		req.PartialFile = remotefs.PartialFileRemove
	}
	if _, err := os.Stat(destination); err == nil && req.Overwrite != remotefs.OverwriteReplace {
		return result, remotefs.WrapError("download", DriverName, remotefs.ErrConflict, "destination already exists")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return result, mapFileError("download", err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return result, mapFileError("download", err)
	}
	partialPath := destination + ".partial"
	partial, err := os.OpenFile(partialPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return result, remotefs.WrapError("download", DriverName, remotefs.ErrConflict, "partial destination already exists")
		}
		return result, mapFileError("download", err)
	}
	keepPartial := req.PartialFile == remotefs.PartialFileKeep
	defer func() {
		if partial != nil {
			_ = partial.Close()
		}
		if resultErr != nil && !keepPartial {
			_ = os.Remove(partialPath)
		}
	}()
	if err := remotefs.EmitProgress(ctx, sink, remotefs.Progress{Phase: remotefs.ProgressPreparing, TotalBytes: info.Size()}); err != nil {
		return result, err
	}
	started := time.Now()
	written, err := copyWithProgress(ctx, partial, source, info.Size(), started, sink)
	if err != nil {
		return result, err
	}
	_, _, currentInfo, err := f.inspect(req.Reference, false)
	if err != nil {
		return result, remotefs.WrapError("download", DriverName, remotefs.ErrSourceChanged, "source could not be revalidated")
	}
	current := f.entry(remotePath, currentInfo)
	if !os.SameFile(info, currentInfo) || current.Reference.ID != snapshot.Reference.ID || current.Version != snapshot.Version {
		return result, remotefs.WrapError("download", DriverName, remotefs.ErrSourceChanged, "source changed during download")
	}
	if written != info.Size() {
		return result, remotefs.WrapError("download", DriverName, remotefs.ErrIntegrity, "downloaded byte count does not match the source")
	}
	if err := partial.Sync(); err != nil {
		return result, mapFileError("download", err)
	}
	if err := partial.Close(); err != nil {
		return result, mapFileError("download", err)
	}
	partial = nil
	if err := remotefs.EmitProgress(ctx, sink, remotefs.Progress{
		Phase:            remotefs.ProgressFinalizing,
		TransferredBytes: written,
		TotalBytes:       info.Size(),
		BytesPerSecond:   transferRate(written, started),
	}); err != nil {
		return result, err
	}
	if err := commitPartial(partialPath, destination, req.Overwrite); err != nil {
		return result, mapFileError("download", err)
	}
	result = remotefs.DownloadResult{Path: destination, BytesWritten: written}
	return result, nil
}

func (f *FileSystem) remove(ctx context.Context, req remotefs.RemoveRequest) error {
	if err := f.ready(ctx); err != nil {
		return err
	}
	name, remotePath, info, err := f.inspect(req.Reference, true)
	if err != nil {
		return err
	}
	if remotePath == "/" {
		return remotefs.WrapError("remove", DriverName, remotefs.ErrPermissionDenied, "root cannot be removed")
	}
	if err := validateSnapshot(req.Reference, req.ExpectedVersion, f.entry(remotePath, info)); err != nil {
		return err
	}
	if err := f.root.Remove(name); err != nil {
		return mapFileError("remove", err)
	}
	return nil
}

func (f *removableFileSystem) Remove(ctx context.Context, req remotefs.RemoveRequest) error {
	return f.FileSystem.remove(ctx, req)
}

func (f *FileSystem) Close() error {
	if f == nil || !f.closed.CompareAndSwap(false, true) {
		return nil
	}
	return f.root.Close()
}

func (f *FileSystem) ready(ctx context.Context) error {
	if f == nil || f.root == nil || f.closed.Load() {
		return remotefs.ErrClosed
	}
	if ctx == nil {
		return remotefs.ErrNilContext
	}
	return ctx.Err()
}

func (f *FileSystem) inspect(ref remotefs.Reference, allowFinalSymlink bool) (string, string, os.FileInfo, error) {
	remotePath, err := cleanRemotePath(ref.Path)
	if err != nil {
		return "", "", nil, err
	}
	name := filepath.FromSlash(strings.TrimPrefix(remotePath, "/"))
	if name == "" {
		name = "."
	}
	parts := strings.Split(filepath.ToSlash(name), "/")
	prefix := ""
	var info os.FileInfo
	for index, part := range parts {
		if part == "" || part == "." {
			prefix = "."
		} else if prefix == "" || prefix == "." {
			prefix = part
		} else {
			prefix = filepath.Join(prefix, part)
		}
		info, err = f.root.Lstat(prefix)
		if err != nil {
			return "", "", nil, mapFileError("resolve", err)
		}
		final := index == len(parts)-1
		if info.Mode()&os.ModeSymlink != 0 && (!final || !allowFinalSymlink) {
			return "", "", nil, remotefs.WrapError("resolve", DriverName, remotefs.ErrPermissionDenied, "symbolic links are not allowed")
		}
		if !final && !info.IsDir() {
			return "", "", nil, remotefs.WrapError("resolve", DriverName, remotefs.ErrNotDirectory, "path component is not a directory")
		}
	}
	if info == nil {
		return "", "", nil, remotefs.WrapError("resolve", DriverName, remotefs.ErrNotFound, "path was not found")
	}
	return name, remotePath, info, nil
}

func cleanRemotePath(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || value == "." {
		return "/", nil
	}
	if strings.ContainsRune(value, '\x00') {
		return "", remotefs.WrapError("resolve", DriverName, remotefs.ErrPermissionDenied, "path contains an invalid character")
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return "", remotefs.WrapError("resolve", DriverName, remotefs.ErrPermissionDenied, "parent traversal is not allowed")
		}
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(value, "/"))
	if cleaned == "." {
		cleaned = "/"
	}
	return cleaned, nil
}

func withinRoot(root string, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func destinationWithinRoot(root string, destination string) bool {
	parent, err := filepath.EvalSymlinks(filepath.Dir(destination))
	if err != nil {
		return withinRoot(root, destination)
	}
	return withinRoot(root, filepath.Join(parent, filepath.Base(destination)))
}

func validateListOptions(opts remotefs.ListOptions) error {
	if opts.PageSize < 0 || opts.PageSize > maxPageSize {
		return remotefs.WrapError("list", DriverName, remotefs.ErrInvalidOption, "page size is outside the supported range")
	}
	switch opts.SortBy {
	case "", remotefs.SortByName, remotefs.SortBySize, remotefs.SortByModTime:
	default:
		return remotefs.WrapError("list", DriverName, remotefs.ErrInvalidOption, "sort field is invalid")
	}
	switch opts.Order {
	case "", remotefs.SortAscending, remotefs.SortDescending:
	default:
		return remotefs.WrapError("list", DriverName, remotefs.ErrInvalidOption, "sort order is invalid")
	}
	return nil
}

func commitPartial(partialPath string, destination string, policy remotefs.OverwritePolicy) error {
	if policy == remotefs.OverwriteReplace {
		return os.Rename(partialPath, destination)
	}
	if err := os.Link(partialPath, destination); err != nil {
		if errors.Is(err, os.ErrExist) {
			return remotefs.WrapError("download", DriverName, remotefs.ErrConflict, "destination already exists")
		}
		return err
	}
	if err := os.Remove(partialPath); err != nil {
		return err
	}
	return nil
}

func (f *FileSystem) entry(remotePath string, info os.FileInfo) remotefs.Entry {
	entryType := remotefs.EntryFile
	if info.IsDir() {
		entryType = remotefs.EntryDirectory
	}
	name := info.Name()
	if remotePath == "/" {
		name = "/"
	}
	modTime := info.ModTime()
	size := info.Size()
	if info.IsDir() {
		size = 0
	}
	ref := remotefs.Reference{ID: localFileID(remotePath, info), Path: remotePath}
	return remotefs.Entry{
		Reference: ref,
		Name:      name,
		Type:      entryType,
		Size:      size,
		ModTime:   &modTime,
		Version:   fmt.Sprintf("%d:%d", info.ModTime().UnixNano(), info.Size()),
	}
}

func localFileID(remotePath string, info os.FileInfo) string {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino)
	}
	return remotePath
}

func validateSnapshot(ref remotefs.Reference, expectedVersion string, entry remotefs.Entry) error {
	if ref.ID != "" && ref.ID != entry.Reference.ID || expectedVersion != "" && expectedVersion != entry.Version {
		return remotefs.WrapError("validate", DriverName, remotefs.ErrSourceChanged, "source snapshot no longer matches the request")
	}
	return nil
}

func sortEntries(entries []remotefs.Entry, opts remotefs.ListOptions) {
	sort.SliceStable(entries, func(i int, j int) bool {
		comparison := 0
		switch opts.SortBy {
		case remotefs.SortBySize:
			switch {
			case entries[i].Size < entries[j].Size:
				comparison = -1
			case entries[i].Size > entries[j].Size:
				comparison = 1
			}
		case remotefs.SortByModTime:
			switch {
			case entries[i].ModTime == nil && entries[j].ModTime != nil:
				comparison = -1
			case entries[i].ModTime != nil && entries[j].ModTime == nil:
				comparison = 1
			case entries[i].ModTime != nil && entries[j].ModTime != nil && entries[i].ModTime.Before(*entries[j].ModTime):
				comparison = -1
			case entries[i].ModTime != nil && entries[j].ModTime != nil && entries[i].ModTime.After(*entries[j].ModTime):
				comparison = 1
			}
		default:
			comparison = strings.Compare(entries[i].Name, entries[j].Name)
		}
		if comparison == 0 {
			comparison = strings.Compare(entries[i].Name, entries[j].Name)
		}
		if opts.Order == remotefs.SortDescending {
			return comparison > 0
		}
		return comparison < 0
	})
}

func listCursorScope(remotePath string, opts remotefs.ListOptions) string {
	sortBy := opts.SortBy
	if sortBy == "" {
		sortBy = remotefs.SortByName
	}
	order := opts.Order
	if order == "" {
		order = remotefs.SortAscending
	}
	return offsetcursor.Scope(remotePath, string(sortBy), string(order))
}

func decodeCursor(cursor string, scope string, length int) (int, error) {
	value, err := offsetcursor.Decode(cursor, scope, length)
	if err != nil {
		return 0, remotefs.WrapError("list", DriverName, remotefs.ErrInvalidOption, "pagination cursor does not belong to this query")
	}
	return value, nil
}

func copyWithProgress(ctx context.Context, dst io.Writer, src io.Reader, total int64, started time.Time, sink remotefs.ProgressSink) (int64, error) {
	buffer := make([]byte, copyBufferSize)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		n, readErr := src.Read(buffer)
		if n > 0 {
			writeN, writeErr := dst.Write(buffer[:n])
			written += int64(writeN)
			if writeErr != nil {
				return written, mapFileError("download", writeErr)
			}
			if writeN != n {
				return written, io.ErrShortWrite
			}
			if err := remotefs.EmitProgress(ctx, sink, remotefs.Progress{
				Phase:            remotefs.ProgressTransferring,
				TransferredBytes: written,
				TotalBytes:       total,
				BytesPerSecond:   transferRate(written, started),
			}); err != nil {
				return written, err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return written, nil
		}
		if readErr != nil {
			return written, mapFileError("download", readErr)
		}
	}
}

func transferRate(written int64, started time.Time) float64 {
	elapsed := time.Since(started).Seconds()
	if elapsed <= 0 {
		return 0
	}
	return float64(written) / elapsed
}

func mapFileError(operation string, err error) error {
	if err == nil {
		return nil
	}
	standard := remotefs.ErrTemporary
	summary := "local file operation failed"
	switch {
	case errors.Is(err, os.ErrNotExist):
		standard = remotefs.ErrNotFound
		summary = "path was not found"
	case errors.Is(err, os.ErrPermission):
		standard = remotefs.ErrPermissionDenied
		summary = "permission was denied"
	}
	return &remotefs.Error{Operation: operation, Driver: DriverName, Err: errors.Join(standard, err), Summary: summary}
}

func withOptionalRemove(fs *FileSystem, enabled bool) remotefs.FileSystem {
	if enabled {
		return &removableFileSystem{FileSystem: fs}
	}
	return fs
}
