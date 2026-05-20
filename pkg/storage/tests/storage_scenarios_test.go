package tests

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/pkg/storage"
	"github.com/huwenlong92/sdkit/pkg/storage/core"
)

var errAfterUpload = errors.New("after upload failed")

func TestUploadStreamRunsHooksAndValidation(t *testing.T) {
	dir := t.TempDir()
	fs, err := storage.NewFromPolicy(
		core.StoragePolicy{Driver: "local", LocalDir: dir},
		storage.WithMaxSize(32),
		storage.WithAllowedExtensions(".txt"),
	)
	if err != nil {
		t.Fatal(err)
	}
	var afterCalled bool

	result := fs.UploadStreamWithHook(context.Background(), strings.NewReader("hello"), core.FileInfo{Name: "a.txt", Size: 5},
		storage.AfterUpload(func(ctx context.Context, event storage.Event) error {
			afterCalled = true
			if event.File.Path == "" {
				t.Fatal("path should be generated before AfterUpload")
			}
			return nil
		}),
	)
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	if !afterCalled {
		t.Fatal("AfterUpload hook should be called")
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(result.File.Path))); err != nil {
		t.Fatal(err)
	}

	result = fs.UploadStream(context.Background(), strings.NewReader("bad"), core.FileInfo{Name: "a.exe", Size: 3})
	if result.Error == nil {
		t.Fatal("extension validation should reject .exe")
	}
}

