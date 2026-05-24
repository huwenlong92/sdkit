package core

const (
	UploadModeLocalChunk   = "local_chunk"
	UploadModeDirectPut    = "direct_put"
	UploadModeMultipartPut = "multipart_put"
)

// UploadCredential 客户端直传凭证
type UploadCredential struct {
	Mode        string   `json:"mode"`         // 上传模式：local_chunk / direct_put / multipart_put
	UploadID    string   `json:"upload_id"`    // 上传会话 ID
	Gateway     string   `json:"gateway"`      // 存储类型
	ChunkSize   int64    `json:"chunk_size"`   // 分片大小，0 为不分片
	ChunkNum    int      `json:"chunk_num"`    // 总分片数
	UploadURLs  []string `json:"upload_urls"`  // 每片的直传 URL
	CompleteURL string   `json:"complete_url"` // 完成合并的 URL
	Path        string   `json:"path"`         // 存储路径
	AccessPath  string   `json:"access_path"`  // 前端访问值
}
