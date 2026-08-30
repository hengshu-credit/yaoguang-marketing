package realtimecache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var reserveSlidingWindowScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local cutoff = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]
local ttl = tonumber(ARGV[5])

redis.call('ZREMRANGEBYSCORE', key, '-inf', cutoff)
local existing = redis.call('ZSCORE', key, member)
if existing then
  local count = redis.call('ZCARD', key)
  redis.call('PEXPIRE', key, ttl)
  return {1, count, 0}
end

local count = redis.call('ZCARD', key)
if count >= limit then
  local earliest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
  local retry_after = 0
  if earliest[2] then
    retry_after = math.max(0, tonumber(earliest[2]) + ttl - now)
  end
  redis.call('PEXPIRE', key, ttl)
  return {0, count, retry_after}
end

redis.call('ZADD', key, now, member)
redis.call('PEXPIRE', key, ttl)
return {1, count + 1, 0}
`)

var reserveFrequencyWindowsScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local member = ARGV[2]
local all_existing = 1

for i, key in ipairs(KEYS) do
  local offset = 2 + ((i - 1) * 3)
  local cutoff = tonumber(ARGV[offset + 1])
  redis.call('ZREMRANGEBYSCORE', key, '-inf', cutoff)
  if not redis.call('ZSCORE', key, member) then
    all_existing = 0
  end
end

if all_existing == 1 then
  return {1, 0, 0, 1}
end

for i, key in ipairs(KEYS) do
  local offset = 2 + ((i - 1) * 3)
  local limit = tonumber(ARGV[offset + 2])
  local ttl = tonumber(ARGV[offset + 3])
  local count = redis.call('ZCARD', key)
  if count >= limit then
    local earliest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
    local retry_after = 0
    if earliest[2] then
      retry_after = math.max(0, tonumber(earliest[2]) + ttl - now)
    end
    return {0, i, count, retry_after}
  end
end

for i, key in ipairs(KEYS) do
  local offset = 2 + ((i - 1) * 3)
  local ttl = tonumber(ARGV[offset + 3])
  redis.call('ZADD', key, now, member)
  redis.call('PEXPIRE', key, ttl)
end
return {1, 0, 0, 0}
`)

type RedisStore struct {
	client redis.UniversalClient
}

func NewRedisStore(client redis.UniversalClient) (*RedisStore, error) {
	if client == nil {
		return nil, errors.New("redis client is required")
	}
	return &RedisStore{client: client}, nil
}

func NewRedisStoreFromOptions(options *redis.Options) (*RedisStore, error) {
	if options == nil || options.Addr == "" {
		return nil, errors.New("redis address is required")
	}
	return NewRedisStore(redis.NewClient(options))
}

func (s *RedisStore) Close() error {
	return s.client.Close()
}

func (s *RedisStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

func (s *RedisStore) SetTrigger(
	ctx context.Context,
	workspaceID, automationID string,
	entry TriggerEntry,
	ttl time.Duration,
) error {
	if ttl <= 0 {
		return errors.New("trigger cache ttl must be positive")
	}
	if entry.AutomationVersion <= 0 {
		return errors.New("trigger cache automation version must be positive")
	}
	if !json.Valid(entry.Payload) {
		return errors.New("trigger cache payload must be valid json")
	}
	key, err := TriggerKey(workspaceID, automationID, entry.AutomationVersion)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal trigger cache entry: %w", err)
	}
	if err := s.client.Set(ctx, key, encoded, ttl).Err(); err != nil {
		return fmt.Errorf("write trigger cache: %w", err)
	}
	return nil
}

func (s *RedisStore) GetTrigger(
	ctx context.Context,
	workspaceID, automationID string,
	automationVersion int,
) (TriggerEntry, bool, error) {
	key, err := TriggerKey(workspaceID, automationID, automationVersion)
	if err != nil {
		return TriggerEntry{}, false, err
	}
	encoded, err := s.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return TriggerEntry{}, false, nil
	}
	if err != nil {
		return TriggerEntry{}, false, fmt.Errorf("read trigger cache: %w", err)
	}
	var entry TriggerEntry
	if err := json.Unmarshal(encoded, &entry); err != nil {
		return TriggerEntry{}, false, fmt.Errorf("decode trigger cache: %w", err)
	}
	if entry.AutomationVersion != automationVersion || !json.Valid(entry.Payload) {
		return TriggerEntry{}, false, errors.New("trigger cache entry version or payload is invalid")
	}
	return entry, true, nil
}

