package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	webhookKeyPrefix            = "wh:"         // wh:<token> → webhook JSON
	rateLimitPrefix             = "rl:"         // rl:<userID> → request count
	webhookRateLimitPrefix      = "rl:wh:"      // rl:wh:<webhookID> → per-sec ingest count
	webhookRateLimitPerMinPrefix = "rl:wh:min:" // rl:wh:min:<webhookID> → per-min ingest count
	dailyWebhookPrefix          = "daily:wh:"   // daily:wh:<YYYY-MM-DD>:<userID> → daily request count
	credRateLimitPrefix         = "rl:cred:"    // rl:cred:<brokerCredID> → per-credential order count
	verifyResendPrefix          = "rl:verify_resend:"
	dedupKeyPrefix              = "dedup:wh:"
	jwtRevokedPrefix            = "jwtrevoked:"
	paperQuotaNotifyPrefix      = "paper_quota_notify:"
	webhookTTL                  = 5 * time.Minute
)

type RedisStore struct {
	client *redis.Client
}

func NewRedis(url string) *RedisStore {
	opt, err := redis.ParseURL(url)
	if err != nil {
		panic(fmt.Sprintf("redis: invalid URL %q: %v", url, err))
	}
	return &RedisStore{client: redis.NewClient(opt)}
}

// GetWebhookByToken returns the cached webhook config or ErrCacheMiss.
func (r *RedisStore) GetWebhookByToken(ctx context.Context, token string) (*Webhook, error) {
	val, err := r.client.Get(ctx, webhookKeyPrefix+token).Result()
	if err == redis.Nil {
		return nil, nil // cache miss — caller must hit Postgres
	}
	if err != nil {
		return nil, err
	}
	var wh Webhook
	if err := json.Unmarshal([]byte(val), &wh); err != nil {
		return nil, err
	}
	return &wh, nil
}

// CacheWebhook stores a webhook config in Redis with a short TTL.
func (r *RedisStore) CacheWebhook(ctx context.Context, wh *Webhook) error {
	b, err := json.Marshal(wh)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, webhookKeyPrefix+wh.TokenHash, b, webhookTTL).Err()
}

// InvalidateWebhook removes the cached webhook (e.g. on pause or delete).
func (r *RedisStore) InvalidateWebhook(ctx context.Context, token string) error {
	return r.client.Del(ctx, webhookKeyPrefix+token).Err()
}

// IncrRateLimit increments a per-user request counter within a 1-second window.
// Returns the new count. Use this to enforce broker-side rate limits.
func (r *RedisStore) IncrWebhookRateLimit(ctx context.Context, webhookID string) (int64, error) {
	key := webhookRateLimitPrefix + webhookID
	pipe := r.client.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

func (r *RedisStore) IncrRateLimit(ctx context.Context, userID string) (int64, error) {
	key := rateLimitPrefix + userID
	pipe := r.client.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

// IncrWebhookRatePerMin increments the per-webhook ingest counter within a 1-minute window.
func (r *RedisStore) IncrWebhookRatePerMin(ctx context.Context, webhookID string) (int64, error) {
	key := webhookRateLimitPerMinPrefix + webhookID
	pipe := r.client.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, time.Minute)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

// IncrDailyWebhookRequests increments the per-user daily ingest counter.
// The key expires at the next UTC midnight so the count resets each day.
func (r *RedisStore) IncrDailyWebhookRequests(ctx context.Context, userID string) (int64, error) {
	now := time.Now().UTC()
	day := now.Format("2006-01-02")
	key := dailyWebhookPrefix + day + ":" + userID
	midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	pipe := r.client.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.ExpireAt(ctx, key, midnight)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

// TryClaimPaperQuotaNotify returns true the first time a user hits their monthly
// paper trade quota in a given UTC month (dedupes upgrade notifications).
func (r *RedisStore) TryClaimPaperQuotaNotify(ctx context.Context, userID string) (bool, error) {
	now := time.Now().UTC()
	key := fmt.Sprintf("%s%s:%d-%02d", paperQuotaNotifyPrefix, userID, now.Year(), int(now.Month()))
	ok, err := r.client.SetNX(ctx, key, "1", 32*24*time.Hour).Result()
	return ok, err
}

func (r *RedisStore) IncrBrokerCredRateLimit(ctx context.Context, brokerCredID string) (int64, error) {
	key := credRateLimitPrefix + brokerCredID
	pipe := r.client.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

// IncrVerifyResendLimit counts verification resend attempts per email within one hour.
func (r *RedisStore) IncrVerifyResendLimit(ctx context.Context, email string) (int64, error) {
	key := verifyResendPrefix + email
	pipe := r.client.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

// ClearVerifyResendLimit removes the rate-limit counter for a given email,
// allowing immediate resend attempts (admin use only).
func (r *RedisStore) ClearVerifyResendLimit(ctx context.Context, email string) error {
	return r.client.Del(ctx, verifyResendPrefix+email).Err()
}

// TryClaimSignalDedup atomically claims a signal key for the dedup window (SET NX EX).
// Returns true when this caller is the first claim within the window.
func (r *RedisStore) TryClaimSignalDedup(ctx context.Context, webhookID, signalKey string, window time.Duration) (bool, error) {
	if webhookID == "" || signalKey == "" || window <= 0 {
		return true, nil
	}
	sec := int(window / time.Second)
	if sec < 1 {
		sec = 1
	}
	key := dedupKeyPrefix + webhookID + ":" + signalKey
	ok, err := r.client.SetNX(ctx, key, "1", time.Duration(sec)*time.Second).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}

// ReleaseSignalDedup clears a claimed dedup key (e.g. when enqueue fails after claim).
func (r *RedisStore) ReleaseSignalDedup(ctx context.Context, webhookID, signalKey string) error {
	if webhookID == "" || signalKey == "" {
		return nil
	}
	key := dedupKeyPrefix + webhookID + ":" + signalKey
	return r.client.Del(ctx, key).Err()
}

func (r *RedisStore) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func jwtRevokedKey(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return jwtRevokedPrefix + hex.EncodeToString(sum[:])
}

// RevokeJWT blocks a bearer token until its natural expiry (logout).
func (r *RedisStore) RevokeJWT(ctx context.Context, rawToken string, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}
	return r.client.Set(ctx, jwtRevokedKey(rawToken), "1", ttl).Err()
}

// IsJWTRevoked reports whether a bearer token was logged out early.
func (r *RedisStore) IsJWTRevoked(ctx context.Context, rawToken string) (bool, error) {
	n, err := r.client.Exists(ctx, jwtRevokedKey(rawToken)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
