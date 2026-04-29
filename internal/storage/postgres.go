package storage

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

// InitDB connects to Postgres and auto-migrates the table
func InitDB(ctx context.Context, dbURL string) *pgxpool.Pool {
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}

	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS tenant_configs (
			tenant_id VARCHAR(255) PRIMARY KEY,
			rules_json JSONB NOT NULL
		);
	`)
	if err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}

	return pool
}

// SaveConfig persists the JSON to disk
func SaveConfig(ctx context.Context, db *pgxpool.Pool, tenantID string, configBytes []byte) error {
	_, err := db.Exec(ctx, `
		INSERT INTO tenant_configs (tenant_id, rules_json) 
		VALUES ($1, $2) 
		ON CONFLICT (tenant_id) 
		DO UPDATE SET rules_json = EXCLUDED.rules_json
	`, tenantID, configBytes)
	
	if err == nil {
		log.Printf("[Tier 1 - Postgres] Saved config for %s\n", tenantID)
	}
	return err
}

// FetchConfig retrieves the JSON from disk
func FetchConfig(ctx context.Context, db *pgxpool.Pool, tenantID string) ([]byte, error) {
	var rulesJSON []byte
	err := db.QueryRow(ctx, "SELECT rules_json FROM tenant_configs WHERE tenant_id = $1", tenantID).Scan(&rulesJSON)
	if err == nil {
		log.Printf("[Tier 1 - Postgres] Fetched config for %s\n", tenantID)
	}
	return rulesJSON, err
}