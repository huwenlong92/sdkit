package rediscontrol

import (
	"testing"

	corequeue "github.com/huwenlong92/sdkit/core/queue"
)

func TestRedisControlImplementsCoreInterfaces(t *testing.T) {
	var _ corequeue.Locker = NewLocker(nil, "")
	var _ corequeue.Idempotency = NewIdempotency(nil, "")
	var _ corequeue.RateLimiter = NewRateLimiter(nil, "")
}
