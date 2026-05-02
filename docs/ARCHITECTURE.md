# Architecture

## High-level architecture

TickForge is a **single-service** Go application (plus PostgreSQL) that accepts ticks over HTTP, processes them through a **validated, bounded pipeline**, aggregates **1-minute OHLCV** candles, persists candles to Postgres, serves queries, broadcasts updates over WebSocket, and exposes **Prometheus** metrics. A **simulator** command generates load for local testing.

## Text architecture diagram

```
Tick Producer / Simulator
        │
        ▼
   Rate Limiter  ◄── per-IP token bucket (TICKFORGE_RATE_LIMIT / TICKFORGE_RATE_BURST)
        │  429 if exceeded
        ▼
HTTP Ingestion API  ◄── X-API-Key auth (TICKFORGE_API_KEY)
        │  401 if missing/wrong
        ▼
Validation Layer
        │  400/422 on bad input
        ▼
Bounded Tick Queue  ◄── TICKFORGE_QUEUE_SIZE
        │  429/503 if full
        ▼
Worker Pool  ◄── TICKFORGE_WORKERS goroutines
        │
        ▼
Candle Aggregator  ◄── UTC 1m windows, drop-late policy
        │  completed Candle
        ▼
Candle Store  ◄── upsert ON CONFLICT
        │
        ▼
   PostgreSQL
        │
        ├──────────────────────────┐
        ▼                          ▼
WebSocket Broadcaster       Prometheus Metrics
(all-symbol broadcast MVP)  GET /metrics
/ws/v1/candles?api_key=...  (public, no auth)
```

## Lifecycle of a tick

1. **Rate-limit** — Per-IP token-bucket check at the HTTP layer; `429` if exceeded.
2. **Authenticate** — `X-API-Key` header compared constant-time against `TICKFORGE_API_KEY`; `401` if missing or wrong.
3. **Receive** — HTTP handler accepts JSON body and parses a tick.
4. **Validate** — Reject malformed or out-of-policy ticks with stable error codes.
5. **Enqueue** — Push to a **bounded** channel; if full, `429 Too Many Requests`.
6. **Process** — Worker dequeues, routes to aggregator keyed by **symbol**.
7. **Aggregate** — Update the **current 1m bucket** (OHLCV); on bucket close, emit a **completed candle**.
8. **Persist** — Candle store upserts completed candle to PostgreSQL.
9. **Broadcast** — WebSocket hub sends candle event to all connected clients.
10. **Observe** — Metrics record latency, depth, errors, and drops at each stage.

## Package responsibilities

| Package | Responsibility |
|---------|----------------|
| `cmd/server` | Process entry, HTTP server wiring, signal handling |
| `cmd/simulator` | Synthetic tick generation for dev/test |
| `internal/config` | Env config loading with defaults and validation |
| `internal/ingest` | HTTP handlers for tick ingest (thin layer) |
| `internal/pipeline` | Queue, workers, dispatch to aggregator |
| `internal/aggregator` | OHLCV state machines per symbol/timeframe |
| `internal/storage` | Postgres repository for candles |
| `internal/websocket` | Hub, subscriptions, broadcast |
| `internal/metrics` | Prometheus registration and collectors |
| `internal/server` | HTTP server, health/readiness routes, middleware glue |
| `pkg/models` | Tick and candle types shared at boundaries, JSON contracts, basic tick validation |

Phase 1 implements the process/config/server/model foundation. Pipeline, aggregation,
storage, WebSocket, and metrics packages intentionally remain placeholders until the
MVP implementation phase.

## Data type decisions

- **Volume:** Both `Tick.Volume` and `Candle.Volume` are `float64` in Go and `DOUBLE PRECISION`
  in PostgreSQL.
  Rationale: crypto and forex assets use fractional quantities (e.g. `0.00051 BTC`). Using
  integer types truncates valid input and makes TickForge unusable for those asset classes.
  `float64` provides 15–17 significant decimal digits, sufficient for all common tick sizes.
  Validation: volume must be `>= 0`, finite, and not NaN (enforced in `pkg/models.Tick.Validate()`).
- **Price:** `float64` — same finite/positive guard, established in Phase 1.
- **Timestamps:** `time.Time` in Go; `TIMESTAMPTZ` in Postgres; always stored and compared in UTC.

## Concurrency model

- **One goroutine** (or small set) owns HTTP accept; handlers should **not** block on slow aggregation.  
- **Bounded queue** between ingest and workers; worker count capped by configuration.  
- **Aggregator** receives tick processing calls from workers; per-symbol **serialization** may use fine-grained locks or single goroutine per symbol—implementation choice must preserve correctness for 1m windows.  
- **WebSocket hub** runs its own goroutine for fan-out; broadcasts must not block persistence (decouple via channels or bounded buffers).  

## Authentication design

TickForge uses a **static API key** for MVP authentication.

