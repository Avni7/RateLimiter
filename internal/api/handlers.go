package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"ratelimiter/internal/limiter"
	"ratelimiter/internal/storage"
)

// UpdateConfigHandler executes the Write-Through Cache
func UpdateConfigHandler(ctx context.Context, engine *limiter.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
			return
		}

		tenantID := r.Header.Get("X-Tenant-ID")
		if tenantID == "" {
			http.Error(w, "Missing X-Tenant-ID", http.StatusBadRequest)
			return
		}

		var newConfig limiter.Config
		if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
			http.Error(w, "Invalid JSON format", http.StatusBadRequest)
			return
		}

		configBytes, _ := json.Marshal(newConfig)

		// 1. Write to Postgres
		if err := storage.SaveConfig(ctx, engine.DB, tenantID, configBytes); err != nil {
			http.Error(w, "Database failure", http.StatusInternalServerError)
			return
		}

		// 2. Update Redis
		engine.RedisClient.Set(ctx, "config:"+tenantID, string(configBytes), 7*24*time.Hour)
		log.Printf("[Tier 2 - Redis] Cache updated for %s\n", tenantID)

		// 3. Alert the Cluster
		engine.RedisClient.Publish(ctx, "config_updates", tenantID)

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Successfully deployed new rules for %s\n", tenantID)
	}
}