package casbin

import (
	"context"

	corecasbin "github.com/huwenlong92/sdkit/core/casbin"
	"github.com/huwenlong92/sdkit/core/runtime"

	gocasbin "github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"
)

type RoleResolver = corecasbin.RoleResolver
type ObjectResolver = corecasbin.ObjectResolver
type MiddlewareConfig = corecasbin.MiddlewareConfig
type MiddlewareOption = corecasbin.MiddlewareOption

func Init(db *Database, cfg Config) error {
	return corecasbin.Init(db, cfg)
}

func InitContext(ctx context.Context, db *Database, cfg Config) error {
	return corecasbin.InitContext(ctx, db, cfg)
}

func New(db *Database, cfg Config) (*Manager, error) {
	return corecasbin.New(db, cfg)
}

func NewContext(ctx context.Context, db *Database, cfg Config) (*Manager, error) {
	return corecasbin.NewContext(ctx, db, cfg)
}

func EnforcerFrom(app *runtime.App) *gocasbin.Enforcer {
	if manager := From(app); manager != nil {
		return manager.Enforcer()
	}
	return nil
}

func Reload() {
	corecasbin.Reload()
}

func WithManager(manager *Manager) MiddlewareOption {
	return corecasbin.WithManager(manager)
}

func WithRoleResolver(resolver RoleResolver) MiddlewareOption {
	return corecasbin.WithRoleResolver(resolver)
}

func WithObjectResolver(resolver ObjectResolver) MiddlewareOption {
	return corecasbin.WithObjectResolver(resolver)
}

func Middleware(opts ...MiddlewareOption) gin.HandlerFunc {
	return corecasbin.Middleware(opts...)
}
