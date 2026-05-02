# Phase 2 Plan

## Goal

Build the real MVP data path for TickForge:

```
Tick Producer / Simulator
        │  (POST /api/v1/ticks  +  X-API-Key header)
        ▼
HTTP Ingestion Handler  (internal/ingest)
        │  202 / 429 / 503
        ▼
Bounded Tick Queue  (TICKFORGE_QUEUE_SIZE, Go channel)
        │
        ▼
Worker Pool  (TICKFORGE_WORKERS goroutines)
        │
        ▼
Candle Aggregator  (internal/aggregator — UTC 1m windows, drop-late policy)
        │  completed Candle
        ▼
Storage Repository  (internal/storage — upsert ON CONFLICT)
        │
        ▼
PostgreSQL  (migrations/001_create_candles.sql)
        │
        ├─────────────────────────────────────┐
        ▼                                     ▼
REST Query API                     WebSocket Broadcaster
GET /api/v1/candles                /ws/v1/candles
(symbol, timeframe,                (X-API-Key query param,
 from, to, limit, offset)          all-symbols broadcast MVP)
        │
        ▼
GET /metrics  (Prometheus)
```

Phase 1 built the foundation (config, server skeleton, models, simulator, health
endpoints). Phase 2 turns that foundation into a working end-to-end pipeline.

---

## Phase 1 status (confirmed baseline)

| Check | Result |
|---|---|
| `go test ./...` | ✅ pass |
| `go vet ./...` | ✅ pass |
| HTTP service skeleton | ✅ |
| Config loading + validation | ✅ |
| `/healthz` and `/readyz` | ✅ |
| `Tick` and `Candle` models (`float64` volume) | ✅ |
| Tick validation (symbol, price, volume, timestamp) | ✅ |
| Simulator command | ✅ |
| `migrations/001_create_candles.sql` (schema + UNIQUE constraint) | ✅ |
| Architecture documented (late-tick policy, upsert strategy, data types) | ✅ |

---

## Configuration additions for Phase 2

All new settings follow the existing `TICKFORGE_` prefix convention in `internal/config`.

| Env var | Default | Description |
|---|---|---|
| `TICKFORGE_DB_DSN` | *(required)* | `postgres://user:pass@host:5432/tickforge?sslmode=disable` |
| `TICKFORGE_DB_MAX_OPEN_CONNS` | `10` | `pgxpool` max open connections |
| `TICKFORGE_DB_MAX_IDLE_CONNS` | `5` | `pgxpool` idle connections |
| `TICKFORGE_DB_CONN_TIMEOUT` | `5s` | Per-connection acquisition timeout |
| `TICKFORGE_DB_QUERY_TIMEOUT` | `3s` | Per-query context deadline |
| `TICKFORGE_API_KEY` | *(required)* | Static API key for ingest + WebSocket auth (MVP) |
| `TICKFORGE_WS_OUTBOUND_BUF` | `64` | Per-client WebSocket outbound channel buffer size |
| `TICKFORGE_CANDLE_QUERY_LIMIT` | `100` | Default and max candle query result count |

> Config validation must reject a missing `TICKFORGE_DB_DSN` or `TICKFORGE_API_KEY` at
> startup with a clear fatal log — the server must not start without them.

---

## Work plan

### 1. API key authentication middleware

**Before any other handler work**, add a thin authentication middleware in
`internal/server` that gates the ingest and WebSocket endpoints.

#### Design

- Check `X-API-Key` request header against `TICKFORGE_API_KEY` from config.
- Use `subtle.ConstantTimeCompare` to prevent timing attacks.
- On mismatch: return `401 Unauthorized` with JSON body `{"error":"unauthorized"}`.
- On match: call `next`.
- `/healthz`, `/readyz`, and `/metrics` are **not** gated — they must remain
  accessible to load balancers and Prometheus scrapers without credentials.

#### Tests

- Valid key passes through to handler.
- Missing key returns `401`.
- Wrong key returns `401`.
- Key comparison is constant-time (verified by using `subtle.ConstantTimeCompare`,
  not by timing the test itself).
- Health and metrics routes bypass auth.

---

### 2. HTTP tick ingestion

