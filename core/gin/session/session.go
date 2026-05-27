// Package session provides thin helpers around gin-contrib/sessions.
package session

import (
	"encoding/gob"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	sessionredis "github.com/gin-contrib/sessions/redis"
	"github.com/gin-gonic/gin"
)

const (
	TypeCookie = "cookie"
	TypeMemory = "memory"
	TypeRedis  = "redis"

	DefaultKey    = "sid"
	DefaultPath   = "/"
	DefaultMaxAge = 30 * 60
)

var ErrSecretRequired = errors.New("session: secret is required")

const optionsContextKey = "__sdkit_session_options"

type Store = sessions.Store
type Session = sessions.Session
type Options = sessions.Options
type Hook func(c *gin.Context, s Session, opts Options) error

type Config struct {
	Type     string        `mapstructure:"type" yaml:"type"`
	Key      string        `mapstructure:"key" yaml:"key"`
	Secret   string        `mapstructure:"secret" yaml:"secret"`
	Path     string        `mapstructure:"path" yaml:"path"`
	Domain   string        `mapstructure:"domain" yaml:"domain"`
	MaxAge   int           `mapstructure:"max_age" yaml:"max_age"`
	Secure   bool          `mapstructure:"secure" yaml:"secure"`
	HTTPOnly bool          `mapstructure:"http_only" yaml:"http_only"`
	SameSite http.SameSite `mapstructure:"same_site" yaml:"same_site"`
	Redis    RedisConfig   `mapstructure:"redis" yaml:"redis"`
}

type RedisConfig struct {
	Network   string `mapstructure:"network" yaml:"network"`
	Address   string `mapstructure:"address" yaml:"address"`
	Host      string `mapstructure:"host" yaml:"host"`
	Port      int    `mapstructure:"port" yaml:"port"`
	Username  string `mapstructure:"username" yaml:"username"`
	Password  string `mapstructure:"password" yaml:"password"`
	DB        int    `mapstructure:"db" yaml:"db"`
	PoolSize  int    `mapstructure:"pool_size" yaml:"pool_size"`
	KeyPrefix string `mapstructure:"key_prefix" yaml:"key_prefix"`
}

func Register(value any) {
	if value != nil {
		gob.Register(value)
	}
}

func NewStore(cfg Config) (Store, error) {
	if strings.TrimSpace(cfg.Secret) == "" {
		return nil, ErrSecretRequired
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Type)) {
	case "", TypeCookie, TypeMemory:
		store := cookie.NewStore([]byte(cfg.Secret))
		store.Options(cfg.Options())
		return store, nil
	case TypeRedis:
		store, err := newRedisStore(cfg.Redis, cfg.Secret)
		if err != nil {
			return nil, err
		}
		store.Options(cfg.Options())
		if cfg.Redis.KeyPrefix != "" {
			if err := sessionredis.SetKeyPrefix(store, cfg.Redis.KeyPrefix); err != nil {
				return nil, err
			}
		}
		return store, nil
	default:
		return nil, fmt.Errorf("session: unsupported store type %q", cfg.Type)
	}
}

func Middleware(cfg Config) (gin.HandlerFunc, error) {
	store, err := NewStore(cfg)
	if err != nil {
		return nil, err
	}
	opts := cfg.Options()
	middleware := sessions.Sessions(cfg.CookieKey(), store)
	return func(c *gin.Context) {
		c.Set(optionsContextKey, opts)
		middleware(c)
	}, nil
}

func Set(c *gin.Context, key string, value any, hooks ...Hook) error {
	return SetWith(c, key, value, Options{}, hooks...)
}

func SetWith(c *gin.Context, key string, value any, opts Options, hooks ...Hook) error {
	s := sessions.Default(c)
	finalOpts := optionsWith(c, opts)
	applyOptions(s, opts, finalOpts)
	s.Set(key, value)
	if err := s.Save(); err != nil {
		return err
	}
	return runHooks(c, s, finalOpts, hooks...)
}

func Get(c *gin.Context, key string) any {
	return sessions.Default(c).Get(key)
}

func Delete(c *gin.Context, key string, hooks ...Hook) error {
	return DeleteWith(c, key, Options{}, hooks...)
}

