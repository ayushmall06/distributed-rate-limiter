# Distributed Rate Limiter as a Service (RLaaS)

## 1. Problem Statement

Modern backend systems are distributed and horizontally scalable.
In such systems, implementing rate limiting inside individual services leads to:

- Inconsistent behavior across instances
- Difficulty enforcing global limits
- Code duplication
- Operational complexity

A centralized rate limiting service solves these problems by providing a **single source of truth** for request limits.

This project implements a **Distributed Rate Limiter as a Service (RLaaS)** that can be used by any backend service via HTTP.

## 2. High Level Goals

- Enforce request limits **consistently across distributed service**
- Support **burst traffic** while preventing abuse
- Provide **client friendly responses** (retry hints)
- Separate **configuration (control plane)** from **request enforcement (data plane)**
- Be restart-safe and horizontally scalable

### Explicit Non-Goals (Current Version)

- Multi-region replication
- UI Dashboard
- Authentication/ authorization
- Per-user quotas across multiple resources

## 3. System Architecture Overview

```graph
Client
  |
  | HTTP request
  |
  v
RLaaS API (Go)
  |
  | Redis Lua Script (atomic)
  |
  v
Redis (shared state)
```

### Key architectural principles

- Stateless service layer
- State stored externally (Redis)
- Atomic enforcement using Lua
- Clear separation of responsibilities

## 4. Core Concepts & Terminology

### Tenant

A logical owner of rate limits (e.g. `payments`, `search`).

### Resource

The protected endpoint or operation (e.g. `/charge`, `/search`).

### Key

The entity being rate-limited (e.g. user ID, API key).

### Rule

Configuration defining how requests are limited:

- Capacity
- Refill rate

### Bucket

Runtime state used by the tocken bucket algorithm:

- `token`
- `last_refill_ts`

## 5. Rate Limiting Algorithm Choice

### Algorithm Used: Token Bucket

#### Why Token Bucket ?

- Allows short bursts of traffic
- Smooths request rate over time
- Widely used in production systems
- Client-friendly behavior

#### Core Idea

- A bucket has a maximum capacity
- Tokens refill at a fixed rate
- Each request consumes tokens
- Requests are rejected if insufficient tokens exists

## 6. Redis Data Model

### Rate Limit Buckets

#### Redis Key Format

```note
rl:{tenant}:{resource}:{key}
```

#### Redis Value (Hash)

```note
tokens
last_refill_ts
```

#### TTL

- Buckets expire automatically after inactivity
- Prevents unbounded memory growth

### Rules Storage

#### Redis Key Format (Rule)

```note
rule:{tenant}:{resource}
```

#### Redis Value (Hash) (Rule)

```note
capcaity
refill_rate
```

Rules are stored persistently and survive service restarts.

## 7. Atomic Environment Using Redis Lua

### Why Lua?

- Redis executes Lua scripts atomically
- Prevents race conditions
- Combines read-modify-write safely

### Lua Responsibilities

1. Load existing bucket state
2. Refill tokens based on elapsed time
3. Check token availability
4. Update bucket state
5. Compute retry time if blocked

### Key Insight

> All time-sensitive state logic lives inside Redis

## 8. Retry-After Semantics

### Purpose

When a request is blocked, clients need to know **when to retry.**

### Behavior

- If refill rate > 0 -> compute exact wait time
- If refill rate = 0 -> retry is impossible (`retry_after_time_ms = -1`)

### Why Computed in Lua?

- Lua has authorative state
- Prevents race conditions
- Guarantees correctness

## 9. API Design

### Rate Limit Check (Data Plane)

#### Endpoint

```Note
POST /v1/ratelimit/check
```

#### Request

```note
{
  "tenant_id": "payments",
  "resource": "/charge",
  "key": "user1",
  "tokens_requested": 1
}
```

#### Response

```note
{
  "allowed": true,
  "remaining": 4,
  "retry_after_ms": 0
}
```

### Rule Management (Control Plane)

#### Endpoints

```note
POST /v1/rules
GET /v1/rules
```

Rules can be added dynamically without restarting the service.

## 10. Control Plane vs Data Plane

### Control Plane

- Rule creation and management
- Low QPS
- Configuration-focused

### Data Plane

