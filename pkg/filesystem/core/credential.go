package core

// UploadCredential 客户端直传凭证
type UploadCredential struct {
	UploadID    string   `json:"upload_id"`    // 上传会话 ID
	Gateway     string   `json:"gateway"`      // 存储类型
	ChunkSize   int64    `json:"chunk_size"`   // 分片大小，0 为不分片
	ChunkNum    int      `json:"chunk_num"`    // 总分片数
	UploadURLs  []string `json:"upload_urls"`  // 每片的直传 URL
	CompleteURL string   `json:"complete_url"` // 完成合并的 URL
	Path        string   `json:"path"`         // 存储路径
}
