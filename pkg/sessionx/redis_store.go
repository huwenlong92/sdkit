package sessionx

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/huwenlong92/sdkit/core/jsonx"

	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	client *redis.Client
	prefix string
}

func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{client: client, prefix: "session:"}
}

func NewRedisStoreWithPrefix(client *redis.Client, prefix string) *RedisStore {
	return &RedisStore{client: client, prefix: prefix}
}

func (r *RedisStore) key(id string) string {
	return r.prefix + id
}

func (r *RedisStore) Get(ctx context.Context, id string) (*Session, bool) {
	key := r.key(id)

	pipe := r.client.Pipeline()
	dataCmd := pipe.HGetAll(ctx, key)
	ttlCmd := pipe.TTL(ctx, key)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, false
	}

	data := dataCmd.Val()
	ttl := ttlCmd.Val()
	if len(data) == 0 || ttl <= 0 {
		return nil, false
	}

	subjectID, _ := strconv.ParseInt(data["subject_id"], 10, 64)
	roleID, _ := strconv.ParseInt(data["role_id"], 10, 64)
	s := &Session{
		ID:          id,
		SubjectID:   subjectID,
		SubjectType: data["subject_type"],
		Username:    data["username"],
		RoleID:      roleID,
		ExpiresAt:   time.Now().Add(ttl),
	}
	if permissions := data["permissions"]; permissions != "" {
		_ = jsonx.Unmarshal([]byte(permissions), &s.Permissions)
	}
	if extra := data["extra"]; extra != "" {
		_ = jsonx.Unmarshal([]byte(extra), &s.Extra)
	}
	return s, true
}

func (r *RedisStore) Set(ctx context.Context, s *Session, ttl time.Duration) error {
	key := r.key(s.ID)
	pipe := r.client.TxPipeline()
	fields := []any{
		"subject_id", s.SubjectID,
		"subject_type", s.SubjectType,
		"username", s.Username,
		"role_id", s.RoleID,
	}
	if len(s.Permissions) > 0 {
		b, err := jsonx.Marshal(s.Permissions)
		if err != nil {
			return fmt.Errorf("session redis marshal permissions: %w", err)
		}
		fields = append(fields, "permissions", string(b))
	}
	if s.Extra != nil {
		b, err := jsonx.Marshal(s.Extra)
		if err != nil {
			return fmt.Errorf("session redis marshal extra: %w", err)
		}
		fields = append(fields, "extra", string(b))
	}
	pipe.HSet(ctx, key, fields...)
	if len(s.Permissions) == 0 {
		pipe.HDel(ctx, key, "permissions")
	}
	if s.Extra == nil {
		pipe.HDel(ctx, key, "extra")
	}
	pipe.Expire(ctx, key, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("session redis set: %w", err)
	}
	return nil
}

func (r *RedisStore) Delete(ctx context.Context, id string) error {
	return r.client.Del(ctx, r.key(id)).Err()
}
