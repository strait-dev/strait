package ratelimit

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type redisProcessFunc func(ctx context.Context, cmd redis.Cmder) error

type redisMockHook struct {
	process redisProcessFunc
}

func (h redisMockHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h redisMockHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if h.process != nil {
			return h.process(ctx, cmd)
		}
		return next(ctx, cmd)
	}
}

func (h redisMockHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		if h.process != nil {
			for _, cmd := range cmds {
				if err := h.process(ctx, cmd); err != nil {
					return err
				}
			}
			return nil
		}

		return next(ctx, cmds)
	}
}

func newMockRedisClient(process redisProcessFunc) *redis.Client {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	client.AddHook(redisMockHook{process: process})
	return client
}
