// Package testsuite provides reusable RemoteFS driver contract assertions.
package testsuite

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/huwenlong92/sdkit/pkg/remotefs"
)

type Fixture struct {
	FileSystem remotefs.FileSystem
	Root       remotefs.Reference
	File       remotefs.Reference
	Data       []byte
}

func Run(t *testing.T, fixture Fixture) {
	t.Helper()
	ctx := context.Background()
	entry, err := fixture.FileSystem.Stat(ctx, fixture.File)
	if err != nil {
		t.Fatalf("contract Stat: %v", err)
	}
	if entry.Type != remotefs.EntryFile || entry.Reference.Path != fixture.File.Path || entry.Size != int64(len(fixture.Data)) {
		t.Fatalf("contract Stat entry = %+v", entry)
	}

	page, err := fixture.FileSystem.List(ctx, fixture.Root, remotefs.ListOptions{PageSize: 100})
	if err != nil {
		t.Fatalf("contract List: %v", err)
	}
	found := false
	for _, listed := range page.Entries {
		if listed.Reference.Path == fixture.File.Path {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("contract List did not contain %q: %+v", fixture.File.Path, page.Entries)
	}

	target := filepath.Join(t.TempDir(), "downloaded.bin")
	downloader, ok := fixture.FileSystem.(remotefs.Downloader)
	if !ok {
		t.Fatal("contract filesystem does not implement Downloader")
	}
	result, err := downloader.Download(ctx, remotefs.DownloadRequest{
		Reference:   fixture.File,
		Destination: target,
		Overwrite:   remotefs.OverwriteDeny,
	}, nil)
	if err != nil {
		t.Fatalf("contract Download: %v", err)
	}
	if result.Path != target || result.BytesWritten != int64(len(fixture.Data)) {
		t.Fatalf("contract Download result = %+v", result)
	}
	file, err := os.Open(target)
	if err != nil {
		t.Fatalf("contract open downloaded file: %v", err)
	}
	got, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("contract read/close downloaded file: %v / %v", readErr, closeErr)
	}
	if string(got) != string(fixture.Data) {
		t.Fatalf("contract downloaded data = %q, want %q", got, fixture.Data)
	}
	if err := fixture.FileSystem.Close(); err != nil {
		t.Fatalf("contract Close: %v", err)
	}
	if err := fixture.FileSystem.Close(); err != nil {
		t.Fatalf("contract second Close: %v", err)
	}
}
