package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Seraf-seraf/tasks/internal/domain"
	repository "github.com/Seraf-seraf/tasks/internal/repository/mysql"
	"github.com/go-redis/redis/v8"
)

type TaskCache struct {
	redis *redis.Client
}

func NewTaskCache(rc *redis.Client) *TaskCache {
	return &TaskCache{redis: rc}
}

func (c *TaskCache) Get(ctx context.Context, teamID int64, status string, assignee *int64, limit, offset int) ([]domain.Task, bool) {
	if c.redis == nil {
		return nil, false
	}
	key := repository.CacheKey(teamID, status, assignee, limit, offset)
	b, err := c.redis.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	var out []domain.Task
	if json.Unmarshal(b, &out) != nil {
		return nil, false
	}
	return out, true
}

func (c *TaskCache) Set(ctx context.Context, teamID int64, status string, assignee *int64, limit, offset int, tasks []domain.Task) {
	if c.redis == nil {
		return
	}
	key := repository.CacheKey(teamID, status, assignee, limit, offset)
	if b, err := json.Marshal(tasks); err == nil {
		_ = c.redis.Set(ctx, key, b, 5*time.Minute).Err()
	}
}

func (c *TaskCache) Invalidate(ctx context.Context, teamID int64) {
	if c.redis == nil {
		return
	}
	it := c.redis.Scan(ctx, 0, fmt.Sprintf("tasks:%d:*", teamID), 100).Iterator()
	for it.Next(ctx) {
		_ = c.redis.Del(ctx, it.Val()).Err()
	}
}
