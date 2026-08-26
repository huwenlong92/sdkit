package baidupan

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/huwenlong92/sdkit/pkg/execx"
	"github.com/huwenlong92/sdkit/pkg/remotefs"
)

type downloadSink struct {
	mu        sync.Mutex
	sink      remotefs.ProgressSink
	limit     int64
	size      int64
	output    strings.Builder
	savedPath string
	total     int64
	last      int64
	hasLast   bool
	sinkErr   error
}

func (s *downloadSink) WriteCommandEvent(ctx context.Context, event execx.Event) error {
	line := strings.TrimSpace(event.Text)
	if line == "" && len(event.Data) > 0 {
		line = strings.TrimSpace(string(event.Data))
	}
	s.mu.Lock()
	nextSize := s.size + int64(len(event.Data))
	if len(event.Data) == 0 {
		nextSize += int64(len(event.Text))
	}
	if s.limit > 0 && nextSize > s.limit {
		s.mu.Unlock()
		return execx.ErrOutputLimitExceeded
	}
	s.size = nextSize
	if line != "" {
		if s.output.Len() > 0 {
			s.output.WriteByte('\n')
		}
		s.output.WriteString(line)
		if match := savePathPattern.FindStringSubmatch(line); len(match) == 2 {
			s.savedPath = strings.TrimSpace(match[1])
		}
	}
	s.mu.Unlock()
	if progress, ok := parseProgressLine(line); ok {
		s.mu.Lock()
		if (s.hasLast && progress.TransferredBytes < s.last) ||
			(s.total > 0 && progress.TotalBytes > 0 && progress.TotalBytes != s.total) {
			err := &remotefs.Error{Operation: "download progress", Driver: DriverName, Err: remotefs.ErrProtocol, Summary: "provider progress moved backwards or changed total size"}
			s.sinkErr = err
			s.mu.Unlock()
			return err
		}
		s.last = progress.TransferredBytes
		s.hasLast = true
		if progress.TotalBytes > 0 {
			s.total = progress.TotalBytes
		}
		s.mu.Unlock()
		err := remotefs.EmitProgress(ctx, s.sink, progress)
		if err != nil {
			s.mu.Lock()
			s.sinkErr = err
			s.mu.Unlock()
		}
		return err
	}
	return nil
}

func (s *downloadSink) snapshot() (output string, savedPath string, total int64, sinkErr error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.output.String(), s.savedPath, s.total, s.sinkErr
}

