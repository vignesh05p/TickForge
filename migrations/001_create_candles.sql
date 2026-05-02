-- Migration: 001_create_candles
-- Purpose:   Create the primary durable candle store for TickForge MVP.
--
-- Design decisions (see docs/ARCHITECTURE.md for rationale):
--   1. volume is DOUBLE PRECISION (not BIGINT) to support fractional asset
--      quantities used in crypto and forex (e.g. 0.0051 BTC).
--   2. UNIQUE(symbol, timeframe, start_time) enforces exactly one candle per
--      minute window per symbol.  The storage layer MUST use an upsert
--      (INSERT ... ON CONFLICT (symbol, timeframe, start_time) DO UPDATE)
--      to handle idempotent writes from workers and safe restarts.
--   3. start_time / end_time are stored WITH TIME ZONE and always UTC so that
--      time-range queries are portable and unambiguous.

CREATE TABLE IF NOT EXISTS candles (
    id          BIGSERIAL        PRIMARY KEY,
    symbol      TEXT             NOT NULL,
    timeframe   TEXT             NOT NULL,         -- e.g. '1m'
    open        DOUBLE PRECISION NOT NULL,
    high        DOUBLE PRECISION NOT NULL,
    low         DOUBLE PRECISION NOT NULL,
    close       DOUBLE PRECISION NOT NULL,
    volume      DOUBLE PRECISION NOT NULL,         -- fractional-safe
    start_time  TIMESTAMPTZ      NOT NULL,         -- inclusive, UTC minute boundary
    end_time    TIMESTAMPTZ      NOT NULL,         -- exclusive, start_time + 1m
    created_at  TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ      NOT NULL DEFAULT NOW(),

    -- Enforce one candle per (symbol, timeframe, minute window).
    -- Storage layer uses ON CONFLICT (symbol, timeframe, start_time) DO UPDATE
    -- to handle intra-minute updates and safe restarts without duplicates.
    CONSTRAINT uq_candles_symbol_timeframe_start UNIQUE (symbol, timeframe, start_time)
);

-- Index for the primary candle query: latest N candles for a symbol+timeframe.
CREATE INDEX IF NOT EXISTS idx_candles_symbol_timeframe_start
    ON candles (symbol, timeframe, start_time DESC);
