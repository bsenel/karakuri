package valkey

// The three algorithms, in Lua.
//
// Each returns the same four integers — {allowed, remaining, reset_ms,
// retry_ms} — so the Go side decodes one shape regardless of policy. Everything
// is milliseconds: Valkey's own TTL commands work in milliseconds, and mixing
// units inside a script is how you end up with a limiter that is a thousand
// times too generous.
//
// Two rules these scripts follow, both of which matter:
//
//   - Nothing non-deterministic. No TIME, no randomness. The current instant and
//     the unique member id arrive through ARGV, because a script that produced
//     them itself could not be replicated or replayed consistently — and because
//     the Backend contract says time comes from the caller so tests can drive a
//     clock without sleeping.
//   - A refusal writes nothing. Every write sits inside the allowed branch, so
//     hammering an exhausted key cannot push its own reset further out.

// tokenBucketScript keeps {t: tokens, l: last_ms} in a hash.
//
// KEYS[1] key
// ARGV    now_ms, limit, window_ms, rate_per_ms, n, commit
const tokenBucketScript = `
local now    = tonumber(ARGV[1])
local limit  = tonumber(ARGV[2])
local window = tonumber(ARGV[3])
local rate   = tonumber(ARGV[4])
local n      = tonumber(ARGV[5])
local commit = tonumber(ARGV[6])

local st     = redis.call('HMGET', KEYS[1], 't', 'l')
local tokens = tonumber(st[1])
local last   = tonumber(st[2])
if tokens == nil or last == nil then
  tokens = limit
  last   = now
end

-- Only ever refill forward: a clock that jumps backwards must not drain the
-- bucket, and must not become the new baseline either.
if now > last then
  tokens = math.min(limit, tokens + (now - last) * rate)
  last   = now
end

local allowed = 0
local retry   = 0
-- The same nanotoken of slack the Go implementation uses, for the same reason:
-- refilling for exactly one token's worth of time can land a hair under it.
if tokens + 1e-9 >= n then
  tokens  = tokens - n
  allowed = 1
else
  retry = math.ceil((n - tokens) / rate)
end

local reset = math.ceil((limit - tokens) / rate)
if commit == 1 then
  redis.call('HSET', KEYS[1], 't', tostring(tokens), 'l', tostring(last))
  redis.call('PEXPIRE', KEYS[1], window * 2)
end
return {allowed, math.floor(tokens), now + reset, retry}
`

// fixedWindowScript keeps {ws: window_start_ms, c: count} in a hash.
//
// KEYS[1] key
// ARGV    now_ms, limit, window_ms, n, commit
const fixedWindowScript = `
local now    = tonumber(ARGV[1])
local limit  = tonumber(ARGV[2])
local window = tonumber(ARGV[3])
local n      = tonumber(ARGV[4])
local commit = tonumber(ARGV[5])

-- Anchored to the epoch, not to first use, so every replica and every restart
-- agrees on where the boundary falls.
local start = math.floor(now / window) * window
local stop  = start + window

local st    = redis.call('HMGET', KEYS[1], 'ws', 'c')
local ws    = tonumber(st[1])
local count = tonumber(st[2])
if ws == nil or count == nil or ws ~= start then
  ws    = start
  count = 0
end

local allowed = 0
local retry   = 0
if count + n <= limit then
  count   = count + n
  allowed = 1
else
  retry = stop - now
end

if commit == 1 then
  redis.call('HSET', KEYS[1], 'ws', start, 'c', count)
  redis.call('PEXPIRE', KEYS[1], window * 2)
end
return {allowed, math.max(limit - count, 0), stop, retry}
`

// slidingLogScript keeps one sorted-set member per consumption, scored by the
// instant it happened.
//
// A sorted set cannot carry a payload, so the cost travels in the member name
// as "<n>:<unique>". The unique part comes from ARGV rather than being
// generated here, both to keep the script deterministic and because two takes
// in the same millisecond must not collide into one member.
//
// KEYS[1] key
// ARGV    now_ms, limit, window_ms, n, commit, member_id
const slidingLogScript = `
local now    = tonumber(ARGV[1])
local limit  = tonumber(ARGV[2])
local window = tonumber(ARGV[3])
local n      = tonumber(ARGV[4])
local commit = tonumber(ARGV[5])
local member = ARGV[6]

local cutoff = now - window
if commit == 1 then
  redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', cutoff)
end

local entries = redis.call('ZRANGEBYSCORE', KEYS[1], '(' .. cutoff, '+inf', 'WITHSCORES')
local used    = 0
local costs   = {}
local scores  = {}
for i = 1, #entries, 2 do
  local cost = tonumber(string.match(entries[i], '^(%d+):')) or 1
  used = used + cost
  costs[#costs + 1]   = cost
  scores[#scores + 1] = tonumber(entries[i + 1])
end

local reset = now
if #scores > 0 then
  reset = scores[1] + window
end

local allowed = 0
local retry   = 0
if used + n <= limit then
  allowed = 1
  if commit == 1 and n > 0 then
    redis.call('ZADD', KEYS[1], now, n .. ':' .. member)
    redis.call('PEXPIRE', KEYS[1], window * 2)
    used = used + n
  end
else
  -- Wait until enough of the oldest entries have aged out to make room for n.
  -- Walking from the oldest is what makes this the earliest time the request
  -- would succeed rather than a conservative guess.
  local need  = used + n - limit
  local freed = 0
  for i = 1, #costs do
    freed = freed + costs[i]
    if freed >= need then
      retry = (scores[i] + window) - now
      break
    end
  end
  if retry <= 0 then
    -- n alone exceeds the limit, so no amount of waiting helps. Report the full
    -- window rather than nothing, which would invite an instant retry.
    retry = window
  end
end

return {allowed, math.max(limit - used, 0), reset, retry}
`