Implement `POST /api/v1/ticks` in `internal/ingest`.

#### Design

- Gate with the auth middleware from step 1.
- Accept one JSON tick per request (`Content-Type: application/json`).
- Decode body into `models.Tick`; return `400` with structured JSON on decode
  failure.
- Call `tick.Validate()`; return `422` with the validation error message on
  failure.
- Normalize `tick.Symbol` via `models.NormalizeSymbol`.
- Non-blocking send to the pipeline queue channel:
  - Queue has capacity: return `202 Accepted` with `{"status":"accepted"}`.
  - Queue is full: return `429 Too Many Requests` with
    `{"error":"queue full — retry later"}` and increment the queue-full counter.
- Handler itself does **zero** aggregation work.

#### Error response shape (all endpoints)

```json
{
  "error": "human-readable message",
  "code":  "MACHINE_READABLE_CODE"
}
```

#### Tests

- Valid tick → `202`.
- Bad JSON → `400`.
- Missing `symbol` → `422`.
- Non-finite price → `422`.
- Negative volume → `422`.
- Zero timestamp → `422`.
- Queue full → `429` and counter incremented.
- No API key → `401` (middleware test).

---

### 3. Bounded queue and worker pool

Implement `internal/pipeline`.

#### Design

- `Pipeline` struct holds:
  - `queue chan models.Tick` — buffered, size from `TICKFORGE_QUEUE_SIZE`.
  - `workers int` — from `TICKFORGE_WORKERS`.
  - Reference to `aggregator.Aggregator`.
- `Start(ctx context.Context)` launches `workers` goroutines; each loops on
  `queue` until `ctx` is cancelled.
- `Enqueue(tick models.Tick) bool` — non-blocking send; returns `false` if full.
- `DrainAndStop(timeout time.Duration)` — called during graceful shutdown:
  closes the ingest side, gives workers `timeout` to drain the remaining items,
  then cancels the context.
- Workers call `aggregator.Process(tick)` for every dequeued tick.

#### Shutdown contract

```
HTTP server stops accepting
        │
        ▼
Pipeline.DrainAndStop(TICKFORGE_SHUTDOWN_TIMEOUT / 2)
        │  (remaining budget)
        ▼
Aggregator.FlushAll()  →  Storage.UpsertCandle() for each open bucket
        │
        ▼
DB pool close
```

> `FlushAll` does **not** emit WebSocket events for partial candles — those events
> are only for completed (closed) windows.

#### Tests

- Tick enqueued → worker calls aggregator with that tick.
- Queue full → `Enqueue` returns `false` without blocking.
- `DrainAndStop` processes all queued ticks before returning.
- Second `Start` panics or errors (double-start guard).

---

### 4. 1-minute OHLCV aggregation

Implement `internal/aggregator`.

#### Design

The aggregator maintains a `map[string]*bucket` keyed by **uppercased symbol**.
A `bucket` holds:

```go
type bucket struct {
    symbol    string
    open      float64
    high      float64
    low       float64
    close_    float64
    volume    float64
    startTime time.Time  // UTC minute boundary (truncated to time.Minute)
    endTime   time.Time  // startTime + 1m
    tickCount int64
}
```

`Process(tick models.Tick) *models.Candle`:

1. Compute `windowStart := tick.Timestamp.UTC().Truncate(time.Minute)`.
2. Look up existing bucket for `tick.Symbol`.
3. **No bucket** → create new bucket; `open = price`, `high = price`,
   `low = price`, `close = price`, `volume = tick.Volume`; return `nil`.
4. **Same window** (`windowStart == bucket.startTime`) → update in place:
   `high = max(high, price)`, `low = min(low, price)`, `close = price`,
   `volume += tick.Volume`; return `nil`.
5. **Tick is late** (`windowStart < bucket.startTime`) → increment
   `tickforge_late_ticks_total{symbol}` counter; **discard tick**; return `nil`.
6. **New window** (`windowStart > bucket.startTime`) → snapshot the old bucket
   as a completed `*models.Candle`, replace with a new bucket for the new window;
   return the completed candle.