func TestUploadStreamReturnsFileWhenAfterHookFails(t *testing.T) {
	dir := t.TempDir()
	fs, err := storage.NewFromPolicy(core.StoragePolicy{Driver: "local", LocalDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	result := fs.UploadStreamWithHook(context.Background(), strings.NewReader("hello"), core.FileInfo{Name: "a.txt", Size: 5},
		storage.AfterUpload(func(ctx context.Context, event storage.Event) error {
			return errAfterUpload
		}),
	)
	if !errors.Is(result.Error, errAfterUpload) {
		t.Fatalf("error = %v, want %v", result.Error, errAfterUpload)
	}
	if !result.Uploaded || result.File.Path == "" {
		t.Fatalf("upload result should keep uploaded file: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(result.File.Path))); err != nil {
		t.Fatal(err)
	}
}

func TestSourceUsesPublicURLPrefix(t *testing.T) {
	fs, err := storage.NewFromPolicy(core.StoragePolicy{Driver: "local", LocalDir: t.TempDir(), PublicURL: "https://static.example.com/files", CDNURL: "https://cdn.example.com/assets"})
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

func TestBackendOperationHooks(t *testing.T) {
	dir := t.TempDir()
	fs, err := storage.NewFromPolicy(core.StoragePolicy{Driver: "local", LocalDir: dir, PublicURL: "https://static.example.com/files"})
	if err != nil {
		t.Fatal(err)
	}
	upload := fs.UploadStream(context.Background(), strings.NewReader("hello"), core.FileInfo{Name: "ops.txt", Path: "ops.txt", Size: 5})
	if upload.Error != nil {
		t.Fatal(upload.Error)
	}

	var listCalled bool
	list := fs.ListWithHook(context.Background(), "",
		storage.AfterList(func(ctx context.Context, event storage.Event) error {
			listCalled = len(event.Objects) > 0
			return nil
		}),
	)
	if list.Error != nil {
		t.Fatal(list.Error)
	}
	if !listCalled || len(list.Objects) == 0 {
		t.Fatalf("ListWithHook should expose objects: %+v", list)
	}

	var sourceCalled bool
	source := fs.SourceWithHook(context.Background(), "ops.txt", 0,
		storage.AfterSource(func(ctx context.Context, event storage.Event) error {
			sourceCalled = strings.Contains(event.Source, "ops.txt")
			return nil
		}),
	)
	if source.Error != nil {
		t.Fatal(source.Error)
	}
	if !sourceCalled || !strings.Contains(source.Source, "ops.txt") {
		t.Fatalf("SourceWithHook should expose source: %+v", source)
	}

	var tokenCalled bool
	token := fs.TokenWithHook(context.Background(), core.FileInfo{Name: "token.txt"}, time.Minute,
		storage.AfterToken(func(ctx context.Context, event storage.Event) error {
			tokenCalled = event.Credential != nil && event.Credential.Path != ""
			return nil
		}),
	)
	if token.Error != nil {
		t.Fatal(token.Error)
	}
	if !tokenCalled || token.Credential == nil || token.Credential.Path == "" || token.Credential.Mode != core.UploadModeLocalChunk {
		t.Fatalf("TokenWithHook should expose credential: %+v", token)
	}

	var deleteCalled bool
	deleted := fs.DeleteWithHook(context.Background(), []string{"ops.txt"},
		storage.AfterDelete(func(ctx context.Context, event storage.Event) error {
			deleteCalled = len(event.Paths) == 1 && event.Paths[0] == "ops.txt"
			return nil
		}),
	)
	if deleted.Error != nil {
		t.Fatal(deleted.Error)
	}
	if !deleteCalled || !deleted.Deleted {
		t.Fatalf("DeleteWithHook should expose paths: %+v", deleted)
	}
	if _, err := os.Stat(filepath.Join(dir, "ops.txt")); !os.IsNotExist(err) {
		t.Fatalf("file should be deleted, stat err=%v", err)
	}
}

func TestUnifiedPolicyConfigMapsToDriverConfig(t *testing.T) {
	dir := t.TempDir()
	fs, err := storage.New(&core.Config{
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

func TestR2SourceUsesS3CompatiblePresignedURL(t *testing.T) {
	fs, err := storage.NewFromPolicy(core.StoragePolicy{
		Driver:    "r2",
		Bucket:    "assets",
		Endpoint:  "https://account-id.r2.cloudflarestorage.com",
		AccessKey: "r2-access-key",
		SecretKey: "r2-secret-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	source, err := fs.Source("avatars/a.png", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "account-id.r2.cloudflarestorage.com/assets/avatars/a.png") {
		t.Fatalf("source should use R2 path-style endpoint: %s", source)
	}
	if !strings.Contains(source, "X-Amz-Signature=") || !strings.Contains(source, "auto") {
		t.Fatalf("source should be a region auto presigned URL: %s", source)
	}
}

func TestNewFromPolicyUsesPolicyOnly(t *testing.T) {
	dir := t.TempDir()
	fs, err := storage.NewFromPolicy(
		core.StoragePolicy{
			Driver:   "local",
			LocalDir: dir,
		},
		storage.WithUploadDir("files"),
		storage.WithNameRules("{date}", "{originname}{ext}"),
		storage.WithAllowedExtensions(".txt"),
	)
	if err != nil {
		t.Fatal(err)
	}
	result := fs.UploadStream(context.Background(), strings.NewReader("policy"), core.FileInfo{Name: "policy.txt", Size: 6})
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	if !strings.HasPrefix(result.File.Path, "files/") {
		t.Fatalf("path = %q, want files/ prefix", result.File.Path)
	}
}

func TestLocalChunkUploadRunsUploadHooks(t *testing.T) {
	dir := t.TempDir()
	fs, err := storage.NewFromPolicy(
		core.StoragePolicy{Driver: "local", LocalDir: dir},
		storage.WithChunkSize(3),
	)
	if err != nil {
		t.Fatal(err)
	}
	var afterCount int
	var beforeInitCalled bool
	afterUpload := storage.AfterUpload(func(ctx context.Context, event storage.Event) error {
		afterCount++
		return nil
	})

	cred, err := fs.InitUploadWithHook(context.Background(), storage.UploadInitRequest{
		FileName:  "chunk.txt",
		TotalSize: 6,
	}, storage.BeforeUpload(func(ctx context.Context, event storage.Event) error {
		beforeInitCalled = event.File.Name == "chunk.txt"
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !beforeInitCalled {
		t.Fatal("InitUploadWithHook should run upload hooks")
	}
	if cred.UploadID == "" || cred.ChunkNum != 2 {
		t.Fatalf("unexpected credential: %+v", cred)
	}
	if _, err := fs.UploadChunk(context.Background(), cred.UploadID, 0, strings.NewReader("abc")); err != nil {
		t.Fatal(err)
	}
	result, err := fs.UploadChunkWithHook(context.Background(), cred.UploadID, 1, strings.NewReader("def"), afterUpload)
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
	first, err := storage.NewFromPolicy(policy, storage.WithChunkSize(3))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := storage.NewFromPolicy(policy, storage.WithChunkSize(3))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	cred, err := first.InitUpload(context.Background(), storage.UploadInitRequest{
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
	fs, err := storage.NewFromPolicy(core.StoragePolicy{Driver: "local", LocalDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	upload := fs.UploadStream(context.Background(), strings.NewReader("download-me"), core.FileInfo{Name: "d.txt", Path: "d.txt", Size: 11})
	if upload.Error != nil {
		t.Fatal(upload.Error)
	}
	var out bytes.Buffer
	var progressCalled bool
	download := fs.Download(context.Background(), "d.txt", &out, func(downloaded, total int64) {
		progressCalled = true
		if downloaded <= 0 {
			t.Fatal("downloaded should be positive")
		}
	})
	if download.Error != nil {
		t.Fatal(download.Error)
	}
	if out.String() != "download-me" || !progressCalled {
		t.Fatalf("download out=%q progress=%v", out.String(), progressCalled)
	}

	var getHookCalled bool
	getResult := fs.GetWithHook(context.Background(), "d.txt",
		storage.AfterGet(func(ctx context.Context, event storage.Event) error {
			getHookCalled = event.File.Path == "d.txt" && event.Reader != nil
			return nil
		}),
	)
	if getResult.Error != nil {
		t.Fatal(getResult.Error)
	}
	_ = getResult.Reader.Close()
	if !getHookCalled {
		t.Fatal("GetWithHook should run get hooks")
	}

	var downloadHookCalled bool
	out.Reset()
	download = fs.DownloadWithHook(context.Background(), "d.txt", &out, nil,
		storage.AfterDownload(func(ctx context.Context, event storage.Event) error {
			downloadHookCalled = event.File.Path == "d.txt"
			return nil
		}),
	)
	if download.Error != nil {
		t.Fatal(download.Error)
	}
	if out.String() != "download-me" || !downloadHookCalled {
		t.Fatalf("download with hook out=%q hook=%v", out.String(), downloadHookCalled)
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

	var urlHookCalled bool
	remoteUpload := fs.UploadFromURLWithHook(context.Background(), "https://example.com/remote.txt", core.FileInfo{},
		storage.AfterUpload(func(ctx context.Context, event storage.Event) error {
			urlHookCalled = event.File.Name == "remote.txt"
			return nil
		}),
	)
	if remoteUpload.Error != nil {
		t.Fatal(remoteUpload.Error)
	}
	if !urlHookCalled {
		t.Fatal("UploadFromURLWithHook should run upload hooks")
	}
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(remoteUpload.File.Path)))
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

	info, err := storage.DecodeImageInfo(bytes.NewReader(source.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 4 || info.Height != 4 || info.Format != "png" {
		t.Fatalf("unexpected image info: %+v", info)
	}
	if err := storage.ValidateImageInfo(info, storage.ImageLimit{MaxWidth: 8, MaxHeight: 8}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	fs, err := storage.NewFromPolicy(core.StoragePolicy{Driver: "local", LocalDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var cropHookCalled bool
	uploaded := fs.UploadCroppedImageWithHook(context.Background(), bytes.NewReader(source.Bytes()), core.FileInfo{Name: "crop.png"}, storage.CropRect{X: 1, Y: 1, Width: 2, Height: 2}, "png", 0,
		storage.AfterUpload(func(ctx context.Context, event storage.Event) error {
			cropHookCalled = event.File.Name == "crop.png"
			return nil
		}),
	)
	if uploaded.Error != nil {
		t.Fatal(uploaded.Error)
	}
	if !cropHookCalled {
		t.Fatal("UploadCroppedImageWithHook should run upload hooks")
	}
	file, err := os.Open(filepath.Join(dir, filepath.FromSlash(uploaded.File.Path)))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	cropped, err := storage.DecodeImageInfo(file)
	if err != nil {
		t.Fatal(err)
	}
	if cropped.Width != 2 || cropped.Height != 2 {
		t.Fatalf("cropped image info: %+v", cropped)
	}
}
