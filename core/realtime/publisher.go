package realtime

import (
	"context"
	"sync"
	"time"
)

var (
	defaultPublisherMu sync.RWMutex
	defaultPublisher   Publisher
)

func SetDefaultPublisher(publisher Publisher) {
	defaultPublisherMu.Lock()
	defaultPublisher = publisher
	defaultPublisherMu.Unlock()
}

func DefaultPublisher() Publisher {
	defaultPublisherMu.RLock()
	defer defaultPublisherMu.RUnlock()
	return defaultPublisher
}

func ClearDefaultPublisher(publisher Publisher) {
	defaultPublisherMu.Lock()
	if defaultPublisher == publisher {
		defaultPublisher = nil
	}
	defaultPublisherMu.Unlock()
}

func PushUser(ctx context.Context, userID string, action string, data any) error {
	publisher := DefaultPublisher()
	if publisher == nil {
		return ErrDefaultNotReady
	}
	return publisher.PushUser(ctx, userID, NewEvent(action, data))
}

func PushRoom(ctx context.Context, roomID string, action string, data any) error {
	publisher := DefaultPublisher()
	if publisher == nil {
		return ErrDefaultNotReady
	}
	return publisher.PushRoom(ctx, roomID, NewEvent(action, data))
}

func PublishToUser(ctx context.Context, userID string, action string, data any) error {
	return PushUser(ctx, userID, action, data)
}

func Broadcast(ctx context.Context, action string, data any) error {
	publisher := DefaultPublisher()
	if publisher == nil {
		return ErrDefaultNotReady
	}
	return publisher.Broadcast(ctx, NewEvent(action, data))
}

func NewEvent(action string, data any) *Event {
	now := time.Now().Unix()
	return (&Event{
		Action:    action,
		Event:     action,
		Data:      data,
		Timestamp: now,
		Time:      now,
	}).Normalize()
}