func (f *FileSystem) Download(ctx context.Context, req remotefs.DownloadRequest, sink remotefs.ProgressSink) (result remotefs.DownloadResult, resultErr error) {
	if err := f.ready(ctx); err != nil {
		return result, err
	}
	remotePath, err := f.referencePath(req.Reference)
	if err != nil {
		return result, err
	}
	if err := f.ensureAuthenticated(ctx); err != nil {
		return result, err
	}
	before, err := f.Stat(ctx, req.Reference)
	if err != nil {
		return result, err
	}
	if err := validateSnapshot(req.Reference, req.ExpectedVersion, before); err != nil {
		return result, err
	}
	destination := strings.TrimSpace(req.Destination)
	if destination == "" {
		return result, &remotefs.Error{Operation: "download", Driver: DriverName, Err: remotefs.ErrInvalidReference, Summary: "destination is required"}
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return result, &remotefs.Error{Operation: "download", Driver: DriverName, Err: remotefs.ErrInvalidReference, Summary: "destination cannot be resolved"}
	}
	if req.Overwrite == "" {
		req.Overwrite = remotefs.OverwriteDeny
	}
	if req.PartialFile == "" {
		req.PartialFile = remotefs.PartialFileRemove
	}
	if req.PartialFile == remotefs.PartialFileKeep {
		if _, partialErr := os.Lstat(destination + ".partial"); partialErr == nil {
			return result, &remotefs.Error{Operation: "download", Driver: DriverName, Err: remotefs.ErrConflict, Summary: "partial destination already exists"}
		} else if !errors.Is(partialErr, os.ErrNotExist) {
			return result, &remotefs.Error{Operation: "download", Driver: DriverName, Err: joinStandardError(remotefs.ErrTemporary, partialErr), Summary: "partial destination could not be inspected"}
		}
	}
	if _, statErr := os.Stat(destination); statErr == nil && req.Overwrite != remotefs.OverwriteReplace {
		return result, &remotefs.Error{Operation: "download", Driver: DriverName, Err: remotefs.ErrConflict, Summary: "destination already exists"}
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return result, &remotefs.Error{Operation: "download", Driver: DriverName, Err: joinStandardError(remotefs.ErrTemporary, statErr), Summary: "destination could not be inspected"}
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return result, &remotefs.Error{Operation: "download", Driver: DriverName, Err: joinStandardError(remotefs.ErrPermissionDenied, err), Summary: "destination directory could not be created"}
	}
	stagingDir, err := os.MkdirTemp(filepath.Dir(destination), ".baidupan-download-*")
	if err != nil {
		return result, &remotefs.Error{Operation: "download", Driver: DriverName, Err: joinStandardError(remotefs.ErrPermissionDenied, err), Summary: "download staging directory could not be created"}
	}
	succeeded := false
	defer func() {
		if !succeeded && req.PartialFile == remotefs.PartialFileKeep {
			if preserveErr := preservePartial(stagingDir, destination+".partial"); preserveErr != nil {
				resultErr = errors.Join(resultErr, preserveErr)
			}
		}
		_ = os.RemoveAll(stagingDir)
	}()
	if err := remotefs.EmitProgress(ctx, sink, remotefs.Progress{Phase: remotefs.ProgressPreparing}); err != nil {
		return result, err
	}
	command := Command{
		Name: f.runtime.cfg.BinaryPath,
		Args: []string{
			"download",
			"-p", strconv.Itoa(f.cfg.DownloadConcurrency),
			"--mode=" + f.cfg.DownloadMode,
			"--saveto=" + stagingDir,
			remotePath,
		},
		Env:         f.commandEnv(),
		OutputLimit: f.runtime.cfg.OutputLimit,
		SplitMode:   execx.SplitCRLF,
		MergeStderr: true,
	}
	downloadOutput := &downloadSink{sink: sink, limit: f.runtime.cfg.OutputLimit}
	_, runErr := f.runtime.runner.RunStream(ctx, command, downloadOutput)
	secureErr := f.secureSession(ctx)
	output, savedPath, reportedTotal, sinkErr := downloadOutput.snapshot()
	if sinkErr != nil {
		return result, sinkErr
	}
	if runErr != nil {
		runErr = safeRunnerError(runErr)
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if standard, summary, classified := classifyOutput("download", output); classified {
			return result, &remotefs.Error{Operation: "download", Driver: DriverName, Err: joinStandardError(standard, runErr), Summary: summary}
		}
		return result, &remotefs.Error{Operation: "download", Driver: DriverName, Err: joinStandardError(remotefs.ErrTemporary, runErr), Summary: "BaiduPCS-Go download failed"}
	}
	if secureErr != nil {
		return result, secureErr
	}
	if standard, summary, failed := classifyOutput("download", output); failed {
		return result, &remotefs.Error{Operation: "download", Driver: DriverName, Err: standard, Summary: summary}
	}
	if savedPath == "" {
		return result, errorForOutput("download", output, f.cfg.BDUSS, f.cfg.STOKEN)
	}
	resolvedSavedPath, err := confinedSavedPath(stagingDir, savedPath)
	if err != nil {
		return result, err
	}
	info, err := os.Stat(resolvedSavedPath)
	if err != nil {
		return result, &remotefs.Error{Operation: "download", Driver: DriverName, Err: joinStandardError(remotefs.ErrTemporary, err), Summary: "reported downloaded file could not be inspected"}
	}
	if !info.Mode().IsRegular() {
		return result, &remotefs.Error{Operation: "download", Driver: DriverName, Err: remotefs.ErrUnsupported, Summary: "reported download result is not a regular file"}
	}
	after, err := f.Stat(ctx, req.Reference)
	if err != nil {
		return result, err
	}
	if before.Reference.ID != after.Reference.ID || before.Version != after.Version || before.Size != after.Size {
		return result, &remotefs.Error{Operation: "download", Driver: DriverName, Err: remotefs.ErrSourceChanged, Summary: "source changed during download"}
	}
	if info.Size() != after.Size {
		return result, &remotefs.Error{Operation: "download", Driver: DriverName, Err: remotefs.ErrIntegrity, Summary: "downloaded byte count does not match the source"}
	}
	if reportedTotal == 0 {
		reportedTotal = info.Size()
	}
	if err := remotefs.EmitProgress(ctx, sink, remotefs.Progress{Phase: remotefs.ProgressFinalizing, TransferredBytes: info.Size(), TotalBytes: reportedTotal}); err != nil {
		return result, err
	}
	if err := commitDownload(resolvedSavedPath, destination, req.Overwrite); err != nil {
		return result, err
	}
	result = remotefs.DownloadResult{Path: destination, BytesWritten: info.Size()}
	succeeded = true
	return result, nil
}

