package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"os"
	"strings"

	"ratelimiter/internal/limiter"
	"ratelimiter/internal/storage"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
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

// Request body format for the frontend
type AIConfigRequest struct {
	Prompt string `json:"prompt"`
}

func HandleAIConfig(ctx context.Context, engine *limiter.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.Header.Get("X-Tenant-ID")
		if tenantID == "" {
			http.Error(w, "Missing X-Tenant-ID header", http.StatusBadRequest)
			return
		}

		var req struct {
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}

		apiKey := os.Getenv("GEMINI_API_KEY")
		client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
		if err != nil {
			log.Printf("Failed to create AI client: %v", err)
			http.Error(w, "AI configuration error on server", http.StatusInternalServerError)
			return
		}

		defer client.Close()

		model := client.GenerativeModel("gemini-2.5-flash") // Current fast model

		model.SystemInstruction = genai.NewUserContent(genai.Text(
			`You are a JSON configuration generator. Output ONLY raw JSON matching this structure:
			{"routes": {"/path": {"algorithm": "sliding_window", "limit": 10, "window_seconds": 60}}}`))

		resp, err := model.GenerateContent(ctx, genai.Text(req.Prompt))
		if err != nil {
			log.Printf("GEMINI API ERROR: %v", err)
			http.Error(w, "AI failed", http.StatusInternalServerError)
			return
		}

		aiOutput := string(resp.Candidates[0].Content.Parts[0].(genai.Text))
		aiOutput = strings.TrimSpace(aiOutput)
		aiOutput = strings.TrimPrefix(aiOutput, "```json")
		aiOutput = strings.TrimPrefix(aiOutput, "```")
		aiOutput = strings.TrimSuffix(aiOutput, "```")
		aiOutput = strings.TrimSpace(aiOutput)

		// --- FIX STARTS HERE ---
		// Use the Config struct from your limiter package
		var config limiter.Config
		if err := json.Unmarshal([]byte(aiOutput), &config); err != nil {
			log.Printf("CLEANED AI OUTPUT: %s", aiOutput)
			log.Printf("GEMINI API ERROR: %v", err)
			http.Error(w, "Invalid AI JSON", http.StatusInternalServerError)
			return
		}

		// Save to Postgres (assuming your storage.Save takes a byte slice or the struct)
		// Convert struct back to JSON bytes for the database
		configBytes, _ := json.Marshal(config)
		err = storage.SaveConfig(ctx, engine.DB, tenantID, configBytes)
		if err != nil {
			http.Error(w, "DB Save Failed", http.StatusInternalServerError)
			return
		}

		// Clear Redis and Alert other instances
		engine.RedisClient.Del(ctx, "config:"+tenantID)
		engine.RedisClient.Publish(ctx, "config_updates", tenantID)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(aiOutput))
	}
}
