package tests

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/huwenlong92/sdkit/pkg/filesystem"
	"github.com/huwenlong92/sdkit/pkg/filesystem/core"
)

// TestUploadWithProgress 演示服务端上传 + 进度回调
func TestUploadWithProgress(t *testing.T) {
	dir := t.TempDir()
	fs, err := filesystem.NewFromPolicy(core.StoragePolicy{Driver: "local", LocalDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	data := "Hello from sdkitgo filesystem!"
	var progressCalled bool

	info := core.FileInfo{
		Name: "test_upload.txt",
		Path: "test_upload.txt",
		Size: int64(len(data)),
		Progress: func(uploaded, total int64) {
			progressCalled = true
			t.Logf("进度: %d/%d (%.0f%%)", uploaded, total, float64(uploaded)/float64(total)*100)
		},
	}

	if err := fs.Put(core.NewFileStream(strings.NewReader(data), info)); err != nil {
		t.Fatal(err)
	}

	if !progressCalled {
		t.Error("进度回调未被调用")
	}

	// 下载验证
	reader, err := fs.Get("test_upload.txt")
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len(data))
	n, _ := reader.Read(buf)
	reader.Close()
	if got := string(buf[:n]); got != data {
		t.Errorf("内容不匹配: got %q, want %q", got, data)
	}

	// 验证文件确实写入了临时目录
	if _, err := os.Stat(dir + "/test_upload.txt"); err != nil {
		t.Errorf("文件应该存在: %v", err)
	}
	fmt.Println("上传进度测试通过")
}
