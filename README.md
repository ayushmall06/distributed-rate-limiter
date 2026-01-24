# Distributed Rate Limiter as a Service (RLaaS)

## Problem Statement
Modern backend systems are horizontally scaled and distributed.
Traditional in-memory rate limiting fails under such environments,
while naive centralized approaches become bottlenecks or single points of failure.

Teams often re-implement rate limiting logic per service, leading to
inconsistent behavior, operational complexity, and duplicated effort.

RLaaS provides a centralized, scalable, and configurable rate limiting
service that can be used by any service via a simple API.

## Goals
- Low latency enforcement (P99 < 10ms)
- Horizontally scalable API layer
- Strong consistency for rate enforcement (MVP)
- Simple integration via HTTP/gRPC

## Non-Goals (v1)
- Multi-region enforcement
- UI dashboard
- Advanced anomaly detection
- Perfect global consistency under network partitions

## Architecture (MVP)
Client → RLaaS API → Redis

## Rate Limiting Algorithm
- Token Bucket

## Terminology

### Tenant
An organization or service using RLaaS.  
Example: `search-service`

### Resource
A protected API or logical endpoint.  
Examples:
- `/search`
- `/api/v1/login`
- `checkout`

### Key
The entity being rate-limited.  
Examples:
- `user_id`
- `api_key`
- `ip_address`

### Rule
Defines rate limiting behavior for a tenant and resource.

Attributes:
- tenant_id
- resource
- limit
- window_sec
- burst

### Bucket
Runtime state used for enforcement.

Attributes:
- tokens
- last_refill_timestamp

## API Design

### Rate Limit Check (Hot Path)

**POST** `/v1/ratelimit/check`

## Request:
```json
{
  "tenant_id": "search-service",
  "resource": "/search",
  "key": "user_123",
  "tokens": 1
}

Response:
{
  "allowed": true,
  "remaining": 42,
  "retry_after_ms": 0
}

```
## Notes:
- tokens supports weighted requests
- No rule evaluation logic on the client

## Rule Management (Control Plane)
POST /v1/rules
GET /v1/rules/{tenant_id}
DELETE /v1/rules/{rule_id}

Rule Example:
{
  "tenant_id": "search-service",
  "resource": "/search",
  "limit": 100,
  "window_sec": 60,
  "burst": 20
}

# Failure & Consistency Decisions
## Redis Unavailable
- Fail-open (requests are allowed)
- Reason: availability > strict enforcement
## No Rule Found
- Allow request
- Emit metric for observability
## Time Source
- Redis server time is authoritative
- Client timestamps are never trusted
