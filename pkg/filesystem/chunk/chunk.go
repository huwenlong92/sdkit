package chunk

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ChunkUpload 分片上传管理器
type ChunkUpload struct {
	UploadID  string // 上传会话 ID
	FileName  string // 原始文件名
	FilePath  string // 最终存储路径
	ChunkSize int64  // 每片大小
	TotalSize int64  // 文件总大小
	ChunkNum  int    // 总分片数
	tempDir   string // 临时目录
}

// NewChunkUpload 初始化分片上传
func NewChunkUpload(uploadID, fileName, filePath, tempDir string, totalSize int64, chunkSize int64) *ChunkUpload {
	chunkNum := int(totalSize / chunkSize)
	if totalSize%chunkSize != 0 {
		chunkNum++
	}
	return &ChunkUpload{
		UploadID:  uploadID,
		FileName:  fileName,
		FilePath:  filePath,
		ChunkSize: chunkSize,
		TotalSize: totalSize,
		ChunkNum:  chunkNum,
		tempDir:   tempDir,
	}
}

// ChunkPath 返回第 index 片文件的临时路径
func (cu *ChunkUpload) ChunkPath(index int) string {
	return filepath.Join(cu.tempDir, cu.UploadID, fmt.Sprintf("chunk_%05d", index))
}

// MergePath 返回合并后的文件临时路径
func (cu *ChunkUpload) MergePath() string {
	return filepath.Join(cu.tempDir, cu.UploadID, "merged")
}

// SaveChunk 保存一片分片
func (cu *ChunkUpload) SaveChunk(index int, reader io.Reader) error {
	dir := filepath.Dir(cu.ChunkPath(index))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	out, err := os.Create(cu.ChunkPath(index))
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, reader)
	return err
}

// ChunkReceived 检查第 index 片是否已上传
func (cu *ChunkUpload) ChunkReceived(index int) bool {
	_, err := os.Stat(cu.ChunkPath(index))
	return err == nil
}

// ReceivedChunks 返回已接收的分片索引列表
func (cu *ChunkUpload) ReceivedChunks() []int {
	var chunks []int
	for i := 0; i < cu.ChunkNum; i++ {
		if cu.ChunkReceived(i) {
			chunks = append(chunks, i)
		}
	}
	return chunks
}

// Merge 将所有分片合并为一个文件
func (cu *ChunkUpload) Merge() error {
	out, err := os.Create(cu.MergePath())
	if err != nil {
		return err
	}
	defer out.Close()

	for i := 0; i < cu.ChunkNum; i++ {
		in, err := os.Open(cu.ChunkPath(i))
		if err != nil {
			return fmt.Errorf("合并分片 %d 失败: %w", i, err)
		}
		_, err = io.Copy(out, in)
		in.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// Cleanup 删除所有临时分片文件
func (cu *ChunkUpload) Cleanup() error {
	return os.RemoveAll(filepath.Join(cu.tempDir, cu.UploadID))
}