- **Header:** `X-API-Key: <TICKFORGE_API_KEY>`.
- **Comparison:** `crypto/subtle.ConstantTimeCompare` — prevents timing side-channels.
- **Gated routes:** `POST /api/v1/ticks`, `/ws/v1/candles` (WebSocket upgrade).
- **Public routes (no auth required):** `GET /healthz`, `GET /readyz`, `GET /metrics`.
  Health and readiness probes must remain reachable by load balancers and Prometheus
  without credentials.
- **WebSocket auth:** Clients pass the key as a query parameter
  (`/ws/v1/candles?api_key=<key>`) because browser WebSocket APIs do not support
  custom headers before the upgrade handshake. The server validates the param with
  the same constant-time comparison before upgrading the connection.
- **On failure:** `401 Unauthorized` with JSON body `{"error":{"code":"UNAUTHORIZED"}}`.
- **Implementation:** `internal/server.RequireAPIKey(key)` middleware, applied
  via `Server.AddProtectedRoute`.
- **Future:** Multi-key or JWT-based auth is a Phase 3+ concern.

## Rate limiting design

Rate limiting operates **at the HTTP layer**, upstream of the tick queue.
This means overloaded clients receive `429` before queue backpressure can
build up, protecting server resources at the earliest possible stage.

- **Algorithm:** Token bucket (per client IP).
- **Sustained rate:** `TICKFORGE_RATE_LIMIT` tokens/sec (default: 100 req/s).
- **Burst:** `TICKFORGE_BURST` tokens (default: 20 — allows short spikes).
- **Scope:** Applied globally to all routes via `Server.Handler()`.
  Health, readiness, and metrics routes are rate-limited but not auth-gated.
- **Response on limit:** `429 Too Many Requests` + `Retry-After: 1` header.
  The tick is not enqueued; `tickforge_ticks_rejected_total{reason="rate_limited"}` increments.
- **Memory:** One bucket per unique client IP. Stale entries (idle > 5 min) are
  evicted by a background goroutine to prevent unbounded growth.
- **Implementation:** `internal/server.RateLimit(limit, burst)` middleware.
- **Relation to queue backpressure:** Rate limiting is the first line of defence.
  Queue backpressure is the second. Both may trigger independently:
  - A single fast client hits rate limiting first.
  - Many moderate clients may all pass rate limiting but fill the queue together.

## Backpressure strategy

When the bounded queue is **full**:

- Return `429 Too Many Requests` with `{"error":{"code":"QUEUE_FULL"}}` immediately.
- Increment `tickforge_queue_full_total` so operators can observe the rate.
- Surface **queue depth** and **reject count** on `/metrics` so operators can
  scale workers or tune `TICKFORGE_QUEUE_SIZE` and `TICKFORGE_WORKERS`.
- Never grow queue memory without bound; prefer explicit rejection over silent dropping.

## Aggregation strategy

- **Timeframe:** MVP uses **1 minute** aligned to UTC wall clock (UTC only; not configurable in MVP).
- **OHLCV:** Open = first tick price in the window; High = max price; Low = min price;
  Close = last tick price; Volume = sum of all tick volumes.
- **Late-tick policy (normative):**
  A tick is *late* if its `timestamp` falls before the `start_time` of the current open bucket
  for its symbol. **MVP decision: drop late ticks.**
  - The tick is discarded from aggregation.
  - A `tickforge_late_ticks_total{symbol}` Prometheus counter is incremented so operators
    can observe the rate and tune upstream producers.
  - A completed candle is **never retroactively mutated** once it has been emitted and persisted.
  - Rationale: retroactive mutation would require re-opening closed candles, invalidating
    WebSocket events already broadcast, and complicating the upsert strategy. For MVP,
    predictability beats completeness.
  - Future phases may add a configurable grace window (e.g. accept ticks up to 5 s late
    into the previous bucket) behind a feature flag.

## Persistence strategy

- **Candles** are the **primary durable artifact** in MVP; raw ticks are not stored.
- **Migrations** in `migrations/` define schema; store uses parameterized SQL only.
- **Unique constraint (normative):** The candles table enforces
  `UNIQUE(symbol, timeframe, start_time)`.
  The storage layer **must** issue an upsert on every candle write:
  ```sql
  INSERT INTO candles (...) VALUES (...)
  ON CONFLICT (symbol, timeframe, start_time)
  DO UPDATE SET high=EXCLUDED.high, low=EXCLUDED.low,
               close=EXCLUDED.close, volume=EXCLUDED.volume,
               updated_at=NOW();
  ```
  This guarantees: no duplicate rows after a crash-restart; safe intra-minute updates
  if a future phase emits partial candles before window close; idempotent replays during
  backfill or recovery.
- **Partial candles on restart:** When the server shuts down, the aggregator's current
  open (incomplete) bucket is **discarded, not flushed**. The next tick after restart opens
  a fresh bucket. Ticks from the same UTC minute that arrived before the restart are lost.
  This is an accepted MVP trade-off; WAL-backed recovery is a future-phase concern.
- **Readiness** (`/readyz`) reflects ability to query/write Postgres once storage is wired.

## WebSocket broadcasting design

