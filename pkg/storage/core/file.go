package core

import (
	"io"
	"sync/atomic"
	"time"
)

// FileHeader 上传文件接口
type FileHeader interface {
	io.Reader
	Info() FileInfo
}

// FileInfo 文件元信息
type FileInfo struct {
	Name     string
	Path     string
	Size     int64
	MIMEType string
	ModTime  time.Time
	Progress func(uploaded, total int64)
	Metadata map[string]string
}

// FileStream 文件流
type FileStream struct {
	reader io.Reader
	info   FileInfo
}

func NewFileStream(reader io.Reader, info FileInfo) *FileStream {
	if info.Progress != nil {
		reader = &progressReader{r: reader, total: info.Size, fn: info.Progress}
	}
	return &FileStream{reader: reader, info: info}
}

func (f *FileStream) Read(p []byte) (int, error) { return f.reader.Read(p) }
func (f *FileStream) Info() FileInfo             { return f.info }
func (f *FileStream) Close() error {
	if c, ok := f.reader.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

type progressReader struct {
	r     io.Reader
	total int64
	curr  int64
	fn    func(uploaded, total int64)
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	if n > 0 {
		uploaded := atomic.AddInt64(&p.curr, int64(n))
		p.fn(uploaded, p.total)
	}
	return n, err
}
