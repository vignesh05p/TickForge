# Phase 2 plan

## Goal

Build the real MVP data path for TickForge:

Tick input -> validation -> bounded queue -> worker pool -> 1-minute OHLCV aggregation -> PostgreSQL storage -> REST query API -> WebSocket candle events -> Prometheus metrics.

Phase 1 created the foundation. Phase 2 should turn that foundation into a working backend pipeline.

## Current Phase 1 status

Phase 1 is done.

Confirmed in the current workspace:

- `go test ./...` passes.
- `go vet ./...` passes.
- HTTP service skeleton exists.
- Configuration loading and validation exist.
- `/healthz` and `/readyz` exist.
- Tick and candle models exist.
- Tick validation exists.
- Simulator command exists.
- Architecture, API, MVP, and Phase 1 docs exist.

## Phase 2 work plan

### 1. HTTP tick ingestion

Implement `POST /api/v1/ticks`.

Planned behavior:

- Accept one JSON tick per request.
- Validate symbol, price, volume, and timestamp.
- Normalize symbol casing.
- Return clear JSON errors for invalid input.
- Return `202 Accepted` when the tick is accepted into the queue.
- Return `429` or `503` when the queue is full.

Tests:

- Valid tick is accepted.
- Bad JSON is rejected.
- Missing symbol is rejected.
- Invalid price, volume, or timestamp is rejected.
- Queue-full behavior is tested.

### 2. Bounded queue and worker pool

Implement the internal processing pipeline.

Planned behavior:

- Queue size comes from `TICKFORGE_QUEUE_SIZE`.
- Worker count comes from `TICKFORGE_WORKERS`.
- HTTP handlers should not do aggregation work directly.
- If the queue is full, the system should apply backpressure instead of growing memory without limit.

Tests:

- Ticks move from queue to workers.
- Queue full path is deterministic.
- Shutdown does not panic.

### 3. 1-minute OHLCV aggregation

Implement candle aggregation in `internal/aggregator`.

Planned behavior:

- Support only `1m` candles in Phase 2.
- Use UTC minute boundaries.
- Open is the first tick price in the minute.
- High is the max price.
- Low is the min price.
- Close is the latest tick price in the minute.
- Volume is the sum of tick volume.
- Document and test the late tick policy.

Tests:

- Single tick creates a correct candle.
- Multiple ticks update OHLCV correctly.
- New minute closes the previous candle.
- Multiple symbols do not corrupt each other.
- Late tick behavior is covered.

### 4. PostgreSQL schema and storage

Add durable candle storage.

Planned behavior:

- Add SQL migration for candles.
- Store completed candles.
- Use a repository interface inside `internal/storage`.
- Use parameterized SQL.
- Make readiness depend on database connectivity once storage is wired.

Suggested candle table fields:

- `symbol`
- `timeframe`
- `open`
- `high`
- `low`
- `close`
- `volume`
- `start_time`
- `end_time`
- `created_at`
- `updated_at`

Tests:

- Insert candle.
- Query latest candles.
- Duplicate candle key behavior is defined.
- Readiness fails when database is unavailable.

### 5. Candle query API

Implement `GET /api/v1/candles`.

Planned behavior:

- Required query params: `symbol`, `timeframe`.
- MVP supports `timeframe=1m`.
- Return latest candles in a stable order.
- Add a default limit.
- Reject unsupported timeframe values.

Tests:

- Query returns stored candles.
- Missing symbol is rejected.
- Unsupported timeframe is rejected.
- Empty result returns an empty list, not an error.

### 6. WebSocket candle events

Implement `/ws/v1/candles`.

Planned behavior:

- Clients can connect over WebSocket.
- Server broadcasts completed candle events.
- Event payload matches the REST candle schema.
- Slow clients use bounded buffers and can be disconnected to protect the server.

Tests:

- Client receives a candle event.
- Event schema matches API docs.
- Slow client handling is covered where practical.

### 7. Prometheus metrics

Implement `GET /metrics`.

Planned metrics:

- Ingest requests accepted/rejected.
- Validation failures.
- Queue depth.
- Queue full rejections.
- Worker processing count or duration.
- Candles completed.
- Storage errors.
- WebSocket connected clients.

Tests:

- `/metrics` returns Prometheus-compatible output.
- Important counters increment on known paths.

### 8. Docker Compose local stack

Add local development support.

Planned behavior:

- `docker compose up` starts PostgreSQL.
- Server can connect to Postgres using documented environment variables.
- Migrations can be applied locally.
- README and CONTRIBUTING docs include the Phase 2 setup path.

Tests/checks:

- Fresh clone can start dependencies.
- Migrations apply cleanly.
- Server starts against local Postgres.

## Suggested implementation order

1. Build ingestion handler and queue interface.
2. Add worker pool.
3. Add aggregator with strong unit tests.
4. Add storage migration and repository.
5. Wire aggregator output to storage.
6. Add candle query API.
7. Add WebSocket broadcasting.
8. Add metrics.
9. Add Docker Compose and update docs.
10. Run full verification: `go fmt ./...`, `go test ./...`, `go vet ./...`.

## Definition of done

Phase 2 is done when:

- A valid tick can be posted to the API.
- The tick goes through a bounded queue and worker pool.
- A deterministic `1m` candle is produced.
- Completed candles are persisted in PostgreSQL.
- Candles can be queried through REST.
- Candle events can be received through WebSocket.
- `/metrics`, `/healthz`, and `/readyz` work.
- Local development works with Docker Compose.
- Critical paths have tests.
- `go test ./...` and `go vet ./...` pass.
