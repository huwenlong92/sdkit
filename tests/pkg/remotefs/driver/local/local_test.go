package local_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/pkg/remotefs"
	localdriver "github.com/huwenlong92/sdkit/pkg/remotefs/driver/local"
	"github.com/huwenlong92/sdkit/tests/pkg/remotefs/testsuite"
)

func TestLocalDriverStatListOpenAndDownload(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "video.mp4"), []byte("media-data"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	fs, err := localdriver.New(localdriver.Config{Root: root})
	if err != nil {
		t.Fatalf("new local: %v", err)
	}
	t.Cleanup(func() { _ = fs.Close() })

	entry, err := fs.Stat(context.Background(), remotefs.Reference{Path: "/nested/video.mp4"})
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if entry.Type != remotefs.EntryFile || entry.Size != int64(len("media-data")) {
		t.Fatalf("entry = %+v", entry)
	}

	page, err := fs.List(context.Background(), remotefs.Reference{Path: "/nested"}, remotefs.ListOptions{PageSize: 1})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Entries) != 1 || page.Entries[0].Name != "video.mp4" || page.NextCursor != "" {
		t.Fatalf("page = %+v", page)
	}

	opener, ok := fs.(remotefs.Opener)
	if !ok {
		t.Fatal("local driver does not implement Opener")
	}
	reader, err := opener.Open(context.Background(), remotefs.Reference{Path: "/nested/video.mp4"}, remotefs.OpenOptions{Offset: 6})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	data, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read/close: %v / %v", readErr, closeErr)
	}
	if string(data) != "data" {
		t.Fatalf("open data = %q, want data", data)
	}

	target := filepath.Join(t.TempDir(), "copy.mp4")
	result, err := fs.(remotefs.Downloader).Download(context.Background(), remotefs.DownloadRequest{
		Reference:   remotefs.Reference{Path: "/nested/video.mp4"},
		Destination: target,
		Overwrite:   remotefs.OverwriteDeny,
	}, nil)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if result.Path != target || result.BytesWritten != int64(len("media-data")) {
		t.Fatalf("download result = %+v", result)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "media-data" {
		t.Fatalf("downloaded data = %q, err = %v", got, err)
	}
}

func TestLocalDriverContract(t *testing.T) {
	root := t.TempDir()
	data := []byte("contract-data")
	if err := os.WriteFile(filepath.Join(root, "contract.bin"), data, 0o600); err != nil {
		t.Fatalf("write contract fixture: %v", err)
	}
	fs, err := localdriver.New(localdriver.Config{Root: root})
	if err != nil {
		t.Fatalf("new local: %v", err)
	}
	testsuite.Run(t, testsuite.Fixture{
		FileSystem: fs,
		Root:       remotefs.Reference{Path: "/"},
		File:       remotefs.Reference{Path: "/contract.bin"},
		Data:       data,
	})
}

func TestLocalDriverRejectsTraversalAndSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(root, "escape")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	fs, err := localdriver.New(localdriver.Config{Root: root})
	if err != nil {
		t.Fatalf("new local: %v", err)
	}
	t.Cleanup(func() { _ = fs.Close() })

	for _, path := range []string{"/../secret.txt", "../../secret.txt", "/escape"} {
		if _, err := fs.Stat(context.Background(), remotefs.Reference{Path: path}); !errors.Is(err, remotefs.ErrPermissionDenied) {
			t.Fatalf("stat %q error = %v, want ErrPermissionDenied", path, err)
		}
	}
}

