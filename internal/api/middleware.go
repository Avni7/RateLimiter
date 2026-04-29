package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
	"log"
	"github.com/redis/go-redis/v9"
	"ratelimiter/internal/limiter"
)

// tokenBucketScript atomically calculates token refills and deductions inside Redis
var tokenBucketScript = redis.NewScript(`
	local key = KEYS[1]
	local capacity = tonumber(ARGV[1])
	local window_secs = tonumber(ARGV[2])
	local now_ms = tonumber(ARGV[3])
	
	-- Calculate how many milliseconds it takes to generate 1 token
	local ms_per_token = (window_secs * 1000) / capacity
	
	-- Fetch current state from Redis Hash
	local bucket = redis.call("HMGET", key, "tokens", "last_refill")
	local tokens = tonumber(bucket[1])
	local last_refill = tonumber(bucket[2])
	
	-- Initialize if it doesn't exist
	if not tokens then
		tokens = capacity
		last_refill = now_ms
	end
	
	-- Calculate accrued tokens based on time passed
	local elapsed_ms = math.max(0, now_ms - last_refill)
	local generated_tokens = math.floor(elapsed_ms / ms_per_token)
	
	tokens = math.min(capacity, tokens + generated_tokens)
	
	-- If we added new tokens, update the last refill timestamp
	if generated_tokens > 0 then
		last_refill = now_ms
	end
	
	-- Check if we can allow the request
	if tokens >= 1 then
		tokens = tokens - 1
		redis.call("HMSET", key, "tokens", tokens, "last_refill", last_refill)
		redis.call("EXPIRE", key, window_secs)
		return 1 -- Allowed
	else
		redis.call("HMSET", key, "tokens", tokens, "last_refill", last_refill)
		return 0 -- Rate Limited
	end
`)

// slidingWindowLogScript uses a Redis Sorted Set to perfectly track request timestamps
var slidingWindowLogScript = redis.NewScript(`
	local key = KEYS[1]
	local limit = tonumber(ARGV[1])
	local window_secs = tonumber(ARGV[2])
	local now_ms = tonumber(ARGV[3])
	
	-- Calculate the timestamp for the start of the current rolling window
	local window_start_ms = now_ms - (window_secs * 1000)
	
	-- 1. Remove all old requests that fall outside the current window
	redis.call('ZREMRANGEBYSCORE', key, '-inf', window_start_ms)
	
	-- 2. Count how many requests are currently in the valid window
	local current_requests = redis.call('ZCARD', key)
	
	if current_requests < limit then
		-- 3a. ALLOWED: Add the current request's exact timestamp to the set
		-- We use now_ms as both the score and the member
		redis.call('ZADD', key, now_ms, now_ms)
		
		-- Set a TTL to clean up memory if the user goes completely idle
		redis.call('EXPIRE', key, window_secs)
		return 1
	else
		-- 3b. RATE LIMITED: Do not add the timestamp
		return 0
	end
`)

// LimitMiddleware intercepts traffic and enforces the rules
func LimitMiddleware(ctx context.Context, engine *limiter.Engine, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.Header.Get("X-Tenant-ID")
		if tenantID == "" {
			http.Error(w, "Missing X-Tenant-ID header", http.StatusBadRequest)
			return
		}

		config, err := engine.GetTenantConfig(ctx, tenantID)
		if err != nil {
			http.Error(w, "Tenant config not found", http.StatusForbidden)
			return
		}

		path := r.URL.Path
		rule, exists := config.Routes[path]
		if !exists {
			next.ServeHTTP(w, r)
			return
		}

		ip := r.Header.Get("X-Forwarded-For")
		if ip == "" {
			ip, _, _ = net.SplitHostPort(r.RemoteAddr)
			if ip == "" { ip = r.RemoteAddr }
		}

		key := fmt.Sprintf("rate_limit:%s:%s:%s", tenantID, path, ip)
		
		var allowed bool

		// The Strategy Pattern: Route to the correct algorithm
		switch rule.Algorithm {
		case "token_bucket":
			// Execute the Lua Script
			now := time.Now().UnixMilli()
			result, err := tokenBucketScript.Run(ctx, engine.RedisClient, []string{key}, rule.Limit, rule.WindowSeconds, now).Result()
			if err != nil {
				log.Printf("Lua script error: %v", err)
				next.ServeHTTP(w, r) // Fail-open
				return
			}
			allowed = result.(int64) == 1
			
		case "sliding_window":
			now := time.Now().UnixMilli()
			result, err := slidingWindowLogScript.Run(ctx, engine.RedisClient, []string{key}, rule.Limit, rule.WindowSeconds, now).Result()
			if err != nil {
				log.Printf("Lua script error: %v", err)
				next.ServeHTTP(w, r) // Fail-open
				return
			}
			allowed = result.(int64) == 1

		case "fixed_window":
			fallthrough // If it's fixed_window (or blank), use our original logic
		default:
			// Standard INCR + EXPIRE logic
			count, err := engine.RedisClient.Incr(ctx, key).Result()
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			if count == 1 {
				engine.RedisClient.Expire(ctx, key, time.Duration(rule.WindowSeconds)*time.Second)
			}
			allowed = count <= rule.Limit
		}

		if !allowed {
			log.Printf("BLOCKED [429] - Tenant: %s | Route: %s | Algorithm: %s\n", tenantID, path, rule.Algorithm)
			http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
			return
		}

		log.Printf("ALLOWED [200] - Tenant: %s | Route: %s | Algorithm: %s\n", tenantID, path, rule.Algorithm)
		next.ServeHTTP(w, r)
	}
}