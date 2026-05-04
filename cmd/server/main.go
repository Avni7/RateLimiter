package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/redis/go-redis/v9"
	
	"ratelimiter/internal/api"
	"ratelimiter/internal/limiter"
	"ratelimiter/internal/storage"
	"ratelimiter/internal/analytics"
)

func main() {
	port := flag.String("port", "8081", "The port to run the server on")
	flag.Parse()

	ctx := context.Background()

	// 1. Connect to Postgres
	dbURL := "postgres://postgres:mysecretpassword@localhost:5432/ratelimiter"
	dbPool := storage.InitDB(ctx, dbURL)
	defer dbPool.Close()

	// 2. Connect to Redis
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	defer rdb.Close()

	// 3. Initialize the Rate Limiter Engine
	engine := limiter.NewEngine(dbPool, rdb)
	
	// Start the background Pub/Sub listener
	go engine.SyncConfigFromRedis(ctx)

	// 4. Register HTTP Routes
	http.HandleFunc("/api/data", api.LimitMiddleware(ctx, engine, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Tenant Data API Successful!\n")
	}))

	http.HandleFunc("/api/secure", api.LimitMiddleware(ctx, engine, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Secure API Successful! You bypassed the Sliding Window!\n")
	}))
	
	http.HandleFunc("/admin/config", api.UpdateConfigHandler(ctx, engine))

	http.HandleFunc("/admin/config/ai", api.HandleAIConfig(ctx, engine))

	http.HandleFunc("/api/checkout", api.LimitMiddleware(ctx, engine, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Checkout Successful!\n"))
	}))

	// Start the analytics worker in a background thread
	go analytics.StartAnalyticsWorker()

	corsMiddleware := func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000") // Allow Next.js
        w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
        w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-Tenant-ID")
        if r.Method == "OPTIONS" {
            return
        }
        next.ServeHTTP(w, r)
    })
}

	fmt.Printf("Modular Server running on port %s...\n", *port)
	log.Fatal(http.ListenAndServe(":"+*port, corsMiddleware(http.DefaultServeMux)))
}