func DeleteWith(c *gin.Context, key string, opts Options, hooks ...Hook) error {
	s := sessions.Default(c)
	finalOpts := optionsWith(c, opts)
	applyOptions(s, opts, finalOpts)
	s.Delete(key)
	if err := s.Save(); err != nil {
		return err
	}
	return runHooks(c, s, finalOpts, hooks...)
}

func Clear(c *gin.Context, hooks ...Hook) error {
	return ClearWith(c, Options{}, hooks...)
}

func ClearWith(c *gin.Context, opts Options, hooks ...Hook) error {
	s := sessions.Default(c)
	finalOpts := optionsWith(c, opts)
	applyOptions(s, opts, finalOpts)
	s.Clear()
	if err := s.Save(); err != nil {
		return err
	}
	return runHooks(c, s, finalOpts, hooks...)
}

func (cfg Config) CookieKey() string {
	if strings.TrimSpace(cfg.Key) != "" {
		return strings.TrimSpace(cfg.Key)
	}
	return DefaultKey
}

func (cfg Config) Options() Options {
	opts := Options{
		Path:     DefaultPath,
		MaxAge:   DefaultMaxAge,
		HttpOnly: true,
	}
	if cfg.Path != "" {
		opts.Path = cfg.Path
	}
	if cfg.Domain != "" {
		opts.Domain = cfg.Domain
	}
	if cfg.MaxAge != 0 {
		opts.MaxAge = cfg.MaxAge
	}
	if cfg.Secure {
		opts.Secure = true
	}
	if cfg.HTTPOnly {
		opts.HttpOnly = true
	}
	if cfg.SameSite != 0 {
		opts.SameSite = cfg.SameSite
	}
	return opts
}

func newRedisStore(cfg RedisConfig, secret string) (sessionredis.Store, error) {
	poolSize := cfg.PoolSize
	if poolSize <= 0 {
		poolSize = 10
	}
	network := cfg.Network
	if network == "" {
		network = "tcp"
	}
	address := cfg.Address
	if address == "" {
		host := cfg.Host
		if host == "" {
			host = "127.0.0.1"
		}
		port := cfg.Port
		if port <= 0 {
			port = 6379
		}
		address = fmt.Sprintf("%s:%d", host, port)
	}
	return sessionredis.NewStoreWithDB(poolSize, network, address, cfg.Username, cfg.Password, strconv.Itoa(cfg.DB), []byte(secret))
}

func requestOptions(c *gin.Context) Options {
	if c == nil {
		return defaultOptions()
	}
	value, ok := c.Get(optionsContextKey)
	if !ok {
		return defaultOptions()
	}
	opts, ok := value.(Options)
	if !ok {
		return defaultOptions()
	}
	return mergeOptions(opts)
}

func optionsWith(c *gin.Context, opts Options) Options {
	return mergeOptionsWith(requestOptions(c), opts)
}

func defaultOptions() Options {
	return Options{
		Path:     DefaultPath,
		MaxAge:   DefaultMaxAge,
		HttpOnly: true,
	}
}

func mergeOptions(opts Options) Options {
	if opts.Path == "" {
		opts.Path = DefaultPath
	}
	if opts.MaxAge == 0 {
		opts.MaxAge = DefaultMaxAge
	}
	if !opts.HttpOnly {
		opts.HttpOnly = true
	}
	return opts
}

func mergeOptionsWith(base Options, override Options) Options {
	base = mergeOptions(base)
	if override.Path != "" {
		base.Path = override.Path
	}
	if override.Domain != "" {
		base.Domain = override.Domain
	}
	if override.MaxAge != 0 {
		base.MaxAge = override.MaxAge
	}
	if override.Secure {
		base.Secure = true
	}
	if override.HttpOnly {
		base.HttpOnly = true
	}
	if override.SameSite != 0 {
		base.SameSite = override.SameSite
	}
	return base
}

func applyOptions(s Session, opts Options, finalOpts Options) {
	if s == nil || isZeroOptions(opts) {
		return
	}
	s.Options(finalOpts)
}

func isZeroOptions(opts Options) bool {
	return opts.Path == "" &&
		opts.Domain == "" &&
		opts.MaxAge == 0 &&
		!opts.Secure &&
		!opts.HttpOnly &&
		opts.SameSite == 0
}

func runHooks(c *gin.Context, s Session, opts Options, hooks ...Hook) error {
	for _, hook := range hooks {
		if hook == nil {
			continue
		}
		if err := hook(c, s, opts); err != nil {
			return err
		}
	}
	return nil
}
