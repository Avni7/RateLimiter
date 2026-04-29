package limiter

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"ratelimiter/internal/storage" // Replace 'ratelimiter' if your go.mod name is different
)

type RouteRule struct {
	Algorithm     string `json:"algorithm"` // e.g., "fixed_window", "token_bucket"
	Limit         int64  `json:"limit"`
	WindowSeconds int    `json:"window_seconds"`
}

type Config struct {
	Routes map[string]RouteRule `json:"routes"`
}

type Engine struct {
	DB          *pgxpool.Pool
	RedisClient *redis.Client
	LocalCache  *lru.Cache[string, Config]
}

func NewEngine(dbPool *pgxpool.Pool, redisClient *redis.Client) *Engine {
	cache, _ := lru.New[string, Config](10000)
	return &Engine{
		DB:          dbPool,
		RedisClient: redisClient,
		LocalCache:  cache,
	}
}

// GetTenantConfig executes the Tier 3 -> Tier 2 -> Tier 1 fallback
func (e *Engine) GetTenantConfig(ctx context.Context, tenantID string) (Config, error) {
	// 1. Check LRU Cache
	if config, exists := e.LocalCache.Get(tenantID); exists {
		return config, nil
	}

	// 2. Check Redis
	configJSON, err := e.RedisClient.Get(ctx, "config:"+tenantID).Result()
	var config Config

	if err == nil {
		json.Unmarshal([]byte(configJSON), &config)
		log.Printf("[Tier 2 - Redis] Fetched config for %s\n", tenantID)
	} else {
		// 3. Check Postgres
		rulesBytes, err := storage.FetchConfig(ctx, e.DB, tenantID)
		if err != nil {
			return Config{}, fmt.Errorf("tenant not found")
		}
		json.Unmarshal(rulesBytes, &config)
		e.RedisClient.Set(ctx, "config:"+tenantID, string(rulesBytes), 7*24*time.Hour)
	}

	e.LocalCache.Add(tenantID, config)
	return config, nil
}

// SyncConfigFromRedis listens for invalidation alerts
func (e *Engine) SyncConfigFromRedis(ctx context.Context) {
	pubsub := e.RedisClient.Subscribe(ctx, "config_updates")
	defer pubsub.Close()

	for {
		msg, err := pubsub.ReceiveMessage(ctx)
		if err != nil {
			continue
		}
		log.Printf("Pub/Sub Alert: Evicting Tenant [%s] from Tier 3...\n", msg.Payload)
		e.LocalCache.Remove(msg.Payload)
	}
}