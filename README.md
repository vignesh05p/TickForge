# TickForge

**TickForge** is a high-throughput real-time tick ingestion and OHLCV candle aggregation engine built in Go.

## What TickForge is

TickForge is backend infrastructure that demonstrates **real-time event ingestion**, **bounded queues**, **worker pools**, **backpressure handling**, **candle aggregation**, **PostgreSQL persistence**, **WebSocket broadcasting**, **Prometheus metrics**, **Docker-based local development**, and **graceful shutdown**.

It uses **market-like tick data** as the example domain so the behavior is easy to reason about (price, volume, time). The same architecture applies broadly to **real-time event processing systems**—not only finance.

## What TickForge is not

- Not a trading recommendation system  
- Not a stock prediction or forecasting tool  
- Not financial advice or a regulated financial product  
- Not a broker or exchange integration  
- Not a frontend or dashboard product  

## Problem statement

High-volume continuous event streams overwhelm naive designs: accepting every event synchronously risks **latency spikes** and **connection pile-ups**; writing **every event** straight to a database creates **write amplification**, **hot partitions**, and **fragile failure modes**. Production systems need **buffering**, **aggregation**, **observability**, **backpressure**, and **clean shutdown** so they stay predictable under load.

TickForge is a focused reference implementation of those ideas using ticks and OHLCV candles as the domain.

## Core features (planned)

| Area | Capability |
|------|------------|
| Ingestion | HTTP API for tick submission |
| Pipeline | Validation, bounded queue, worker pool |
| Aggregation | 1-minute OHLCV candles |
| Storage | PostgreSQL for persisted candles |
| Real-time | WebSocket broadcast of candle updates |
| Ops | Health, readiness, Prometheus metrics |
| Runtime | Graceful shutdown, Docker Compose |

## MVP goals

Deliver a **minimal but credible** path from tick → validated queue → workers → 1m candles → Postgres → query API + WebSocket + metrics, with tests and CI—**without** broker feeds, trading logic, or a frontend. See [docs/MVP.md](docs/MVP.md).

## Architecture (text diagram)

```
                    ┌─────────────────────────┐
                    │ Tick Producer / Simulator│
                    └───────────┬─────────────┘
                                │ HTTP
                                ▼
                    ┌─────────────────────────┐
                    │    HTTP Ingestion API     │
                    └───────────┬─────────────┘
                                ▼
                    ┌─────────────────────────┐
                    │    Validation Layer     │
                    └───────────┬─────────────┘
                                ▼
                    ┌─────────────────────────┐
                    │   Bounded Tick Queue    │
                    └───────────┬─────────────┘
                                ▼
                    ┌─────────────────────────┐
                    │      Worker Pool        │
                    └───────────┬─────────────┘
                                ▼
                    ┌─────────────────────────┐
                    │   Candle Aggregator     │
                    └───────────┬─────────────┘
                                ▼
                    ┌─────────────────────────┐
                    │      Candle Store       │
                    └───────────┬─────────────┘
                                ▼
                    ┌─────────────────────────┐
                    │      PostgreSQL         │
                    └───────────┬─────────────┘
                                │
              ┌─────────────────┴─────────────────┐
              ▼                                   ▼
    ┌──────────────────┐              ┌──────────────────┐
    │ WebSocket        │              │ Prometheus       │
    │ Broadcaster      │              │ /metrics         │
    └──────────────────┘              └──────────────────┘
```

## Component overview

| Component | Role |
|-----------|------|
| **Simulator** | Generates or replays tick-like traffic for local testing |
| **Ingestion API** | Accepts ticks, applies limits, returns clear errors |
| **Validation** | Schema and business rules for ticks |
| **Bounded queue** | Absorbs bursts; signals backpressure when full |
| **Worker pool** | Bounded concurrency for processing |
| **Aggregator** | Rolls ticks into 1m OHLCV buckets |
| **Candle store** | Abstraction over Postgres reads/writes |
| **WebSocket** | Pushes candle updates to subscribers |
| **Metrics** | RED/USE-style signals for queues, workers, DB |

Details: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Tech stack

- **Language:** Go 1.22+  
- **Database:** PostgreSQL (migrations under `migrations/`)  
- **Metrics:** Prometheus exposition format  
- **Real-time:** WebSockets  
- **Local dev:** Docker Compose (planned in MVP)  

## Repository structure

```
.
├── cmd/
│   ├── server/          # HTTP + WebSocket entrypoint (implementation pending)
│   └── simulator/       # Tick producer for local load tests
├── internal/            # Private application code
│   ├── aggregator/
│   ├── config/
│   ├── ingest/
│   ├── metrics/
│   ├── pipeline/
│   ├── server/
│   ├── storage/
│   └── websocket/
├── pkg/
│   └── models/          # Shared domain types (ticks, candles)
├── migrations/          # SQL migrations
├── docs/                # Design and API documentation
├── .github/workflows/   # CI
├── Makefile
├── go.mod
└── README.md
```

## Getting started

**Prerequisites:** Go 1.22+, Make (optional), Docker & Docker Compose when the stack is wired up.

```bash
git clone https://github.com/vigneshprabhu/tickforge.git
cd tickforge
go mod download
make test    # or: go test ./...
make run     # placeholder server message until HTTP is implemented
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for full local setup once services are added.

## API overview

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/healthz` | Liveness |
| `GET` | `/readyz` | Readiness (e.g. DB) |
| `POST` | `/api/v1/ticks` | Ingest a tick |
| `GET` | `/api/v1/candles` | Query candles (`symbol`, `timeframe`) |
| `GET` | `/api/v1/symbols` | List known symbols |
| `GET` | `/metrics` | Prometheus metrics |
| `WebSocket` | `/ws/v1/candles` | Candle events |

Full contract: [docs/API.md](docs/API.md).

## Metrics

TickForge will expose **Prometheus** metrics such as (names subject to implementation):

- Ingestion: request rate, latency, validation failures  
- Queue: depth, drops or rejections under backpressure  
- Workers: active goroutines / utilization, task duration  
- Aggregation: candles completed, late ticks  
- Storage: query latency, errors  
- WebSocket: connected clients, broadcast failures  

## Engineering principles

- **Backpressure over unbounded memory** — bounded queues and explicit rejection or slow-down paths  
- **Observability by default** — health, readiness, and metrics for every critical stage  
- **Graceful shutdown** — drain work, flush state, close connections predictably  
- **Small, reviewable changes** — clear packages and boundaries before features land  
- **No scope creep** — infrastructure first; no trading or prediction logic in core  

## Roadmap

1. **Foundation** — module layout, docs, CI (current phase)  
2. **MVP** — ingest → pipeline → 1m OHLCV → Postgres → REST + WS + metrics  
3. **Hardening** — load tests, richer metrics, operational runbooks  
4. **Optional extensions** — additional timeframes, retention policies (as separate, scoped work)  

## Contributing

Contributions that improve reliability, clarity, tests, and documentation are welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a PR.

## License

This project is licensed under the MIT License — see [LICENSE](LICENSE).

## Disclaimer

TickForge is **educational and infrastructural software**. It is **not** financial advice, investment advice, or a recommendation to buy or sell any security. Market-like data is used **only** as a familiar example for event streaming. Use at your own risk.
