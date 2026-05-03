package analytics

import (
	"log"
	"time"
)

// RequestLog represents a single API hit
type RequestLog struct {
	TenantID  string
	Route     string
	Status    int
	Timestamp time.Time
}

// 1. The Global Buffer (Holds up to 10,000 logs at a time)
var LogQueue = make(chan RequestLog, 10000)

// 2. The Background Worker
func StartAnalyticsWorker() {
	log.Println("Background Analytics Worker started...")
	
	// This loop runs forever in the background, reading from the channel
	for logEntry := range LogQueue {
		// In a production app, you would batch these and INSERT to Postgres here.
		// For our test, we will just print them beautifully to the terminal.
		if logEntry.Status == 429 {
			log.Printf("🚨 [ANALYTICS] BLOCKED: Tenant %s exceeded limits on %s", logEntry.TenantID, logEntry.Route)
		} else {
			log.Printf("✅ [ANALYTICS] PASSED: Tenant %s accessed %s", logEntry.TenantID, logEntry.Route)
		}
	}
}