func (s *RedisStore) ReserveSlidingWindow(
	ctx context.Context,
	workspaceID, subjectID, channel, policyID, reservationID string,
	now time.Time,
	window time.Duration,
	maxEvents int,
) (WindowResult, error) {
	if window <= 0 || maxEvents <= 0 {
		return WindowResult{}, errors.New("frequency window and max events must be positive")
	}
	if reservationID == "" {
		return WindowResult{}, errors.New("frequency reservation id is required")
	}
	key, err := FrequencyKey(workspaceID, subjectID, channel, policyID)
	if err != nil {
		return WindowResult{}, err
	}
	nowMillis := now.UTC().UnixMilli()
	windowMillis := window.Milliseconds()
	if windowMillis <= 0 {
		return WindowResult{}, errors.New("frequency window must be at least one millisecond")
	}
	values, err := reserveSlidingWindowScript.Run(
		ctx, s.client, []string{key},
		nowMillis, nowMillis-windowMillis, maxEvents, reservationID, windowMillis,
	).Slice()
	if err != nil {
		return WindowResult{}, fmt.Errorf("reserve frequency window: %w", err)
	}
	if len(values) != 3 {
		return WindowResult{}, fmt.Errorf("reserve frequency window returned %d values", len(values))
	}
	allowed, err := redisInteger(values[0])
	if err != nil {
		return WindowResult{}, err
	}
	count, err := redisInteger(values[1])
	if err != nil {
		return WindowResult{}, err
	}
	retryMillis, err := redisInteger(values[2])
	if err != nil {
		return WindowResult{}, err
	}
	return WindowResult{
		Allowed: allowed == 1, Count: int(count),
		RetryAfter: time.Duration(retryMillis) * time.Millisecond,
	}, nil
}

func (s *RedisStore) ReserveWindows(
	ctx context.Context,
	workspaceID, subjectID, channel, reservationID string,
	now time.Time,
	windows []WindowReservation,
) (MultiWindowResult, error) {
	if reservationID == "" || len(windows) == 0 {
		return MultiWindowResult{}, errors.New("frequency reservation id and windows are required")
	}
	keys := make([]string, 0, len(windows))
	args := make([]interface{}, 0, 2+len(windows)*3)
	args = append(args, now.UTC().UnixMilli(), reservationID)
	for _, window := range windows {
		if window.PolicyID == "" || window.Window <= 0 || window.MaxEvents <= 0 {
			return MultiWindowResult{}, errors.New("frequency policy id, window and max events are required")
		}
		key, err := FrequencyKey(workspaceID, subjectID, channel, window.PolicyID)
		if err != nil {
			return MultiWindowResult{}, err
		}
		start := window.Start.UTC()
		if start.IsZero() {
			start = now.UTC().Add(-window.Window)
		}
		keys = append(keys, key)
		args = append(args, start.UnixMilli(), window.MaxEvents, window.Window.Milliseconds())
	}
	values, err := reserveFrequencyWindowsScript.Run(ctx, s.client, keys, args...).Slice()
	if err != nil {
		return MultiWindowResult{}, fmt.Errorf("reserve frequency windows: %w", err)
	}
	if len(values) != 4 {
		return MultiWindowResult{}, fmt.Errorf("reserve frequency windows returned %d values", len(values))
	}
	allowed, err := redisInteger(values[0])
	if err != nil {
		return MultiWindowResult{}, err
	}
	deniedIndex, err := redisInteger(values[1])
	if err != nil {
		return MultiWindowResult{}, err
	}
	count, err := redisInteger(values[2])
	if err != nil {
		return MultiWindowResult{}, err
	}
	retryMillis, err := redisInteger(values[3])
	if err != nil {
		return MultiWindowResult{}, err
	}
	result := MultiWindowResult{Allowed: allowed == 1, Count: int(count), RetryAfter: time.Duration(retryMillis) * time.Millisecond}
	if result.Allowed {
		result.Replayed = deniedIndex == 0 && count == 0 && retryMillis == 1
	} else if deniedIndex > 0 && int(deniedIndex) <= len(windows) {
		result.DeniedPolicyID = windows[deniedIndex-1].PolicyID
	}
	return result, nil
}

func redisInteger(value interface{}) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	default:
		return 0, fmt.Errorf("unexpected redis integer type %T", value)
	}
}
