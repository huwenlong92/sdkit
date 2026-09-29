//go:build sdkit_storage_cos

package tests

import (
	"bytes"
	"fmt"
	"hash/crc64"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/huwenlong92/sdkit/pkg/storage"
	"github.com/huwenlong92/sdkit/pkg/storage/core"
	_ "github.com/huwenlong92/sdkit/pkg/storage/driver/cos"
)

func TestCOSMultipartProgressAdvancesAfterPartSuccess(t *testing.T) {
	const partSize = int64(1 << 20)
	total := partSize + 7
	var uploaded int64
	var partRequests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch {
		case r.Method == http.MethodPost && query.Has("uploads"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprint(w, `<InitiateMultipartUploadResult><Bucket>assets</Bucket><Key>runs/cos.zip</Key><UploadId>cos-upload</UploadId></InitiateMultipartUploadResult>`)
		case r.Method == http.MethodPut && query.Get("uploadId") == "cos-upload":
			partNumber := query.Get("partNumber")
			checksum := crc64.New(crc64.MakeTable(crc64.ECMA))
			if _, err := io.Copy(checksum, r.Body); err != nil {
				t.Errorf("read part %s: %v", partNumber, err)
			}
			expected := int64(0)
			if partNumber == "2" {
				expected = partSize
			}
			if got := atomic.LoadInt64(&uploaded); got != expected {
				t.Errorf("progress before part %s response = %d, want %d", partNumber, got, expected)
			}
			atomic.AddInt32(&partRequests, 1)
			w.Header().Set("ETag", fmt.Sprintf(`"cos-%s"`, partNumber))
			w.Header().Set("x-cos-hash-crc64ecma", fmt.Sprintf("%d", checksum.Sum64()))
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && query.Get("uploadId") == "cos-upload":
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprint(w, `<CompleteMultipartUploadResult><Bucket>assets</Bucket><Key>runs/cos.zip</Key><ETag>"complete"</ETag></CompleteMultipartUploadResult>`)
		case r.Method == http.MethodDelete && query.Get("uploadId") == "cos-upload":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	fs, err := storage.NewFromPolicy(core.StoragePolicy{
		Driver: "cos", Bucket: "assets", Endpoint: server.URL, AccessKey: "secret-id", SecretKey: "secret-key",
	}, storage.WithChunkSize(partSize))
	if err != nil {
		t.Fatal(err)
	}
	result := fs.UploadStream(t.Context(), bytes.NewReader(bytes.Repeat([]byte("x"), int(total))), core.FileInfo{
		Name: "cos.zip", Path: "runs/cos.zip", Size: total,
		Progress: func(current, progressTotal int64) {
			if progressTotal != total {
				t.Errorf("progress total = %d, want %d", progressTotal, total)
			}
			atomic.StoreInt64(&uploaded, current)
		},
	})
	if result.Error != nil {
		t.Fatalf("upload stream error = %v", result.Error)
	}
	if got := atomic.LoadInt32(&partRequests); got != 2 {
		t.Fatalf("part requests = %d, want 2", got)
	}
	if got := atomic.LoadInt64(&uploaded); got != total {
		t.Fatalf("uploaded = %d, want %d", got, total)
	}
}
