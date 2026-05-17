package transport

import (
	"context"
	"fmt"
)

func Recover(ctx context.Context, log Logger, msg string, fields ...any) {
	if v := recover(); v != nil {
		Warn(ctx, log, msg, fmt.Errorf("panic: %v", v), fields...)
	}
}