`FlushAll() []models.Candle` — called during shutdown — snapshots and returns
all open buckets without clearing them (caller persists, then discards).

**Concurrency:** The aggregator is protected by a single `sync.Mutex`. Workers
call `Process` concurrently; the lock serializes per-map access. A future phase
may shard by symbol for higher throughput.

#### Tests

- Single tick → no candle emitted, bucket created with correct OHLCV.
- Two ticks, same minute → no candle, OHLCV updated (`high`, `low`, `close`).
- Third tick, next minute → candle emitted for first minute with correct values.
- Late tick → no candle, counter incremented, aggregator state unchanged.
- Two symbols → candles are independent; no cross-symbol corruption.
- `FlushAll` returns open buckets.
- `Process` is safe to call from multiple goroutines (race detector).

---

### 5. PostgreSQL schema and storage

Wire `internal/storage` using `pgxpool`.

#### Migration tool: `golang-migrate`

Use [`golang-migrate/migrate`](https://github.com/golang-migrate/migrate) with
the `postgres` driver.

Install CLI (local dev):
```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

Apply migrations:
```bash
migrate -database "$TICKFORGE_DB_DSN" -path migrations up
```

Migration files follow the naming convention:
```
migrations/
  001_create_candles.up.sql    ← already defined (rename from .sql)
  001_create_candles.down.sql  ← DROP TABLE candles;
```

> Rename `migrations/001_create_candles.sql` →
> `migrations/001_create_candles.up.sql` and add the `.down.sql` counterpart.

#### Repository interface

```go
// internal/storage/repository.go
type Repository interface {
    UpsertCandle(ctx context.Context, c models.Candle) error
    QueryCandles(ctx context.Context, q CandleQuery) ([]models.Candle, error)
    Ping(ctx context.Context) error
}

type CandleQuery struct {
    Symbol    string
    Timeframe string
    From      time.Time   // inclusive; zero = no lower bound
    To        time.Time   // exclusive; zero = no upper bound
    Limit     int         // capped at TICKFORGE_CANDLE_QUERY_LIMIT
    Offset    int
}
```

#### `UpsertCandle` SQL

```sql
INSERT INTO candles
    (symbol, timeframe, open, high, low, close, volume, start_time, end_time)
VALUES
    ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (symbol, timeframe, start_time)
DO UPDATE SET
    high       = EXCLUDED.high,
    low        = EXCLUDED.low,
    close      = EXCLUDED.close,
    volume     = EXCLUDED.volume,
    end_time   = EXCLUDED.end_time,
    updated_at = NOW();
```

#### `QueryCandles` SQL

```sql
SELECT symbol, timeframe, open, high, low, close, volume, start_time, end_time
FROM candles
WHERE symbol    = $1
  AND timeframe = $2
  AND ($3::timestamptz IS NULL OR start_time >= $3)
  AND ($4::timestamptz IS NULL OR start_time <  $4)
ORDER BY start_time DESC
LIMIT  $5
OFFSET $6;
```

#### DB connection pool config (mapped from config)

```go
poolCfg.MaxConns         = int32(cfg.DBMaxOpenConns)
poolCfg.MinConns         = int32(cfg.DBMaxIdleConns)
poolCfg.MaxConnLifetime  = 30 * time.Minute
poolCfg.MaxConnIdleTime  = cfg.DBConnTimeout
```

#### `/readyz` upgrade

Once storage is wired, `GET /readyz` must call `Repository.Ping(ctx)` with a
`TICKFORGE_DB_QUERY_TIMEOUT` deadline. If the ping fails, respond `503` with
`{"status":"db unavailable"}`.

#### Tests

- `UpsertCandle` inserts a new row.
- Second `UpsertCandle` for same `(symbol, timeframe, start_time)` updates
  `high`, `low`, `close`, `volume`, `updated_at` without inserting a duplicate.
- `QueryCandles` returns rows in descending `start_time` order.
- `QueryCandles` with `from`/`to` filters only the correct window.
- `QueryCandles` with `limit` and `offset` pages correctly.
- `QueryCandles` with no rows returns an empty slice, not an error.
- `/readyz` returns `503` when the DB is unreachable (use a dead DSN in the test).

> Use a real Postgres instance (via Docker Compose or `testcontainers-go`) for
> storage tests. Do **not** mock the DB — the upsert SQL must be verified against
> the real Postgres planner.

---

### 6. Candle query API

Implement `GET /api/v1/candles` in `internal/ingest` (or a new `internal/query`
package if scope warrants).

#### Route: `GET /api/v1/candles`

**Not** gated by API key — read endpoints are public in MVP.

Query parameters:

| Param | Required | Default | Notes |
|---|---|---|---|
| `symbol` | ✅ | — | Case-insensitive; normalized server-side |
| `timeframe` | ✅ | — | MVP only accepts `1m` |
| `from` | ❌ | none | RFC 3339 UTC timestamp; inclusive lower bound on `start_time` |
| `to` | ❌ | none | RFC 3339 UTC timestamp; exclusive upper bound on `start_time` |
| `limit` | ❌ | 100 | Max `TICKFORGE_CANDLE_QUERY_LIMIT`; clamped server-side |
| `offset` | ❌ | 0 | Non-negative integer |

**Response (200)**:

```json
{
  "symbol":    "INFY",
  "timeframe": "1m",
  "from":      "2026-05-02T10:00:00Z",
  "to":        "2026-05-02T10:05:00Z",
  "limit":     100,
  "offset":    0,
  "count":     3,
  "candles": [
    { "symbol":"INFY","timeframe":"1m","open":100.00,"high":100.50,
      "low":99.80,"close":100.20,"volume":350.5,
      "start_time":"2026-05-02T10:04:00Z","end_time":"2026-05-02T10:05:00Z" }
  ]
}
```

- `candles` is always an array; never `null`.
- Results are ordered by `start_time DESC`.

**Error responses** use the standard `{"error":"...", "code":"..."}` shape:

| Case | Status |
|---|---|
| Missing `symbol` | `400` |
| Missing `timeframe` | `400` |
| Unsupported `timeframe` | `400` |
| Non-RFC3339 `from`/`to` | `400` |
| `limit` < 1 or non-integer | `400` |
| `offset` < 0 or non-integer | `400` |

#### Tests

- Returns stored candles for a valid query.
- `from`/`to` filter works correctly.
- `limit` and `offset` page correctly.
- Missing `symbol` → `400`.
- Missing `timeframe` → `400`.
- Unsupported `timeframe` → `400`.
- Malformed `from` → `400`.
- `limit` exceeding max is clamped (not rejected).
- Empty result → `200` with `"candles":[]`.

---

### 7. WebSocket candle events

Implement `/ws/v1/candles` in `internal/websocket`.

#### Auth

WebSocket clients pass the API key as a query param (HTTP headers are unreliable
for browser WebSocket upgrades):

```
ws://host/ws/v1/candles?api_key=<TICKFORGE_API_KEY>
```

The upgrade handler reads `r.URL.Query().Get("api_key")` and compares with
`subtle.ConstantTimeCompare`. Reject with HTTP `401` **before** the upgrade if
the key is missing or wrong.

#### Hub design

```go
type Hub struct {
    clients    map[*Client]struct{}
    broadcast  chan models.Candle
    register   chan *Client
    unregister chan *Client
    mu         sync.RWMutex  // protects clients map
}
```

- Hub goroutine fan-out loop:
  - `register` → add client.
  - `unregister` → remove client, close outbound channel.
  - `broadcast` → range over clients; non-blocking send to each client's
    outbound channel; if outbound channel is full (size = `TICKFORGE_WS_OUTBOUND_BUF`),
    **unregister the client and close the connection** (slow-client drop);
    increment `tickforge_ws_dropped_clients_total`.

- **MVP scope:** All clients receive all symbols. Per-symbol subscription is
  a future-phase enhancement. This resolves the contradiction between the
  architecture doc and the old plan.

#### Event payload (matches REST candle schema exactly)

```json
{
  "symbol":     "INFY",
  "timeframe":  "1m",
  "open":       100.00,
  "high":       100.50,
  "low":        99.80,
  "close":      100.20,
  "volume":     350.5,
  "start_time": "2026-05-02T10:04:00Z",
  "end_time":   "2026-05-02T10:05:00Z"
}
```

#### Wiring

Aggregator emits a completed `*models.Candle` → worker passes it to
`Storage.UpsertCandle` **and** sends it to `Hub.broadcast`. Both paths are
independent; a storage error must not suppress the broadcast and vice versa.

#### Tests

- Client connects with valid key → receives a broadcast candle.
- Client connects with invalid key → connection rejected with `401`.
- Client connects with no key → `401`.
- Hub drops slow client when outbound buffer is full; `dropped` counter
  increments.
- Event JSON matches the REST candle schema.
- Hub continues serving remaining clients after one is dropped.

---

### 8. Prometheus metrics

Implement `GET /metrics` in `internal/metrics` using `prometheus/client_golang`.

#### Metric registry

```go
// Counters
tickforge_ticks_accepted_total          counter  {symbol}
tickforge_ticks_rejected_total          counter  {reason}   // "invalid"|"queue_full"|"unauthorized"
tickforge_validation_errors_total       counter  {field}    // "symbol"|"price"|"volume"|"timestamp"
tickforge_queue_full_total              counter  —
tickforge_late_ticks_total              counter  {symbol}
tickforge_candles_completed_total       counter  {symbol}
tickforge_storage_errors_total          counter  {op}       // "upsert"|"query"
tickforge_ws_connected_clients          gauge    —
tickforge_ws_dropped_clients_total      counter  —

// Gauges
tickforge_queue_depth                   gauge    —

// Histograms
tickforge_http_handler_duration_seconds histogram {route, method, status}
tickforge_db_query_duration_seconds     histogram {op}
```

- All metrics registered in `internal/metrics/metrics.go` as package-level vars;
  no global `prometheus.DefaultRegisterer` pollution — use a dedicated registry
  passed into server wiring.
- `/metrics` handler served on the same port as the rest of the API (no separate
  port in MVP).

#### Tests

- `/metrics` responds `200` with `Content-Type: text/plain; version=0.0.4`.
- After posting a valid tick, `tickforge_ticks_accepted_total` increments.
- After posting an invalid tick, `tickforge_validation_errors_total` increments.
- After a queue-full rejection, `tickforge_queue_full_total` increments.

---

### 9. Simulator wired to live ingest

Update `cmd/simulator` to support **posting directly to the running API** in
addition to its existing stdout-JSON mode.

#### New flags

```
-target   string   HTTP base URL to POST ticks to (e.g. http://localhost:8080)
-api-key  string   API key for X-API-Key header
-rate     int      Ticks per second to send (default 1)
```

**Behaviour when `-target` is set:**

- For each tick, issue `POST <target>/api/v1/ticks` with JSON body and
  `X-API-Key: <api-key>` header.
- Log response status for each tick.
- On `429`, back off 500 ms and retry once before moving to the next tick.
- `-volume` flag already exists from the Phase 1 float64 fix.

**Behaviour when `-target` is not set:** unchanged (write to stdout).

#### Tests / checks

- Simulator with no `-target` still writes valid JSON to stdout.
- Simulator with `-target` sends ticks and logs `202` responses.
- Simulator respects `-rate` (approximate; use `time.Ticker`).

---

### 10. Docker Compose local stack

Add `docker-compose.yml` at repo root.

#### Services

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: tickforge
      POSTGRES_PASSWORD: tickforge
      POSTGRES_DB: tickforge
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD", "pg_isready", "-U", "tickforge"]
      interval: 5s
      retries: 5
```

#### Local startup sequence (document in README)

```bash
# 1. Start Postgres
docker compose up -d postgres

# 2. Apply migrations
migrate -database "postgres://tickforge:tickforge@localhost:5432/tickforge?sslmode=disable" \
        -path migrations up

# 3. Start server
TICKFORGE_DB_DSN="postgres://tickforge:tickforge@localhost:5432/tickforge?sslmode=disable" \
TICKFORGE_API_KEY="devkey" \
go run ./cmd/server

# 4. Send test ticks via simulator
go run ./cmd/simulator -target http://localhost:8080 -api-key devkey -count 10

# 5. Query candles
curl "http://localhost:8080/api/v1/candles?symbol=INFY&timeframe=1m"
```

#### Checks

- Fresh clone → `docker compose up -d` → `migrate up` → server starts → no
  errors.
- `curl /healthz` → `200`.
- `curl /readyz` → `200` (DB reachable).
- Simulator sends ticks → candles appear in query response.

---

### 11. Integration test (end-to-end)

Add `internal/integration/` (or `test/integration/`) with a build tag
`//go:build integration`.

#### Scenario: full pipeline

1. Start a real Postgres via `testcontainers-go`.
2. Apply migrations programmatically.
3. Start the TickForge HTTP server in-process against the test DB.
4. POST 5 ticks for `BTCUSDT` within the same UTC minute.
5. POST 1 tick for `BTCUSDT` in the next UTC minute (triggers candle close).
6. Assert:
   - `GET /api/v1/candles?symbol=BTCUSDT&timeframe=1m` returns exactly 1
     completed candle with correct OHLCV values.
   - WebSocket client received a candle event with the same values.
   - `GET /metrics` contains `tickforge_candles_completed_total` > 0.
7. POST a tick with a past timestamp; assert `tickforge_late_ticks_total` > 0.
8. Assert `/readyz` returns `200`.

Run with:

```bash
go test -tags integration ./internal/integration/... -v
```

> Integration tests are **excluded** from the standard `go test ./...` run so
> CI can run them in a separate step with Docker available.

---

## Implementation order

| Step | Task | Depends on |
|---|---|---|
| 1 | Auth middleware | config |
| 2 | HTTP ingest handler | auth middleware, pipeline queue interface |
| 3 | Bounded queue + worker pool | ingest handler |
| 4 | Aggregator (unit-tested heavily) | models |
| 5 | Storage: migration rename + repository | schema, config (DB DSN + pool) |
| 6 | Wire aggregator → storage | aggregator, storage |
| 7 | Candle query API | storage |
| 8 | WebSocket hub + auth | aggregator, storage |
| 9 | Prometheus metrics (instrument all layers) | all above |
| 10 | Docker Compose + docs update | storage |
| 11 | Simulator live-post mode | ingest handler |
| 12 | Integration test | all above + Docker |
| 13 | `go fmt ./...` · `go test ./...` · `go vet ./...` | — |

---

## Definition of done

Phase 2 is complete when **all** of the following are true:

### Functional
- [ ] A valid tick can be posted to `POST /api/v1/ticks` with an API key.
- [ ] The tick travels through the bounded queue and worker pool to the aggregator.
- [ ] A deterministic 1-minute OHLCV candle is produced at UTC minute boundaries.
- [ ] Late ticks are dropped and counted in `tickforge_late_ticks_total`.
- [ ] Completed candles are persisted via upsert; no duplicates after restart.
- [ ] Candles are queryable via `GET /api/v1/candles` with `from`, `to`, `limit`,
      and `offset` support.
- [ ] Candle events are broadcast to WebSocket clients authenticated with the API key.
- [ ] Slow WebSocket clients are dropped without affecting other clients.

### Operational
- [ ] `GET /healthz` returns `200` (process alive).
- [ ] `GET /readyz` returns `200` when DB is reachable; `503` otherwise.
- [ ] `GET /metrics` returns valid Prometheus text with all planned metrics.
- [ ] Graceful shutdown drains the queue and flushes open buckets to Postgres.
- [ ] `docker compose up -d postgres` + `migrate up` + server start works on a
      fresh clone.

### Quality
- [ ] `go test ./...` passes (unit tests only).
- [ ] `go test -tags integration ./...` passes (requires Docker).
- [ ] `go vet ./...` passes.
- [ ] `go fmt ./...` produces no diff.
- [ ] Race detector clean: `go test -race ./...`.
- [ ] Integration test covers the full tick→candle→REST+WebSocket path.

### Known limitations (accepted for MVP)
- No per-symbol WebSocket subscription filtering (all clients receive all symbols).
- No tick deduplication (retried POSTs may inflate volume).
- Partial candles from the current open minute are lost on unclean shutdown.
- No pagination cursor; offset-based only.
- Single API key (no per-client key management).
