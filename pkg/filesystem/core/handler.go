package core

import (
	"io"
	"time"
)

// Handler 文件系统存储驱动接口
type Handler interface {
	// Put 服务端上传文件
	Put(file FileHeader) error
	// Get 获取文件内容
	Get(path string) (io.ReadCloser, error)
	// Delete 删除文件
	Delete(paths ...string) error
	// List 列出目录
	List(path string) ([]Object, error)
	// Source 获取文件访问 URL
	Source(path string, ttl time.Duration) (string, error)
	// Token 生成客户端直传凭证（S3 返回 presigned URL，local 返回路径）
	Token(fileInfo FileInfo, ttl time.Duration) (*UploadCredential, error)
}
