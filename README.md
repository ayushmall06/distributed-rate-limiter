# Distributed Rate Limiter as a Service (RLaaS)

## Problem Statement

Modern backend systems are distributed and horizontally scalable.
In such systems, implementing rate limiting inside individual services leads to:

- Inconsistent behavior across instances
- Difficulty enforcing global limits
- Code duplication
- Operational complexity

A centralized rate limiting service solves these problems by providing a **single source of truth** for request limits.

This project implements a **Distributed Rate Limiter as a Service (RLaaS)** that can be used by any backend service via HTTP.

## High Level Goals

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

## System Architecture Overview

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

## Core Concepts & Terminology

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

## Rate Limiting Algorithm Choice

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

## Redis Data Model

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

## Atomic Environment Using Redis Lua

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

## Retry-After Semantics

### Purpose

When a request is blocked, clients need to know **when to retry.**

### Behavior

- If refill rate > 0 -> compute exact wait time
- If refill rate = 0 -> retry is impossible (`retry_after_time_ms = -1`)

### Why Computed in Lua?

- Lua has authorative state
- Prevents race conditions
- Guarantees correctness

## API Design

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

## Control Plane vs Data Plane

### Control Plane

- Rule creation and management
- Low QPS
- Configuration-focused

### Data Plane

- Rate limit checks
- High QPS
- Latency-critical

This separation ensures scalability and correctness.

## Failure Handling & Edge Cases

### Redis unavailable

- Fail-open strategy (configurable later)

### Zero refill rate

- No token regeneration
- Retry-after = -1

### Input Validation

- Reject malformed requests early
- Prevent silent failures

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