//go:build sdkit_storage_s3

package tests

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/pkg/storage"
	"github.com/huwenlong92/sdkit/pkg/storage/core"
	_ "github.com/huwenlong92/sdkit/pkg/storage/driver/s3"
)

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
	cred, err := fs.Token(core.FileInfo{Name: "b.png", Path: "avatars/b.png", Size: 1024}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if cred.Mode != core.UploadModeDirectPut || len(cred.UploadURLs) != 1 {
		t.Fatalf("expected direct put credential: %+v", cred)
	}
	if !strings.Contains(cred.UploadURLs[0], "account-id.r2.cloudflarestorage.com/assets/avatars/b.png") {
		t.Fatalf("upload url should use R2 path-style endpoint: %s", cred.UploadURLs[0])
	}
	if !strings.Contains(cred.UploadURLs[0], "X-Amz-Signature=") || !strings.Contains(cred.UploadURLs[0], "auto") {
		t.Fatalf("upload url should be a region auto presigned URL: %s", cred.UploadURLs[0])
	}
}

func TestS3UploadStreamCanRetrySeekableBody(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/assets/runs/input.csv" {
			t.Errorf("path = %s, want /assets/runs/input.csv", r.URL.Path)
		}
		current := atomic.AddInt32(&attempts, 1)
		if current == 1 {
			buf := make([]byte, 1)
			_, _ = r.Body.Read(buf)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body error = %v", err)
		}
		if string(body) != "seekable upload payload" {
			t.Errorf("body = %q, want seekable upload payload", body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tmp, err := os.CreateTemp(t.TempDir(), "s3-upload-*")
	if err != nil {
		t.Fatal(err)
	}
	defer tmp.Close()
	if _, err := tmp.WriteString("seekable upload payload"); err != nil {
		t.Fatal(err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	fs, err := storage.NewFromPolicy(core.StoragePolicy{
		Driver:    "minio",
		Bucket:    "assets",
		Endpoint:  server.URL,
		Region:    "us-east-1",
		AccessKey: "access-key",
		SecretKey: "secret-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	var uploadedBytes int64
	result := fs.UploadStream(t.Context(), tmp, core.FileInfo{
		Name: "input.csv",
		Path: "runs/input.csv",
		Size: int64(len("seekable upload payload")),
		Progress: func(uploaded, total int64) {
			if total != int64(len("seekable upload payload")) {
				t.Errorf("progress total = %d, want %d", total, len("seekable upload payload"))
			}
			atomic.StoreInt64(&uploadedBytes, uploaded)
		},
	})
	if result.Error != nil {
		t.Fatalf("upload stream error = %v", result.Error)
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
	if got := atomic.LoadInt64(&uploadedBytes); got != int64(len("seekable upload payload")) {
		t.Fatalf("uploaded bytes = %d, want %d", got, len("seekable upload payload"))
	}
}