func TestLocalDriverRejectsInternalSymlinksLoopsAndSpecialFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.txt"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("source.txt", filepath.Join(root, "internal-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("loop-b", filepath.Join(root, "loop-a")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("loop-a", filepath.Join(root, "loop-b")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "named-pipe"), 0o600); err != nil {
		t.Fatal(err)
	}
	fs, err := localdriver.New(localdriver.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fs.Close() })
	for _, remotePath := range []string{"/internal-link", "/loop-a"} {
		if _, err := fs.Stat(context.Background(), remotefs.Reference{Path: remotePath}); !errors.Is(err, remotefs.ErrPermissionDenied) {
			t.Fatalf("Stat(%s) error = %v", remotePath, err)
		}
	}
	if _, err := fs.Stat(context.Background(), remotefs.Reference{Path: "/named-pipe"}); !errors.Is(err, remotefs.ErrUnsupported) {
		t.Fatalf("special file error = %v", err)
	}
	if _, err := fs.List(context.Background(), remotefs.Reference{Path: "/"}, remotefs.ListOptions{}); !errors.Is(err, remotefs.ErrPermissionDenied) {
		t.Fatalf("list containing symlink error = %v", err)
	}
}

func TestLocalOpenAndDownloadRejectSpecialFilesAndDirectories(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "rfs-special-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := os.Mkdir(filepath.Join(root, "directory"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "named-pipe"), 0o600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(root, "socket"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	fs, err := localdriver.New(localdriver.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fs.Close() })

	for _, remotePath := range []string{"/directory", "/named-pipe", "/socket"} {
		if _, err := fs.(remotefs.Opener).Open(context.Background(), remotefs.Reference{Path: remotePath}, remotefs.OpenOptions{}); !errors.Is(err, remotefs.ErrNotRegularFile) {
			t.Fatalf("Open(%s) error = %v, want ErrNotRegularFile", remotePath, err)
		}
		destination := filepath.Join(t.TempDir(), filepath.Base(remotePath))
		if _, err := fs.(remotefs.Downloader).Download(context.Background(), remotefs.DownloadRequest{
			Reference: remotefs.Reference{Path: remotePath}, Destination: destination,
		}, nil); !errors.Is(err, remotefs.ErrNotRegularFile) {
			t.Fatalf("Download(%s) error = %v, want ErrNotRegularFile", remotePath, err)
		}
	}
}

func TestLocalDriverRejectsSymlinkRoot(t *testing.T) {
	realRoot := t.TempDir()
	rootLink := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(realRoot, rootLink); err != nil {
		t.Fatal(err)
	}
	if _, err := localdriver.New(localdriver.Config{Root: rootLink}); !errors.Is(err, remotefs.ErrInvalidReference) {
		t.Fatalf("symlink root error = %v", err)
	}
}

