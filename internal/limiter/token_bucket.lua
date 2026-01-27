-- Token Bucket Rate Limiter (Redis Lua Script)

local key = KEYS[1]

local now = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2]) -- tokens per second
local capacity = tonumber(ARGV[3]) -- maximum tokens
local tokens_requested = tonumber(ARGV[4]) -- tokens requested

local data = redis.call("HMGET", key, "tokens", "last_refill_ts")

local tokens = tonumber(data[1])
local last_refill_ts = tonumber(data[2])

if tokens == nil then
    tokens = capacity
    last_refill_ts = now
end

-- Refill tokens based on elapsed time
local elapsed = now - last_refill_ts
local refill = elapsed * refill_rate
tokens = math.min(tokens + refill, capacity)


local allowed = 0
if tokens >= tokens_requested then
    tokens = tokens - tokens_requested
    allowed = 1
end

redis.call("HMSET", key,
    "tokens", tokens,
    "last_refill_ts", now
)

redis.call("EXPIRE", key, 120) -- Set TTL to avoid stale keys

local retry_after_ms = 0

if allowed == 0 then
    if refill_rate > 0 then
        local missing = tokens_requested - tokens
        local wait_second = missing / refill_rate
        retry_after_ms = math.ceil(wait_second * 1000)
    else
        retry_after_ms = -1 -- Indicate that no tokens will ever be available
    end
end

redis.call("HSET", key, "last_refill_ts", now)

return {allowed, tokens, retry_after_ms}