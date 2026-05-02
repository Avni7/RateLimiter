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

	fmt.Printf("Modular Server running on port %s...\n", *port)
	log.Fatal(http.ListenAndServe(":"+*port, nil))
}