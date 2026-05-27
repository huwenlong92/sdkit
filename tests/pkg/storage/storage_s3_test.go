//go:build sdkit_storage_s3

package tests

import (
	"strings"
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
