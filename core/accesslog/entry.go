package accesslog

import "context"

// Entry is a generic HTTP access log record.
// It intentionally has no dependency on any concrete database model.
type Entry struct {
	Source     string
	TrackID    string
	TraceID    string
	RequestID  string
	UID        string
	Method     string
	Path       string
	Query      string
	IP         string
	UserAgent  string
	Headers    []byte
	ReqBody    []byte
	StatusCode int
	ErrCode    int
	ErrMsg     string
	RespBody   []byte
	Latency    int64
	CreatedAt  int64
}

// Writer persists access log entries in batches.
type Writer interface {
	WriteBatch(ctx context.Context, entries []*Entry) error
}
