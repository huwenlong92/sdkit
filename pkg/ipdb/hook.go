package ipdb

import "context"

type LookupEvent struct {
	IP     string
	Record *Record
	Err    error
}

type Hook interface {
	AfterLookup(ctx context.Context, event LookupEvent)
}

type HookFunc func(ctx context.Context, event LookupEvent)

func (f HookFunc) AfterLookup(ctx context.Context, event LookupEvent) {
	if f != nil {
		f(ctx, event)
	}
}

type hookedLocator struct {
	next  Locator
	hooks []Hook
}

func (l *hookedLocator) Lookup(ctx context.Context, ip string) (*Record, error) {
	record, err := l.next.Lookup(ctx, ip)
	event := LookupEvent{
		IP:     ip,
		Record: record,
		Err:    err,
	}
	for _, hook := range l.hooks {
		if hook != nil {
			hook.AfterLookup(ctx, event)
		}
	}
	return record, err
}

func (l *hookedLocator) Close() error {
	return l.next.Close()
}
