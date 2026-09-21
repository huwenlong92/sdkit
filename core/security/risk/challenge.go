package risk

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// ChallengeKey identifies one rule's verification requirement for one target.
// It deliberately excludes the request IP for account-scoped requirements.
type ChallengeKey struct {
	Service     string
	Scene       string
	Rule        string
	TargetType  string
	TargetValue string
}

type ChallengeRequirement struct {
	Key     ChallengeKey
	Version string
}

// ChallengeStore keeps temporary verification requirements separate from audit
// counters. Reading does not increment counters or extend the requirement TTL.
type ChallengeStore struct{ client redis.UniversalClient }

var ErrChallengeUnavailable = errors.New("risk: challenge store unavailable")

func NewChallengeStore(client redis.UniversalClient) *ChallengeStore {
	return &ChallengeStore{client: client}
}

func (s *ChallengeStore) Get(ctx context.Context, keys []ChallengeKey) ([]ChallengeRequirement, error) {
	if s == nil || s.client == nil {
		return nil, ErrChallengeUnavailable
	}
	result := make([]ChallengeRequirement, 0, len(keys))
	if len(keys) == 0 {
		return result, s.client.Ping(ctx).Err()
	}
	pipe := s.client.Pipeline()
	commands := make([]*redis.StringCmd, len(keys))
	for i, key := range keys {
		commands[i] = pipe.Get(ctx, challengeRedisKey(key))
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	for i, command := range commands {
		value, err := command.Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result = append(result, ChallengeRequirement{Key: keys[i], Version: value})
	}
	return result, nil
}

func (s *ChallengeStore) Require(ctx context.Context, key ChallengeKey, ttl time.Duration) error {
	if s == nil || s.client == nil {
		return ErrChallengeUnavailable
	}
	if key.Service == "" || key.Scene == "" || key.Rule == "" || key.TargetType == "" || key.TargetValue == "" || ttl <= 0 {
		return errors.New("risk: invalid challenge requirement")
	}
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return err
	}
	return s.client.Set(ctx, challengeRedisKey(key), hex.EncodeToString(value[:]), ttl).Err()
}

var resolveChallengeScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
 return redis.call("DEL", KEYS[1])
end
return 0
`)

// Resolve clears only the observed generation. A concurrent failure's newer
// requirement survives. Callers explicitly choose which target types to clear.
func (s *ChallengeStore) Resolve(ctx context.Context, requirement ChallengeRequirement) error {
	if s == nil || s.client == nil {
		return ErrChallengeUnavailable
	}
	if requirement.Version == "" {
		return nil
	}
	return resolveChallengeScript.Run(ctx, s.client, []string{challengeRedisKey(requirement.Key)}, requirement.Version).Err()
}

// ClaimProof prevents concurrent reuse of a proof before its verifier runs.
// The scope must match the verifier's namespace; ttl must cover proof validity.
func (s *ChallengeStore) ClaimProof(ctx context.Context, scope, proof string, ttl time.Duration) (bool, error) {
	if s == nil || s.client == nil {
		return false, ErrChallengeUnavailable
	}
	if scope == "" || proof == "" || ttl <= 0 {
		return false, errors.New("risk: invalid proof claim")
	}
	return s.client.SetNX(ctx, "security:risk:proof:"+hashParts(scope, proof), "1", ttl).Result()
}

func challengeRedisKey(key ChallengeKey) string {
	return "security:risk:challenge:" + hashParts(key.Service, key.Scene, key.Rule, key.TargetType, key.TargetValue)
}
