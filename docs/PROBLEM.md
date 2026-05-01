# Problem statement

## The real-world problem

Many systems must process **continuous streams of events**—arriving in bursts, out of order relative to wall clocks, and at rates that vary by orders of magnitude across the day. Finance-style **ticks** (price and volume updates) are one familiar instance of that pattern, but the underlying difficulty is generic: **sustain throughput**, **bound resource usage**, and **remain observable** when the world does not cooperate.

## Why continuous event streams are hard

- **Bursty traffic** creates short-lived overloads that exhaust memory, file descriptors, or DB connections if every request is handled optimistically.  
- **Fan-out** (many producers or consumers) amplifies small inefficiencies into systemic latency.  
- **Ordering and time** are subtle: events may be processed after their logical window closes, requiring clear rules for late data.  
- **Failure is normal** at scale: partial outages, restarts, and slow dependencies must not corrupt state or hide errors.

## Why writing every event directly to a database is fragile

Databases excel at **durable, queryable state**, not at absorbing **firehoses of raw events**:

- **Write amplification** — one tick per `INSERT` can dominate IOPS and WAL growth.  
- **Schema pressure** — storing raw ticks at full rate often demands partitioning, retention jobs, and careful indexing long before the product “does anything useful.”  
- **Coupling** — ingest latency becomes DB latency; a slow commit path blocks producers with no intermediate buffer.  
- **Operational blindness** — without queue depth and worker metrics, teams only see “DB is slow” instead of **where** backpressure should have kicked in.

A healthier pattern is to **buffer**, **aggregate** (for example into OHLCV candles), and **persist derived, lower-volume artifacts** that still answer product questions.

## Why buffering, aggregation, observability, backpressure, and graceful shutdown matter

| Concern | Role |
|---------|------|
| **Buffering** | Smooths bursts and decouples ingest from processing speed. |
| **Aggregation** | Reduces volume and aligns data with human or product time windows (e.g. 1-minute candles). |
| **Observability** | Makes queue depth, drops, latency, and errors visible before users complain. |
| **Backpressure** | Signals overload explicitly (reject, shed, or slow) instead of silently degrading or OOMing. |
| **Graceful shutdown** | Finishes in-flight work, flushes state, and closes connections so deploys and crashes are predictable. |

## Why market ticks are a useful example domain

Ticks have intuitive fields (**symbol**, **price**, **volume**, **timestamp**) and natural **time bucketing** (OHLCV per interval). That makes it easy to discuss **ordering**, **windows**, and **aggregation** without domain-specific jargon from IoT or gaming.

## Broader applicability

The same architecture applies to:

- **IoT** — sensor readings batched into minute or hourly rollups  
- **Logs and telemetry** — high-cardinality events summarized into counts and histograms  
- **Payments** — transaction events consolidated into balances or settlement batches  
- **Gaming** — action streams aggregated into sessions, leaderboards, or analytics windows  

TickForge uses ticks as the **tutorial surface**; the **pipeline and operational patterns** are the reusable lesson.
