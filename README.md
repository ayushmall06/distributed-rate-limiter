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

## 13. Testing Strategy

### Motivation

Correctness is a distributed rate limiter cannot be guaranteed through manual testing alone. Subtle bugs in state handling, time calculations, or Redis interactions can lead to incorrect enforcement or outages.

To ensure long-term correctness and safe refactoring, the system includes both **unit tests** and **integration tests**.

### Testing Philosophy

The testing strategy follows these principles:

- **Behavior over implementation**
- **Real dependencies for stateful logic**
- **Fail fast on setup errors**
- **Clear separation between deterministic and stateful tests**

Redis and Lua behavior is treated as a **black box** and validated through observable inputs and outputs.

### Unit Tests

Unit tests validate **deterministic, pure logic** that does not depend on extenal systems.

#### Covered Areas

- Redis key construction
- Rule key formatting
- Helper utilities
- Configuration edge cases

#### Characteristics

- Fast execution
- No external dependencies
- Fully deterministic
- Suitable for frequent execution

Unit tests protect against accidental changes that could silently break Redis key consistency or rule lookups.

### Integration Tests

Integration tests validate the **full Redis + Lua execution path**.

These tests ensure:

- Atomic token bucket enforcement
- Correct token consumption
- Correct refill behavior over time
- Correct handling of zero refill rate
- Correct retry-after semantics

Integration tests intentionally use a **real Redis instance** to surface issues that mocks would hide.

### Redis Dependency Model

Integration tests require a running Redis instance.

If Redis is unavailable, tests fail explicitly with a clear error message. This behavior is intentional and mirrors productions dependencies.

This approach ensures:

- Realistic validation
- Early detection of integration issues
- No false confidence from mocked state

### Lua Scirpt Embedding

Lua scripts are embedded directly into the Go binary using `go:embed`.

#### Rationale

- Eliminates filesystem path dependencies
- Ensures consistent behavior across:
  - Local development
  - Tests
  - CI
  - Docker
  - Production

Embedding guarantees that the exact smae script is executed in all environments.

### Failure Handling in Tests

Test setup failures (e.g., Redis unavailable, limiter initialization failure)
cause immediate test termination.

This prevents:

- Nil pointer dereferences
- Misleading test results
- Masking of root causes

Fail-fast behavior is considered mandatory for integration tests.

### What is Explicitly Not Tested

The following are intentionally excluded from unit tests:

- Redis internal behavior
- Lua runtime internals
- Go Redis client internals

These components are assumed correct and validated indirectly through integration tests.

## 14. Alerting Configuration & File Orgranization

### Purpose

Alerting rules are part of the **operational configuration** of the rate limiter.
They define _when humans should be notified_ based on system behavior observed through metrics.

Alert rules are **not application code** and are therefore kept separate from the Go source tree.

### Repository Layout for Alerting

Alert rules are stored in a dedicated top-level directory:

```go
distributed-rate-limiter/
├── alerts/
│   └── rate_limiter_alerts.yml
├── cmd/
│   └── server/
├── internal/
├── DESIGN.md
├── go.mod
└── go.sum
```

This layout intentionally separates:

| **Category** | **Location** | **Responsibility** |
|--------------|--------------|--------------------|
| Application Code | `cmd/`, `internal` | Runtime Behavior |
| Alert Rules | `alerts/` | Operational Monitoring |
| Documentation | `DESIGN.md` | System design & decisions |

### Why Alerts are Kept Outside Application Code

Alert rules are:

- Evaluated by **Prometheus**, not by application
- Changed independently of code deployments
- Owned jointly by **engineering and operations**

Keeping alerts outside the Go codebase ensures:

- Clear separation of concerns
- No coupling between runtime logic and monitoring policy
- Safer operational changes without rebuilding binaries

### Alert Rule File: `rate_limiter_alerts.yml`

The file `alerts/rate_limiter_alerts.yml` contains all Prometheus alert rules related to the rate limiter.

These rules monitor:

- Service availability
- Internal errors
- Abnormal traffic patterns
- Latency regressions

The file is written in **Prometheus-native YML format** and is intended to be loaded by a Prometheus server.

### How Alert Rules Are Used in Production

In a real deployment, Prometheus is configured to load alert rules using:

```yaml
rule_files:
  - "alerts/*.yaml"
```

Prometheus:

1. Scrapes metrics from `/metrics`
2. Evaluates alerts rules periodically
3. Fires alerts when conditions are met
4. Forwards alerts to AlertManager for notification

The rate limiter service itself is **not aware of alert rules**.