func commitDownload(stagedPath string, destination string, policy remotefs.OverwritePolicy) error {
	if policy == remotefs.OverwriteReplace {
		if err := os.Rename(stagedPath, destination); err != nil {
			return &remotefs.Error{Operation: "download", Driver: DriverName, Err: err, Summary: "downloaded file could not be moved to its destination"}
		}
		return nil
	}
	if err := os.Link(stagedPath, destination); err != nil {
		if errors.Is(err, os.ErrExist) {
			return &remotefs.Error{Operation: "download", Driver: DriverName, Err: remotefs.ErrConflict, Summary: "destination already exists"}
		}
		return &remotefs.Error{Operation: "download", Driver: DriverName, Err: err, Summary: "downloaded file could not be committed"}
	}
	if err := os.Remove(stagedPath); err != nil {
		return &remotefs.Error{Operation: "download", Driver: DriverName, Err: err, Summary: "download staging file could not be released"}
	}
	return nil
}

func confinedSavedPath(stagingDir string, savedPath string) (string, error) {
	absolute, err := filepath.Abs(savedPath)
	if err != nil {
		return "", &remotefs.Error{Operation: "download", Driver: DriverName, Err: remotefs.ErrPermissionDenied, Summary: "reported save path is invalid"}
	}
	if err := ensureWithinDirectory(stagingDir, absolute); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", &remotefs.Error{Operation: "download", Driver: DriverName, Err: joinStandardError(remotefs.ErrTemporary, err), Summary: "reported save path could not be resolved"}
	}
	resolvedStagingDir, err := filepath.EvalSymlinks(stagingDir)
	if err != nil {
		return "", &remotefs.Error{Operation: "download", Driver: DriverName, Err: joinStandardError(remotefs.ErrTemporary, err), Summary: "download staging directory could not be resolved"}
	}
	if err := ensureWithinDirectory(resolvedStagingDir, resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

func ensureWithinDirectory(directory string, candidate string) error {
	relative, err := filepath.Rel(directory, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return &remotefs.Error{Operation: "download", Driver: DriverName, Err: remotefs.ErrPermissionDenied, Summary: "reported save path escapes the staging directory"}
	}
	return nil
}

func preservePartial(stagingDir string, destination string) error {
	var candidate string
	var candidateSize int64 = -1
	err := filepath.WalkDir(stagingDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Size() > candidateSize {
				candidate = path
				candidateSize = info.Size()
			}
		}
		return nil
	})
	if err != nil || candidate == "" {
		return err
	}
	if err := os.Link(candidate, destination); err != nil {
		if errors.Is(err, os.ErrExist) {
			return &remotefs.Error{Operation: "preserve partial", Driver: DriverName, Err: remotefs.ErrConflict, Summary: "partial destination already belongs to another download"}
		}
		return &remotefs.Error{Operation: "preserve partial", Driver: DriverName, Err: err, Summary: "partial download could not be preserved"}
	}
	if err := os.Remove(candidate); err != nil {
		return &remotefs.Error{Operation: "preserve partial", Driver: DriverName, Err: err, Summary: "partial staging file could not be released"}
	}
	return nil
}