- Rate limit checks
- High QPS
- Latency-critical

This separation ensures scalability and correctness.

## 11. Failure Handling & Edge Cases

### Redis unavailable

- Fail-open strategy (configurable later)

### Zero refill rate

- No token regeneration
- Retry-after = -1

### Input Validation

- Reject malformed requests early
- Prevent silent failures

## 12. Observability & Metrics

### Motivation

In a production rate limiter, correctness alone is insufficient.

Operators must be able to answer questions such as:

- How many requests are being rate limited ?
- Which tenants or resources are being throttled ?
- Is the limiter introducing latency ?
- Are internal errors occuring ?

To support this, the system exposes **Prometheus-compatible metrics** that provide real-time visibility into rate-limiting behavior.

### Metric Philosophy

The system follows these observability principles:

- **Metrics over logs** for alerting and trend analysis
- **Event-based counters** instead of per-request logs
- **Low cardinality labels** to avoid metric explosion
- **Clear separation** between business metrics and runtime metrics

Metrics are collected only for **valid, processed requests** to ensure accuracy.

### Metics Exposed

The following core metrics are exposed:

#### Request Metrocs

| **Metic Name** | **Type** | **Labels** | **Description** |
|----------------|----------|------------|-----------------|
| `rate_limit_requests_total` | Counter | tenant, resource | Total number of rate limit checks |
| `rate_limit_allowed_total` | Counter | tenant, resource | Total number of allowed requests |
| `rate_limit_blocked_total` | Counter | tenant, resource | Total number of blocked requests |

These metrics enable:

- Traffic volume analysis
- Abuse detection
- Rule effectiveness validation

#### Error Metrics

| **Metric Name** | **Type** | **Labels** | **Description** |
|-----------------|----------|------------|-----------------|
|`rate_limit_errors_total`| Counter | none | Total number of internal limiter errors |

This metric is used to:

- Detect Redis or Lua execution failures
- Alert on system instablility

#### Latency Metrics

| **Metric Name** | **Type** | **Labels** | **Description** |
|-----------------|----------|------------|-----------------|
|`rate_limit_latency_ms`| Histogram | tenant, resource | End-to-end latency of rate limit checks |

Latency is measured at the HTTP handler level and includes:

- Request validation
- Redis round-trip
- Lua script execution

### Metrics Endpoint

The service exposes a Prometheus scrape endpoint:
```bash
GET /metrics
```

This endpoint returns:

- Go runtime metrics (GC, memory, goroutines)
- Custom rate limiter analytics

The endpoint is designed to be scraped periodically by Prometheus.

### Metric Lifecycle

A metric becomes visible only after it is **observed atleast once**.

For example:

- `rate_limit_requests_total` appears only after the first successful rate-limit check
- Runtime metrics appear immediately on startup

This behavior is consistent with Prometheus client semantics.

### Correct Instrumentaion Semantics

Metrics are recorded using the following ordering:

1. Request validation
2. Request counting
3. Rate limit enforcement
4. Allowed/blocked classification
5. Latency observation

This ensures:

- Invlid requests are not counted
- Errors are traced separately
- Latency reflects real limiter behavior

### Operational Use Cases

This exposed metrics enable:

- **Alerting**
  - Sudden spikes in blocked requests
  - Increase in internal errors
- **Capacity planning**
  - Identifying hot tenants/resources
- **Debugging**
  - Correlating latency with Redis behavior

## Observed Debugging Learnings

During developement, several real-world issues were encoutered:

- Silent JSON field mismatches
- Lua-Go integration mismatches
- Token bucket refill masking consumption
- Redis state visibility issues

These reflect **actual production debugging scenarios**, not toy problems.

## Current Limitations

- Single Redis instance
- No metics
- No authentication
- No unit tests
- No multi-region support

These are **intentional**, not oversights.

## Roadmap (Planned Enhancements)

1. Prometheus metrics
2. Unit and Integration tests
3. Multiple rate limiting algorithms
4. Rate-limit headers
5. Redis clustering
6. Multi-region support

## Summary

This project demonstrates:

- Distributed system thinking
- Correct use of Redis and Lua
- Clear API contracts
- Control plane vs data place separation
- Real-world debugging and correctness

It is designed to evolve incrementally into a production-grade system.
