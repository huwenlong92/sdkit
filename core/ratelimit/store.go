package ratelimit

import "github.com/huwenlong92/sdkit/pkg/ratelimit/store"

// CustomStore is the shared store used by framework adapters.
var CustomStore store.Store

// SetStore sets the shared rate limit store. Passing nil restores adapter defaults.
func SetStore(s store.Store) {
	CustomStore = s
}

func PickStore() store.Store {
	if CustomStore != nil {
		return CustomStore
	}
	return store.NewMemoryStore()
}