### Responsibily Boundaries

| **Component** | **Responsibility** |
|---------------|--------------------|
|Rate Limiter Service | Emit Metrics |
| Prometheus | Scrape emtrics and evaluate rules |
| Alert Rules | Define failure conditions |
| Alertmanager | Notify humans (Slack, PagerDuty, etc.) |

This separation ensures the service remains **stateless and focused**, while observability concerns are handled externally.

### Current Scope

For this project:

- Alert rules are **defined and version-controlled**
- Prometheus and Alertmanager are **not run locally**
- The focus is on **design correctness**, not deployment

This mirrors how alerting is often introduced incrementally in real systems.

### Future Enhancements (Planned)

Planned improvements to alerting include:

- Per-tenant alert thresholds
- Environment-specific alert tuning
- Alert routing by severity
- Integration with on-call rotation tools


## 15. Rate-Limit HTTP Headers

The rate limiter exposes standard HTTP headers to communicate rate-limit state to clients in a lightweight and interoperable manner.

### Headers Exposed

- `X-RateLimit-Limit` : Maximum number of tokens allowed in the bucket.
- `X-RateLimit-Remaining` : Number of tokenms remaining after the current request.
- `X-RateLimit-Retry-After-Ms` : Time (in milliseconds) the client should wait before retrying when blocked.

### Design Rationale

Rate-limit headers allow clients, proxies, and API gateways to implement backoff and retry logic without parsing response bodies.

Headers are set at the HTTP layer and are independent of the underlying rate-limiting algorithm.

### Scope

Headers are currently exposes for token bucket enforcement and may be extended to support additional algorithms in the future.

## 16. Load Testing & Observed Behavior

### Purpose

Load testing was conducted to validate the rate limiter's behavior under realistic traffic patterns and to inform both **scalability decisions** and **dashboard design.**

Unlike synthetic correctness tests, these load tests focused on:

- Sustained traffic behavior
- Burst handling
- Latency under correction
- Error visibility
- Metrics suitability for end-user dashboards

### Test Environment

- Single-region deployment
- Single Redis instance
- Token Bucket algorithm (Redis + Lua)
- Metrics collected via Prometheus endpoint
- Traffic generated using `hey`

The goal was to evaluate **baseline system behavior**, not maximum throughput.

### Scenario A - Steady Traffic

#### Description

A steady stream of requests was generated for a single tenant and resource, simulating normal application usage exceeding the configured rate limit.

#### Observation

- Total requests significantly exceeded allowed capacity
- Blocked requests dominated total traffic
- Token bucket enforcement behaved deterministically
- Latency remained low for most requests
- A small number of internal errors were observed
- Unexpected resource values appeared in metrics

#### Key Metrics Observed

- High blocked ratio (~99%)
- Latency remained within acceptable bounds (sub~10ms tail)
- Internal error counter incremented under load
- Metics revealed unintended resource cardinality

#### Interpretation

The high blocked ration indicates a **misalignment between configured rate limits and observed traffic**, not a system failure.

The system successfully protected downstream services but highlighted the need for visibility into rule effectiveness.

Unexpected resource values demonstrate the importance of strict validation and anomaly detection in real-world usage.

### Implications for Dashboard Design

Load testing directly informed the initial dashboard feature set.

### Required Dashboard Capabilities

Based on observed behavior, the dashboard mush surface:

- Allowed vs blocked request ratios
- Blocked percentage over time
- Per-tenant and per-resource traffic
- Latency percentiles (p50, p95, p99)
- Internal error indicators
- Recently observed or unknown resources

These insights are critical for users to:

- Understand rule effectiveness
- Detect misconfiguration
- Identify abuse or anomalies
- Trust system performance

### Operational Learnings

Load testing revealed several important operational realities:

- Rate limiting correctness alone is sufficient without visibility
- Misconfigured limits can appear as system failures without context
- Real traffic introduces unexpected resource patterns
- Metrics cardinality must be monitored carefully
- Alerts on internal errors are justified even at low counts

These findings reinforce the importance of **observability-first design.**

### Design Decisions Influenced by Load Testing

As a direct result of load testing:

- Strict resource validation is prioritized 
- Blocked ratio is treated as a first-class metric
- Dashboard design is driven by real traffic behavior
- Sharding decisions are deferred until multi-key load tests
- Alert thresholds are validated against real data

### Current Limitations Identified

The following limitations were intentionally accepted at this stage:

- Single Redis instance
- Single-region deployment
- Hot-key-dominant traffic patterns
- No authentication or user isolation

These limitations are addressed in subsequent phases.

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
