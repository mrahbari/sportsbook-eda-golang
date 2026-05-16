package redisstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"learning.local/sportsbook/internal/domain"
)

type RedisOddsCache struct {
	Client *redis.Client
}

func (c *RedisOddsCache) Set(ctx context.Context, odds *domain.MarketOdds) error {
	payload, err := json.Marshal(odds)
	if err != nil {
		return err
	}
	return c.Client.Set(ctx, "odds:"+odds.MarketID, payload, 10*time.Minute).Err()
}

func (c *RedisOddsCache) Get(ctx context.Context, marketID string) (*domain.MarketOdds, error) {
	val, err := c.Client.Get(ctx, "odds:"+marketID).Result()
	if err != nil {
		return nil, err
	}

	var m domain.MarketOdds
	if err := json.Unmarshal([]byte(val), &m); err != nil {
		return nil, fmt.Errorf("redis unmarshal: %w", err)
	}
	return &m, nil
}
