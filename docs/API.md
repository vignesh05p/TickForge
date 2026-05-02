# API contract

Base URL example: `http://localhost:8080`. All JSON uses UTF-8. Timestamps are **RFC3339** UTC unless noted.

## Error response format

Failed requests return JSON:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Human-readable explanation",
    "details": {}
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `error.code` | string | Stable machine-readable code |
| `error.message` | string | Safe to show to API clients |
| `error.details` | object | Optional key-value context |

HTTP status codes follow semantics (e.g. `400` validation, `404` not found, `429`/`503` overload).

---

## GET /healthz

**Purpose:** Liveness: process is up.

**Response** `200 OK`:

```json
{
  "status": "ok"
}
```

---

## GET /readyz

**Purpose:** Readiness: process is able to accept traffic.

**Response** `200 OK`:

```json
{
  "status": "ready"
}
```

**Response** `503 Service Unavailable` when not ready (same `error` envelope as above).

Phase 1 uses an in-process readiness check. PostgreSQL readiness will be wired when
storage lands in the MVP phase.

---

## POST /api/v1/ticks

**Purpose:** Ingest a single tick.

**Request headers:** `Content-Type: application/json`

**Request body example:**

```json
{
  "symbol": "INFY",
  "price": 1485.50,
  "volume": 100,
  "timestamp": "2026-05-01T10:30:02Z"
}
```

**Response** `202 Accepted` or `201 Created` (final choice at implementation):

```json
{
  "accepted": true
}
```

**Validation rules (MVP):**

| Field | Rules |
|-------|--------|
| `symbol` | Required; non-empty string; allowed charset TBD (e.g. alphanumeric + `.`); max length enforced |
| `price` | Required; finite number `> 0` |
| `volume` | Required; integer `>= 0` |
| `timestamp` | Required; valid RFC3339; may reject “too far” future/past per config |

---

## GET /api/v1/candles

**Purpose:** Query candles.

**Query parameters:**

| Param | Required | Example | Description |
|-------|----------|---------|-------------|
| `symbol` | yes | `INFY` | Instrument symbol |
| `timeframe` | yes | `1m` | MVP: `1m` only |

**Example:** `GET /api/v1/candles?symbol=INFY&timeframe=1m`

**Response** `200 OK`:

```json
{
  "candles": [
    {
      "symbol": "INFY",
      "timeframe": "1m",
      "open": 1480.00,
      "high": 1490.25,
      "low": 1478.40,
      "close": 1485.50,
      "volume": 12500,
      "start_time": "2026-05-01T10:30:00Z",
      "end_time": "2026-05-01T10:30:59Z"
    }
  ]
}
```

Semantics for “latest only” vs “range” TBD; MVP should document default limit and ordering.

---

## GET /api/v1/symbols

**Purpose:** List symbols for which candle data exists (or is actively aggregated).

**Response** `200 OK`:

```json
{
  "symbols": ["INFY", "RELIANCE"]
}
```

---

## GET /metrics

**Purpose:** Prometheus exposition format.

**Response:** `text/plain; version=0.0.4` (Prometheus default).

Example (illustrative only):

```
# HELP tickforge_ingest_requests_total Total tick ingest requests
# TYPE tickforge_ingest_requests_total counter
tickforge_ingest_requests_total{status="accepted"} 42
```

---

## WebSocket /ws/v1/candles

**Purpose:** Subscribe to candle events.

**Handshake:** `GET` with `Upgrade: websocket`. Subprotocol and query parameters (e.g. `symbol=INFY`) TBD at implementation.

**Server → client event** (completed candle), JSON:

```json
{
  "type": "candle",
  "data": {
    "symbol": "INFY",
    "timeframe": "1m",
    "open": 1480.00,
    "high": 1490.25,
    "low": 1478.40,
    "close": 1485.50,
    "volume": 12500,
    "start_time": "2026-05-01T10:30:00Z",
    "end_time": "2026-05-01T10:30:59Z"
  }
}
```

Optional `type` values (e.g. heartbeat) may be added; clients should ignore unknown types.

---

## Schemas

### Tick

| Field | JSON type | Description |
|-------|-----------|-------------|
| `symbol` | string | Ticker / instrument id |
| `price` | number | Last or trade price |
| `volume` | number (integer) | Volume in tick units |
| `timestamp` | string | Event time, RFC3339 |

Example:

```json
{
  "symbol": "INFY",
  "price": 1485.50,
  "volume": 100,
  "timestamp": "2026-05-01T10:30:02Z"
}
```

### Candle

| Field | JSON type | Description |
|-------|-----------|-------------|
| `symbol` | string | Instrument |
| `timeframe` | string | e.g. `1m` |
| `open` | number | Open price |
| `high` | number | High price |
| `low` | number | Low price |
| `close` | number | Close price |
| `volume` | number | Total volume in window |
| `start_time` | string | Window start, RFC3339 |
| `end_time` | string | Window end, RFC3339 |

Example:

```json
{
  "symbol": "INFY",
  "timeframe": "1m",
  "open": 1480.00,
  "high": 1490.25,
  "low": 1478.40,
  "close": 1485.50,
  "volume": 12500,
  "start_time": "2026-05-01T10:30:00Z",
  "end_time": "2026-05-01T10:30:59Z"
}
```

Health and readiness are implemented in Phase 1. The ingestion, candle query,
symbols, metrics, and WebSocket contracts remain planned for the MVP phase.