- Hub tracks **connected clients** with a per-client outbound channel of size `TICKFORGE_WS_OUTBOUND_BUF`.
- **MVP scope (normative):** All connected clients receive events for **all symbols**. Per-symbol
  subscription filtering is a future-phase enhancement. This is a deliberate MVP simplification;
  the architecture and Phase 2 plan are aligned on this point.
- On **candle close**, hub publishes a JSON event whose payload matches the REST candle schema
  (see [API.md](API.md)). Intra-minute partial candle events are not broadcast in MVP.
- **Slow-client handling:** If a client's outbound channel is full at broadcast time, the hub
  immediately unregisters the client, closes the WebSocket connection, and increments
  `tickforge_ws_dropped_clients_total`. This protects the server from head-of-line blocking.
- **Authentication:** Clients supply the API key as a query parameter
  (`?api_key=<TICKFORGE_API_KEY>`) before the WebSocket upgrade. Invalid or missing keys
  are rejected with HTTP `401` before the connection is upgraded.

## Observability design

- **Logs:** Structured, low-cardinality fields (request id, symbol, error class).  
- **Metrics:** Counters for ingest outcomes; gauges for queue depth; histograms for handler and DB latency.  
- **Tracing:** Optional future enhancement; not required for MVP.  

## Graceful shutdown behavior

On **SIGINT/SIGTERM** (normative sequence):

1. **Stop HTTP/WebSocket accept** -- the HTTP server stops accepting new connections
   immediately. In-flight requests complete up to `TICKFORGE_SHUTDOWN_TIMEOUT`.
2. **Drain the tick queue** -- `Pipeline.DrainAndStop` is called with a deadline of
   `TICKFORGE_SHUTDOWN_TIMEOUT / 2`. Workers process queued ticks until the queue is
   empty or the deadline is reached. Remaining ticks at deadline expiry are dropped;
   `tickforge_queue_dropped_on_shutdown_total` is incremented for each.
3. **Flush open aggregator buckets** -- `Aggregator.FlushAll()` snapshots all
   in-progress candle buckets. Each is upserted to Postgres with the remaining
   shutdown budget. Partial buckets may be overwritten by ticks arriving after restart.
4. **Close WebSocket connections** -- hub broadcasts a close frame to all clients.
5. **Close DB pool** -- `pgxpool.Pool.Close()` is called after storage writes complete.

Shutdown is **best-effort**: if the deadline expires the server exits without waiting.
No data-loss guarantees are made for the current open minute's partial candle.

## Failure handling principles

- **Validate early** — bad ticks never corrupt aggregator state.  
- **Isolate failures** — DB errors increment metrics and surface in readiness; avoid panics on I/O.  
- **Don’t hide overload** — prefer explicit errors and metrics over silent drops.  
- **Test the unhappy path** — queue full, DB down, slow clients, bad API key, rate-limited burst.

## Known limitations (MVP)

These are **accepted trade-offs** for MVP scope, not oversights. Each has a documented
rationale and a named future-phase path.

### Tick deduplication

`POST /api/v1/ticks` is **not idempotent**. A client that retries a `202 Accepted`
response (e.g. due to a network timeout) will submit the tick a second time. This
causes the tick to be processed twice, inflating the candle's volume figure.

- **Accepted for MVP** because adding deduplication requires either a client-supplied
  idempotency key (and a short-lived dedup store) or a global sequence number scheme,
  both of which add complexity disproportionate to MVP scope.
- **Client guidance:** producers should tolerate occasional volume inflation and treat
  the `202` as a best-effort acknowledgement, not a delivery guarantee.
- **Metric exposure:** `tickforge_ticks_accepted_total` counts all accepted ticks
  including duplicates; no per-tick dedup counter is tracked.
- **Future:** Phase 3+ may add `Idempotency-Key` header support backed by a Redis
  TTL store. See Phase 2 plan known-limitations section.

### Partial candle data loss on unclean shutdown

If the server is killed with SIGKILL (or crashes) while the aggregator has an open
1-minute bucket, the in-progress candle data for that window is lost. On restart,
the next tick for that symbol opens a fresh bucket from scratch.

- **Accepted for MVP.** Graceful shutdown (SIGTERM) flushes open buckets via
  `Aggregator.FlushAll()` + upsert; only unclean termination causes loss.
- **Impact:** at most 1 minute of data per symbol per crash, which is tolerable
  for a tick-aggregation MVP.
- **Future:** WAL-backed aggregator state or periodic in-memory checkpointing
  is a Phase 3+ concern.

### Single static API key

MVP uses one shared API key (`TICKFORGE_API_KEY`). There is no per-client key
management, key rotation endpoint, or revocation mechanism.

- **Accepted for MVP.** Rotation requires a redeploy (restart with new env var).
- **Future:** Multi-key registry or JWT-based auth is a Phase 3+ concern.

### WebSocket: all-symbol broadcast only

All connected WebSocket clients receive candle events for **all symbols**. There
is no per-symbol subscription filter in MVP. This is a deliberate simplification
(see WebSocket broadcasting design above). Per-symbol filtering is a Phase 3+ concern.

---

This document describes **intent and normative decisions**; the codebase should
link here from PRs that materially change behaviour.
