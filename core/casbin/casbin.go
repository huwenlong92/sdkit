package casbin

import (
	"context"

	"github.com/huwenlong92/sdkit/core/database"
	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/runtime"

	gocasbin "github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

const (
	KeyCasbin runtime.Key = "casbin"

	DefaultModelPath = "configs/rbac_model.conf"
	DefaultSuperRole = "admin"
	DefaultRuleTable = "casbin_rule"
)

type Config struct {
	ModelPath       string `mapstructure:"model_path" yaml:"model_path"`
	SuperRole       string `mapstructure:"super_role" yaml:"super_role"`
	AutoCreateTable bool   `mapstructure:"auto_create_table" yaml:"auto_create_table"`
	RuleTable       string `mapstructure:"rule_table" yaml:"rule_table"`
}

type Manager struct {
	db       *database.Database
	enforcer *gocasbin.Enforcer
	config   Config
}

var Default *Manager

func Init(db *database.Database, cfg Config) error {
	return InitContext(context.Background(), db, cfg)
}

func InitContext(ctx context.Context, db *database.Database, cfg Config) error {
	manager, err := NewContext(ctx, db, cfg)
	if err != nil {
		return err
	}
	setDefault(manager)
	return nil
}

func New(db *database.Database, cfg Config) (*Manager, error) {
	return NewContext(context.Background(), db, cfg)
}

func NewContext(ctx context.Context, db *database.Database, cfg Config) (*Manager, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg.ModelPath == "" {
		cfg.ModelPath = DefaultModelPath
	}
	if cfg.SuperRole == "" {
		cfg.SuperRole = DefaultSuperRole
	}
	if cfg.RuleTable == "" {
		cfg.RuleTable = DefaultRuleTable
	}

	m, err := model.NewModelFromFile(cfg.ModelPath)
	if err != nil {
		logger.Default().Warn("Casbin模型加载失败", zap.String(logger.TraceIDKey, ""), zap.Error(err))
		return nil, err
	}

	if db != nil && db.PGX != nil && cfg.AutoCreateTable {
		if err := ensureRuleTable(ctx, db, cfg.RuleTable); err != nil {
			logger.Default().Warn("Casbin规则表创建失败", zap.String(logger.TraceIDKey, ""), zap.Error(err))
			return nil, err
		}
	}

	e, err := gocasbin.NewEnforcer(m)
	if err != nil {
		logger.Default().Warn("Casbin执行器创建失败", zap.String(logger.TraceIDKey, ""), zap.Error(err))
		return nil, err
	}

	manager := &Manager{db: db, enforcer: e, config: cfg}
	manager.loadPolicies(ctx)
	logger.Default().Info("Casbin初始化完成", zap.String(logger.TraceIDKey, ""))
	return manager, nil
}

func From(app *runtime.App) *Manager {
	if app != nil {
		if value, ok := app.Container().Get(KeyCasbin); ok {
			if manager, ok := value.(*Manager); ok {
				return manager
			}
		}
	}
	return Default
}

func Bind(app *runtime.App, manager *Manager) error {
	setDefault(manager)
	if app == nil {
		return nil
	}
	return app.Container().Bind(KeyCasbin, manager)
}

func setDefault(manager *Manager) {
	Default = manager
}

func (m *Manager) Enforcer() *gocasbin.Enforcer {
	if m == nil {
		return nil
	}
	return m.enforcer
}

func (m *Manager) Enforce(sub, obj, act string) (bool, error) {
	if m == nil || m.enforcer == nil {
		return true, nil
	}
	return m.enforcer.Enforce(sub, obj, act)
}

func (m *Manager) Reload() {
	m.ReloadContext(context.Background())
}

func (m *Manager) ReloadContext(ctx context.Context) {
	if m == nil || m.enforcer == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m.enforcer.ClearPolicy()
	m.loadPolicies(ctx)
}

func Reload() {
	if Default != nil {
		Default.Reload()
	}
}

func (m *Manager) loadPolicies(ctx context.Context) {
	if m == nil || m.db == nil || m.db.PGX == nil || m.enforcer == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	type rule struct {
		PType string
		V0    string
		V1    string
		V2    string
	}
	var rules []rule
	rows, err := m.db.PGX.Query(ctx, "SELECT ptype, v0, v1, v2 FROM "+quoteTable(m.config.RuleTable))
	if err != nil {
		logger.Default().Warn("Casbin规则加载失败", zap.String(logger.TraceIDKey, ""), zap.Error(err))
		return
	}
	defer rows.Close()

	for rows.Next() {
		var r rule
		if err := rows.Scan(&r.PType, &r.V0, &r.V1, &r.V2); err != nil {
			logger.Default().Warn("Casbin规则解析失败", zap.String(logger.TraceIDKey, ""), zap.Error(err))
			return
		}
		rules = append(rules, r)
	}
	if err := rows.Err(); err != nil {
		logger.Default().Warn("Casbin规则读取失败", zap.String(logger.TraceIDKey, ""), zap.Error(err))
		return
	}

	for _, r := range rules {
		switch r.PType {
		case "p":
			m.enforcer.AddPolicy(r.V0, r.V1, r.V2)
		case "g":
			m.enforcer.AddGroupingPolicy(r.V0, r.V1)
		}
	}

	has, _ := m.enforcer.HasPolicy(m.config.SuperRole, "*", "*")
	if !has {
		m.enforcer.AddPolicy(m.config.SuperRole, "*", "*")
		_, err := m.db.PGX.Exec(
			ctx,
			"INSERT INTO "+quoteTable(m.config.RuleTable)+" (ptype, v0, v1, v2) VALUES ('p', $1, '*', '*') ON CONFLICT DO NOTHING",
			m.config.SuperRole,
		)
		if err != nil {
			logger.Default().Warn("Casbin超级角色规则写入失败", zap.String(logger.TraceIDKey, ""), zap.Error(err))
		}
	}
}

func quoteTable(table string) string {
	return pgx.Identifier{table}.Sanitize()
}
