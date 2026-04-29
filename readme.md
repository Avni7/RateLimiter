# Distributed API Rate Limiter (SDE-3 Level)

A high-performance, multi-tenant distributed rate limiter built in Go.

## 🚀 Architecture Highlights
- **3-Tier Caching Strategy**: 
    1. **Tier 1 (PostgreSQL)**: Source of truth for tenant configurations.
    2. **Tier 2 (Redis)**: Distributed cache for cluster-wide syncing.
    3. **Tier 3 (Local RAM)**: LRU Cache (in-memory) for sub-millisecond lookups.
- **Atomic Algorithms**: Uses Redis Lua Scripts to ensure 100% accuracy under massive concurrency.
    - **Token Bucket**: Allows for bursty traffic with steady refills.
    - **Sliding Window Log**: The most accurate algorithm for strict rate enforcement.
- **Real-time Sync**: Uses Redis Pub/Sub to invalidate local caches across the cluster instantly when rules change.

## 🛠️ Tech Stack
- **Language**: Go (Golang)
- **Databases**: PostgreSQL, Redis
- **Tooling**: Docker, `hey` (Load Testing)