# Architecture

## High-level architecture

TickForge is a **single-service** Go application (plus PostgreSQL) that accepts ticks over HTTP, processes them through a **validated, bounded pipeline**, aggregates **1-minute OHLCV** candles, persists candles to Postgres, serves queries, broadcasts updates over WebSocket, and exposes **Prometheus** metrics. A **simulator** command generates load for local testing.

## Text architecture diagram

```
Tick Producer / Simulator
        │
        ▼
HTTP Ingestion API
        │
        ▼
Validation Layer
        │
        ▼
Bounded Tick Queue
        │
        ▼
Worker Pool
        │
        ▼
Candle Aggregator
        │
        ▼
Candle Store
        │
        ▼
PostgreSQL
        │
        ├──────────────────┐
        ▼                  ▼
WebSocket Broadcaster   Prometheus Metrics
```

## Lifecycle of a tick

1. **Receive** — HTTP handler accepts JSON body and parses a tick.  
2. **Validate** — Reject malformed or out-of-policy ticks with stable error codes.  
3. **Enqueue** — Push to a **bounded** channel or queue; if full, apply **backpressure** (e.g. `503` or `429`—exact behavior documented at implementation time).  
4. **Process** — Worker dequeues, routes to aggregator keyed by **symbol** (and possibly shard rules later).  
5. **Aggregate** — Update the **current 1m bucket** (open/high/low/close/volume); on bucket close, emit a **completed candle**.  
6. **Persist** — Candle store writes completed candles to PostgreSQL.  
7. **Broadcast** — WebSocket hub sends candle events to interested clients.  
8. **Observe** — Metrics record latency, depth, errors, and drops at each stage.  

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

## Backpressure strategy

When the bounded queue is **full**:

- Prefer **fail fast** with a clear HTTP status and metric increment over unbounded buffering.  
- Document **client retry** expectations (idempotency of tick POST is out of scope for MVP unless explicitly added).  
- Surface **queue depth** and **reject count** on `/metrics` so operators can scale workers or tune limits.  

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

- Hub tracks **connected clients** and optional **symbol filters**.  
- On **candle close** (or significant update if design allows intra-minute updates—MVP should prefer **closed candle** events for simplicity), hub publishes a **JSON event** matching [API.md](API.md).  
- Slow clients may be **dropped** after a bounded outbound buffer to protect the server; drops are counted in metrics.  

## Observability design

- **Logs:** Structured, low-cardinality fields (request id, symbol, error class).  
- **Metrics:** Counters for ingest outcomes; gauges for queue depth; histograms for handler and DB latency.  
- **Tracing:** Optional future enhancement; not required for MVP.  

## Graceful shutdown behavior

On **SIGINT/SIGTERM**:

1. Stop accepting **new HTTP/WebSocket** connections.  
2. **Drain** the tick queue within a timeout or until empty—policy TBD with tests.  
3. **Flush** in-flight candles to Postgres as needed.  
4. Close **WebSocket** connections cleanly.  
5. Close **DB** pool.  

Exact timeouts and “best effort” vs “hard stop” must be documented in implementation PRs.

## Failure handling principles

- **Validate early** — bad ticks never corrupt aggregator state.  
- **Isolate failures** — DB errors increment metrics and surface in readiness; avoid panics on I/O.  
- **Don’t hide overload** — prefer explicit errors and metrics over silent drops.  
- **Test the unhappy path** — queue full, DB down, slow clients.  

This document describes **intent**; the codebase should link here from PRs that materially change behavior.
