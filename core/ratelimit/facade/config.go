package ratelimit

import (
	"github.com/huwenlong92/sdkit/core/runtime"
	"github.com/huwenlong92/sdkit/pkg/ratelimit/store"
)

const KeyRateLimit runtime.Key = "ratelimit"

type Store = store.Store
