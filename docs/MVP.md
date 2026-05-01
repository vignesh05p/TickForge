# MVP definition

## MVP objective

Ship a **minimal, credible** real-time pipeline that ingests ticks, validates them, processes them through a **bounded queue** and **worker pool**, aggregates **1-minute OHLCV** candles, **persists** candles to PostgreSQL, exposes a **latest candle query** API, **broadcasts** candle updates over WebSocket, exposes **Prometheus** metrics and **health/readiness** endpoints, runs under **Docker Compose** for local development, and includes **unit tests**—without trading logic, brokers, or a frontend.

## Included features

- HTTP tick ingestion (`POST /api/v1/ticks`)  
- Tick validation (schema + rules; see [API.md](API.md))  
- Bounded in-memory queue between ingest and workers  
- Worker pool with bounded concurrency  
- **1-minute** OHLCV candle aggregation  
- PostgreSQL persistence for candles  
- Latest candle query API (`GET /api/v1/candles`)  
- WebSocket candle broadcasting (`/ws/v1/candles`)  
- Prometheus metrics (`GET /metrics`)  
- Health endpoint (`GET /healthz`)  
- Readiness endpoint (`GET /readyz`)  
- Graceful shutdown (drain in-flight work, close listeners)  
- Docker Compose for local stack  
- Unit tests for core packages  

## Excluded features

- Real broker or exchange integration  
- Live stock exchange or vendor market data  
- AI trading signals or prediction models  
- Frontend dashboard  
- Authentication or authorization  
- Multi-user accounts  
- Payment or billing  
- Order or trade execution  

## Acceptance criteria

- A tick accepted by the API appears in the pipeline without unbounded memory growth under documented load assumptions.  
- Candles for a symbol and `1m` timeframe are **deterministic** for a given ordered tick sequence (late-tick policy documented).  
- Persisted candles survive restart; query API returns stored data consistent with aggregation rules.  
- WebSocket clients receive candle events matching the REST candle schema for subscribed symbols (exact subscription model TBD in implementation).  
- `/healthz` and `/readyz` behave as documented; `/metrics` scrapes successfully in Prometheus.  
- Shutdown completes without panic; in-flight work policy is documented.  
- `go test ./...` passes in CI; critical paths have unit tests.  
- `docker compose` (or documented equivalent) brings up dependencies for local dev.  

## Non-goals

- Profit, alpha, or “signals” of any kind  
- Compliance packaging as a financial product  
- Multi-region HA or enterprise SLAs in v1  
- Arbitrary timeframe zoo beyond what MVP explicitly includes  

## First release target

**MVP v0.1** — single-node deployment, 1m candles only, Postgres as source of truth for candles, documented limits (max payload, queue size, worker count). Timeline is driven by implementation PRs after this foundation is merged.
