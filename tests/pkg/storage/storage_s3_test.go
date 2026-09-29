//go:build sdkit_storage_s3

package tests

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/pkg/storage"
	"github.com/huwenlong92/sdkit/pkg/storage/core"
	_ "github.com/huwenlong92/sdkit/pkg/storage/driver/s3"
)

func TestCUCloudUsesS3CompatiblePresignedURLs(t *testing.T) {
	fs, err := storage.NewFromPolicy(core.StoragePolicy{
		Driver:        "cucloud",
		Bucket:        "assets",
		Endpoint:      "https://oss.example.cucloud.cn",
		EndpointInner: "https://oss-internal.example.cucloud.cn",
		Region:        "example-region",
		AccessKey:     "access-key",
		SecretKey:     "secret-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	source, err := fs.Source("avatars/a.png", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "assets.oss.example.cucloud.cn/avatars/a.png") {
		t.Fatalf("source should use public endpoint: %s", source)
	}
	if !strings.Contains(source, "X-Amz-Signature=") || !strings.Contains(source, "example-region") {
		t.Fatalf("source should be a region-signed URL: %s", source)
	}
	cred, err := fs.Token(core.FileInfo{Name: "b.png", Path: "avatars/b.png", Size: 1024}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if cred.Mode != core.UploadModeDirectPut || len(cred.UploadURLs) != 1 {
		t.Fatalf("expected direct put credential: %+v", cred)
	}
	if !strings.Contains(cred.UploadURLs[0], "assets.oss.example.cucloud.cn/avatars/b.png") || strings.Contains(cred.UploadURLs[0], "oss-internal") {
		t.Fatalf("upload URL should use public endpoint: %s", cred.UploadURLs[0])
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

func TestS3LargeUploadReportsCommittedMultipartProgress(t *testing.T) {
	const partSize = int64(16 << 20)
	total := partSize + 7
	var uploadedBytes int64
	var partRequests int32
	var progressMu sync.Mutex
	progressValues := make([]int64, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch {
		case r.Method == http.MethodPost && query.Has("uploads"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprint(w, `<InitiateMultipartUploadResult><Bucket>assets</Bucket><Key>runs/large.zip</Key><UploadId>upload-1</UploadId></InitiateMultipartUploadResult>`)
		case r.Method == http.MethodPut && query.Get("uploadId") == "upload-1":
			partNumber := query.Get("partNumber")
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read part %s: %v", partNumber, err)
			}
			expectedBeforeResponse := int64(0)
			if partNumber == "2" {
				expectedBeforeResponse = partSize
			}
			if got := atomic.LoadInt64(&uploadedBytes); got != expectedBeforeResponse {
				t.Errorf("progress before part %s response = %d, want %d", partNumber, got, expectedBeforeResponse)
			}
			if partNumber == "1" && int64(len(body)) != partSize {
				t.Errorf("part 1 size = %d, want %d", len(body), partSize)
			}
			if partNumber == "2" && int64(len(body)) != total-partSize {
				t.Errorf("part 2 size = %d, want %d", len(body), total-partSize)
			}
			atomic.AddInt32(&partRequests, 1)
			w.Header().Set("ETag", fmt.Sprintf(`"etag-%s"`, partNumber))
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && query.Get("uploadId") == "upload-1":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read complete body: %v", err)
			}
			if !bytes.Contains(body, []byte("etag-1")) || !bytes.Contains(body, []byte("etag-2")) {
				t.Errorf("complete body missing parts: %s", body)
			}
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprint(w, `<CompleteMultipartUploadResult><Bucket>assets</Bucket><Key>runs/large.zip</Key><ETag>"complete"</ETag></CompleteMultipartUploadResult>`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	content := bytes.Repeat([]byte("x"), int(total))
	fs, err := storage.NewFromPolicy(core.StoragePolicy{
		Driver: "minio", Bucket: "assets", Endpoint: server.URL, Region: "us-east-1",
		AccessKey: "access-key", SecretKey: "secret-key",
	}, storage.WithChunkSize(partSize))
	if err != nil {
		t.Fatal(err)
	}
	result := fs.UploadStream(t.Context(), bytes.NewReader(content), core.FileInfo{
		Name: "large.zip", Path: "runs/large.zip", Size: total,
		Progress: func(uploaded, progressTotal int64) {
			if progressTotal != total {
				t.Errorf("progress total = %d, want %d", progressTotal, total)
			}
			atomic.StoreInt64(&uploadedBytes, uploaded)
			progressMu.Lock()
			progressValues = append(progressValues, uploaded)
			progressMu.Unlock()
		},
	})
	if result.Error != nil {
		t.Fatalf("upload stream error = %v", result.Error)
	}
	if got := atomic.LoadInt32(&partRequests); got != 2 {
		t.Fatalf("part requests = %d, want 2", got)
	}
	progressMu.Lock()
	defer progressMu.Unlock()
	if len(progressValues) != 2 || progressValues[0] != partSize || progressValues[1] != total {
		t.Fatalf("progress values = %v, want [%d %d]", progressValues, partSize, total)
	}
}
