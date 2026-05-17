package risk

import "context"

type Checker interface {
	Name() string
	Check(ctx context.Context, rc *Context) (*CheckResult, error)
}