func TestLocalDriverPaginationAndContextCancellation(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	fs, err := localdriver.New(localdriver.Config{Root: root})
	if err != nil {
		t.Fatalf("new local: %v", err)
	}
	t.Cleanup(func() { _ = fs.Close() })

	first, err := fs.List(context.Background(), remotefs.Reference{Path: "/"}, remotefs.ListOptions{PageSize: 2})
	if err != nil {
		t.Fatalf("list first: %v", err)
	}
	if len(first.Entries) != 2 || first.Entries[0].Name != "a.txt" || first.Entries[1].Name != "b.txt" || first.NextCursor == "" {
		t.Fatalf("first page = %+v", first)
	}
	second, err := fs.List(context.Background(), remotefs.Reference{Path: "/"}, remotefs.ListOptions{PageSize: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("list second: %v", err)
	}
	if len(second.Entries) != 1 || second.Entries[0].Name != "c.txt" || second.NextCursor != "" {
		t.Fatalf("second page = %+v", second)
	}
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, request := range []struct {
		directory remotefs.Reference
		options   remotefs.ListOptions
	}{
		{directory: remotefs.Reference{Path: "/nested"}, options: remotefs.ListOptions{PageSize: 2, Cursor: first.NextCursor}},
		{directory: remotefs.Reference{Path: "/"}, options: remotefs.ListOptions{PageSize: 2, Cursor: first.NextCursor, SortBy: remotefs.SortBySize}},
	} {
		if _, err := fs.List(context.Background(), request.directory, request.options); !errors.Is(err, remotefs.ErrInvalidOption) {
			t.Fatalf("cross-query cursor error = %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fs.List(ctx, remotefs.Reference{Path: "/"}, remotefs.ListOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled list error = %v", err)
	}
}

func TestLocalDriverOverwritePolicyAndProgressBackpressure(t *testing.T) {
	root := t.TempDir()
	data := bytes.Repeat([]byte("x"), 2*1024*1024)
	if err := os.WriteFile(filepath.Join(root, "large.bin"), data, 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	fs, err := localdriver.New(localdriver.Config{Root: root})
	if err != nil {
		t.Fatalf("new local: %v", err)
	}
	t.Cleanup(func() { _ = fs.Close() })

	target := filepath.Join(t.TempDir(), "large.bin")
	if err := os.WriteFile(target, []byte("existing"), 0o600); err != nil {
		t.Fatalf("write existing: %v", err)
	}
	if _, err := fs.(remotefs.Downloader).Download(context.Background(), remotefs.DownloadRequest{
		Reference:   remotefs.Reference{Path: "/large.bin"},
		Destination: target,
		Overwrite:   remotefs.OverwriteDeny,
	}, nil); !errors.Is(err, remotefs.ErrConflict) {
		t.Fatalf("deny overwrite error = %v", err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := fs.(remotefs.Downloader).Download(context.Background(), remotefs.DownloadRequest{
			Reference:   remotefs.Reference{Path: "/large.bin"},
			Destination: target,
			Overwrite:   remotefs.OverwriteReplace,
		}, remotefs.ProgressSinkFunc(func(_ context.Context, progress remotefs.Progress) error {
			if progress.Phase == remotefs.ProgressTransferring {
				select {
				case <-entered:
				default:
					close(entered)
				}
				<-release
			}
			return nil
		}))
		done <- err
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("progress sink was not called")
	}
	select {
	case err := <-done:
		t.Fatalf("download completed before sink released: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("replace download: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("replaced data length = %d, err = %v", len(got), err)
	}
}

func TestLocalDownloadDenyDoesNotReplaceRacingDestination(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.bin"), []byte("source-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	fs, err := localdriver.New(localdriver.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fs.Close() })
	destination := filepath.Join(t.TempDir(), "destination.bin")

	_, err = fs.(remotefs.Downloader).Download(context.Background(), remotefs.DownloadRequest{
		Reference: remotefs.Reference{Path: "/source.bin"}, Destination: destination, Overwrite: remotefs.OverwriteDeny,
	}, remotefs.ProgressSinkFunc(func(_ context.Context, progress remotefs.Progress) error {
		if progress.Phase == remotefs.ProgressFinalizing {
			return os.WriteFile(destination, []byte("racing-writer"), 0o600)
		}
		return nil
	}))
	if !errors.Is(err, remotefs.ErrConflict) {
		t.Fatalf("Download() error = %v, want ErrConflict", err)
	}
	data, readErr := os.ReadFile(destination)
	if readErr != nil || string(data) != "racing-writer" {
		t.Fatalf("destination = %q, %v", data, readErr)
	}
}

func TestLocalConcurrentOverwriteDenyCommitsOnce(t *testing.T) {
	root := t.TempDir()
	data := bytes.Repeat([]byte("concurrent-source"), 64*1024)
	if err := os.WriteFile(filepath.Join(root, "source.bin"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	fs, err := localdriver.New(localdriver.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fs.Close() })
	destination := filepath.Join(t.TempDir(), "destination.bin")

	errorsOut := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := fs.(remotefs.Downloader).Download(context.Background(), remotefs.DownloadRequest{
				Reference: remotefs.Reference{Path: "/source.bin"}, Destination: destination, Overwrite: remotefs.OverwriteDeny,
			}, nil)
			errorsOut <- err
		}()
	}
	successes := 0
	conflicts := 0
	for range 2 {
		err := <-errorsOut
		switch {
		case err == nil:
			successes++
		case errors.Is(err, remotefs.ErrConflict):
			conflicts++
		default:
			t.Fatalf("Download() error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes = %d, conflicts = %d, want 1/1", successes, conflicts)
	}
	got, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("destination length = %d, err = %v", len(got), err)
	}
}

func TestLocalDownloadReturnsSuccessWithoutDriverCompletedEvent(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.bin"), []byte("source-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	fs, err := localdriver.New(localdriver.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fs.Close() })
	destination := filepath.Join(t.TempDir(), "destination.bin")
	completed := false

	result, err := fs.(remotefs.Downloader).Download(context.Background(), remotefs.DownloadRequest{
		Reference: remotefs.Reference{Path: "/source.bin"}, Destination: destination,
	}, remotefs.ProgressSinkFunc(func(_ context.Context, progress remotefs.Progress) error {
		if progress.Phase == remotefs.ProgressCompleted {
			completed = true
			return errors.New("completed notification failed")
		}
		return nil
	}))
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if completed {
		t.Fatal("driver emitted ProgressCompleted")
	}
	if result.Path != destination || result.BytesWritten != int64(len("source-data")) {
		t.Fatalf("result = %+v", result)
	}
}

func TestLocalRejectsUnsafeDestinationAndNonRegularOpen(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.bin"), []byte("source-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "directory"), 0o700); err != nil {
		t.Fatal(err)
	}
	fs, err := localdriver.New(localdriver.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fs.Close() })

	_, err = fs.(remotefs.Downloader).Download(context.Background(), remotefs.DownloadRequest{
		Reference: remotefs.Reference{Path: "/source.bin"}, Destination: filepath.Join(root, "copy.bin"),
	}, nil)
	if !errors.Is(err, remotefs.ErrPermissionDenied) {
		t.Fatalf("download into source root error = %v", err)
	}
	opener := fs.(remotefs.Opener)
	reader, err := opener.Open(context.Background(), remotefs.Reference{Path: "/directory"}, remotefs.OpenOptions{})
	if reader != nil {
		_ = reader.Close()
	}
	if !errors.Is(err, remotefs.ErrNotRegularFile) {
		t.Fatalf("open directory error = %v", err)
	}
}

func TestLocalRemoveDoesNotFollowSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	link := filepath.Join(root, "link.txt")
	if err := os.WriteFile(target, []byte("keep-target"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	fs, err := localdriver.New(localdriver.Config{Root: root, AllowRemove: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fs.Close() })
	if err := fs.(remotefs.Remover).Remove(context.Background(), remotefs.RemoveRequest{Reference: remotefs.Reference{Path: "/link.txt"}}); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "keep-target" {
		t.Fatalf("target = %q, %v", data, err)
	}
	if _, err := os.Lstat(link); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("link stat error = %v", err)
	}
}

func TestLocalConditionalDownloadAndRemoveRejectChangedSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.bin")
	if err := os.WriteFile(source, bytes.Repeat([]byte("x"), 2*1024*1024), 0o600); err != nil {
		t.Fatal(err)
	}
	fs, err := localdriver.New(localdriver.Config{Root: root, AllowRemove: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fs.Close() })

	destination := filepath.Join(t.TempDir(), "destination.bin")
	_, err = fs.(remotefs.Downloader).Download(context.Background(), remotefs.DownloadRequest{
		Reference: remotefs.Reference{ID: "/different-object", Path: "/source.bin"}, Destination: destination,
	}, nil)
	if !errors.Is(err, remotefs.ErrSourceChanged) {
		t.Fatalf("conditional download error = %v", err)
	}
	if _, statErr := os.Stat(destination); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("destination stat error = %v", statErr)
	}

	err = fs.(remotefs.Remover).Remove(context.Background(), remotefs.RemoveRequest{Reference: remotefs.Reference{ID: "/different-object", Path: "/source.bin"}})
	if !errors.Is(err, remotefs.ErrSourceChanged) {
		t.Fatalf("conditional remove error = %v", err)
	}
	if _, statErr := os.Stat(source); statErr != nil {
		t.Fatalf("source was removed: %v", statErr)
	}
}

