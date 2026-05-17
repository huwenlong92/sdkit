package tests

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/huwenlong92/sdkit/pkg/filesystem"
	"github.com/huwenlong92/sdkit/pkg/filesystem/core"
)

func TestUploadStreamRunsHooksAndValidation(t *testing.T) {
	dir := t.TempDir()
	fs, err := filesystem.NewFromPolicy(
		core.StoragePolicy{Driver: "local", LocalDir: dir},
		filesystem.WithMaxSize(32),
		filesystem.WithAllowedExtensions(".txt"),
	)
	if err != nil {
		t.Fatal(err)
	}
	var afterCalled bool
	if err := fs.Use(filesystem.HookAfterUpload, func(ctx context.Context, fs *filesystem.FileSystem, file core.FileInfo) error {
		afterCalled = true
		if file.Path == "" {
			t.Fatal("path should be generated before AfterUpload")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	info, err := fs.UploadStream(context.Background(), strings.NewReader("hello"), core.FileInfo{Name: "a.txt", Size: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !afterCalled {
		t.Fatal("AfterUpload hook should be called")
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(info.Path))); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.UploadStream(context.Background(), strings.NewReader("bad"), core.FileInfo{Name: "a.exe", Size: 3}); err == nil {
		t.Fatal("extension validation should reject .exe")
	}
}

func TestSourceUsesPublicURLPrefix(t *testing.T) {
	fs, err := filesystem.NewFromPolicy(core.StoragePolicy{Driver: "local", LocalDir: t.TempDir(), PublicURL: "https://static.example.com/files", CDNURL: "https://cdn.example.com/assets"})
	if err != nil {
		t.Fatal(err)
	}
	url, err := fs.Source("uploads/a.txt", 0)
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://cdn.example.com/assets/uploads/a.txt" {
		t.Fatalf("source url = %q", url)
	}
}

func TestUnifiedPolicyConfigMapsToDriverConfig(t *testing.T) {
	dir := t.TempDir()
	fs, err := filesystem.New(&core.Config{
		Policy: core.StoragePolicy{
			Driver:   "local",
			LocalDir: dir,
			CDNURL:   "https://cdn.example.com/fs",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := fs.Config()
	if cfg.Policy.Driver != "local" || cfg.Policy.LocalDir != dir || cfg.Policy.CDNURL != "https://cdn.example.com/fs" {
		t.Fatalf("policy was not mapped: %+v", cfg)
	}
}

func TestNewFromPolicyUsesPolicyOnly(t *testing.T) {
	dir := t.TempDir()
	fs, err := filesystem.NewFromPolicy(
		core.StoragePolicy{
			Driver:   "local",
			LocalDir: dir,
		},
		filesystem.WithUploadDir("files"),
		filesystem.WithNameRules("{date}", "{originname}{ext}"),
		filesystem.WithAllowedExtensions(".txt"),
	)
	if err != nil {
		t.Fatal(err)
	}
	info, err := fs.UploadStream(context.Background(), strings.NewReader("policy"), core.FileInfo{Name: "policy.txt", Size: 6})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(info.Path, "files/") {
		t.Fatalf("path = %q, want files/ prefix", info.Path)
	}
}

func TestLocalChunkUploadRunsUploadHooks(t *testing.T) {
	dir := t.TempDir()
	fs, err := filesystem.NewFromPolicy(
		core.StoragePolicy{Driver: "local", LocalDir: dir},
		filesystem.WithChunkSize(3),
	)
	if err != nil {
		t.Fatal(err)
	}
	var afterCount int
	if err := fs.Use(filesystem.HookAfterUpload, func(ctx context.Context, fs *filesystem.FileSystem, file core.FileInfo) error {
		afterCount++
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	cred, err := fs.InitUpload(context.Background(), filesystem.UploadInitRequest{
		FileName:  "chunk.txt",
		TotalSize: 6,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cred.UploadID == "" || cred.ChunkNum != 2 {
		t.Fatalf("unexpected credential: %+v", cred)
	}
	if _, err := fs.UploadChunk(context.Background(), cred.UploadID, 0, strings.NewReader("abc")); err != nil {
		t.Fatal(err)
	}
	result, err := fs.UploadChunk(context.Background(), cred.UploadID, 1, strings.NewReader("def"))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Done || result.FilePath == "" {
		t.Fatalf("chunk upload should be complete: %+v", result)
	}
	if afterCount != 1 {
		t.Fatalf("AfterUpload hook count = %d, want 1", afterCount)
	}
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(result.FilePath)))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "abcdef" {
		t.Fatalf("merged data = %q", data)
	}
}

func TestLocalChunkUploadSessionBelongsToInstance(t *testing.T) {
	dir := t.TempDir()
	policy := core.StoragePolicy{Driver: "local", LocalDir: dir}
	first, err := filesystem.NewFromPolicy(policy, filesystem.WithChunkSize(3))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := filesystem.NewFromPolicy(policy, filesystem.WithChunkSize(3))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	cred, err := first.InitUpload(context.Background(), filesystem.UploadInitRequest{
		FileName:  "chunk-instance.txt",
		TotalSize: 6,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.UploadChunk(context.Background(), cred.UploadID, 0, strings.NewReader("abc")); err == nil {
		t.Fatal("another FileSystem instance should not see the upload session")
	}
	if _, err := first.UploadChunk(context.Background(), cred.UploadID, 0, strings.NewReader("abc")); err != nil {
		t.Fatal(err)
	}
	result, err := first.UploadChunk(context.Background(), cred.UploadID, 1, strings.NewReader("def"))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Done || result.FilePath == "" {
		t.Fatalf("chunk upload should complete on original instance: %+v", result)
	}
}

func TestDownloadProgressAndUploadFromURL(t *testing.T) {
	dir := t.TempDir()
	fs, err := filesystem.NewFromPolicy(core.StoragePolicy{Driver: "local", LocalDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.UploadStream(context.Background(), strings.NewReader("download-me"), core.FileInfo{Name: "d.txt", Path: "d.txt", Size: 11}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	var progressCalled bool
	if err := fs.Download(context.Background(), "d.txt", &out, func(downloaded, total int64) {
		progressCalled = true
		if downloaded <= 0 {
			t.Fatal("downloaded should be positive")
		}
	}); err != nil {
		t.Fatal(err)
	}
	if out.String() != "download-me" || !progressCalled {
		t.Fatalf("download out=%q progress=%v", out.String(), progressCalled)
	}

	oldClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"text/plain"}},
			Body:          io.NopCloser(strings.NewReader("from-url")),
			ContentLength: 7,
			Request:       req,
		}, nil
	})}
	defer func() { http.DefaultClient = oldClient }()

	info, err := fs.UploadFromURL(context.Background(), "https://example.com/remote.txt", core.FileInfo{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(info.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "from-url" {
		t.Fatalf("url upload data = %q", data)
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestImageInfoAndCropUpload(t *testing.T) {
	var source bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 40), G: uint8(y * 40), A: 255})
		}
	}
	if err := png.Encode(&source, img); err != nil {
		t.Fatal(err)
	}

	info, err := filesystem.DecodeImageInfo(bytes.NewReader(source.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 4 || info.Height != 4 || info.Format != "png" {
		t.Fatalf("unexpected image info: %+v", info)
	}
	if err := filesystem.ValidateImageInfo(info, filesystem.ImageLimit{MaxWidth: 8, MaxHeight: 8}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	fs, err := filesystem.NewFromPolicy(core.StoragePolicy{Driver: "local", LocalDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	uploaded, err := fs.UploadCroppedImage(context.Background(), bytes.NewReader(source.Bytes()), core.FileInfo{Name: "crop.png"}, filesystem.CropRect{X: 1, Y: 1, Width: 2, Height: 2}, "png", 0)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(filepath.Join(dir, filepath.FromSlash(uploaded.Path)))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	cropped, err := filesystem.DecodeImageInfo(file)
	if err != nil {
		t.Fatal(err)
	}
	if cropped.Width != 2 || cropped.Height != 2 {
		t.Fatalf("cropped image info: %+v", cropped)
	}
}
