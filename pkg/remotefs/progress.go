package remotefs

import "context"

type ProgressPhase string

const (
	ProgressPreparing    ProgressPhase = "preparing"
	ProgressTransferring ProgressPhase = "transferring"
	ProgressFinalizing   ProgressPhase = "finalizing"
	ProgressCompleted    ProgressPhase = "completed"
)

type Progress struct {
	Phase            ProgressPhase `json:"phase"`
	TransferredBytes int64         `json:"transferred_bytes"`
	TotalBytes       int64         `json:"total_bytes,omitempty"`
	BytesPerSecond   float64       `json:"bytes_per_second,omitempty"`
}

type ProgressSink interface {
	WriteProgress(ctx context.Context, progress Progress) error
}

type ProgressSinkFunc func(ctx context.Context, progress Progress) error

func (f ProgressSinkFunc) WriteProgress(ctx context.Context, progress Progress) error {
	if f == nil {
		return nil
	}
	return f(ctx, progress)
}

func EmitProgress(ctx context.Context, sink ProgressSink, progress Progress) error {
	if ctx == nil {
		return ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	switch progress.Phase {
	case ProgressPreparing, ProgressTransferring, ProgressFinalizing, ProgressCompleted:
	default:
		return ErrInvalidArgument
	}
	if progress.TransferredBytes < 0 || progress.TotalBytes < 0 || progress.BytesPerSecond < 0 {
		return ErrInvalidArgument
	}
	if progress.TotalBytes > 0 && progress.TransferredBytes > progress.TotalBytes {
		return ErrInvalidArgument
	}
	if sink == nil {
		return nil
	}
	return sink.WriteProgress(ctx, progress)
}