func TestLocalDownloadRejectsSourceReplacedDuringTransfer(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.bin")
	if err := os.WriteFile(source, bytes.Repeat([]byte("a"), 2*1024*1024), 0o600); err != nil {
		t.Fatal(err)
	}
	fs, err := localdriver.New(localdriver.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fs.Close() })
	destination := filepath.Join(t.TempDir(), "destination.bin")
	var replaced bool
	snapshot, err := fs.Stat(context.Background(), remotefs.Reference{Path: "/source.bin"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = fs.(remotefs.Downloader).Download(context.Background(), remotefs.DownloadRequest{
		Reference: snapshot.Reference, ExpectedVersion: snapshot.Version, Destination: destination,
	}, remotefs.ProgressSinkFunc(func(_ context.Context, progress remotefs.Progress) error {
		if progress.Phase != remotefs.ProgressTransferring || replaced {
			return nil
		}
		replaced = true
		if err := os.Rename(source, source+".old"); err != nil {
			return err
		}
		return os.WriteFile(source, []byte("replacement"), 0o600)
	}))
	if !errors.Is(err, remotefs.ErrSourceChanged) {
		t.Fatalf("Download() error = %v, want ErrSourceChanged", err)
	}
	if _, statErr := os.Stat(destination); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("destination stat error = %v", statErr)
	}
}

func TestLocalRejectsNilContextAndInvalidListOptions(t *testing.T) {
	fs, err := localdriver.New(localdriver.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fs.Close() })
	if _, err := fs.Stat(nil, remotefs.Reference{Path: "/"}); !errors.Is(err, remotefs.ErrNilContext) {
		t.Fatalf("nil context error = %v", err)
	}
	for _, opts := range []remotefs.ListOptions{
		{PageSize: -1}, {PageSize: 1001}, {SortBy: "invalid"}, {Order: "invalid"},
	} {
		if _, err := fs.List(context.Background(), remotefs.Reference{Path: "/"}, opts); !errors.Is(err, remotefs.ErrInvalidOption) {
			t.Fatalf("List(%+v) error = %v", opts, err)
		}
	}
}

func TestLocalDriverDoesNotFollowPreexistingPartialSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.bin"), []byte("source-data"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	fs, err := localdriver.New(localdriver.Config{Root: root})
	if err != nil {
		t.Fatalf("new local: %v", err)
	}
	t.Cleanup(func() { _ = fs.Close() })
	destination := filepath.Join(t.TempDir(), "destination.bin")
	outside := filepath.Join(t.TempDir(), "outside.bin")
	if err := os.WriteFile(outside, []byte("do-not-change"), 0o600); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	if err := os.Symlink(outside, destination+".partial"); err != nil {
		t.Fatalf("create partial symlink: %v", err)
	}
	_, err = fs.(remotefs.Downloader).Download(context.Background(), remotefs.DownloadRequest{
		Reference: remotefs.Reference{Path: "/source.bin"}, Destination: destination,
	}, nil)
	if !errors.Is(err, remotefs.ErrConflict) {
		t.Fatalf("download error = %v", err)
	}
	data, err := os.ReadFile(outside)
	if err != nil {
		t.Fatalf("read outside: %v", err)
	}
	if string(data) != "do-not-change" {
		t.Fatalf("outside content = %q", data)
	}
}

func TestLocalRemoveRequiresExplicitEnablement(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "source.txt")
	if err := os.WriteFile(file, []byte("data"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	fs, err := localdriver.New(localdriver.Config{Root: root})
	if err != nil {
		t.Fatalf("new local: %v", err)
	}
	if _, ok := fs.(remotefs.Remover); ok {
		t.Fatal("disabled local driver unexpectedly implements Remover")
	}

	enabled, err := localdriver.New(localdriver.Config{Root: root, AllowRemove: true})
	if err != nil {
		t.Fatalf("new enabled local: %v", err)
	}
	remover, ok := enabled.(remotefs.Remover)
	if !ok {
		t.Fatal("enabled local driver does not report Remove capability")
	}
	if err := remover.Remove(context.Background(), remotefs.RemoveRequest{Reference: remotefs.Reference{Path: "/source.txt"}}); err != nil {
		t.Fatalf("remove enabled: %v", err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed file stat error = %v", err)
	}
}
