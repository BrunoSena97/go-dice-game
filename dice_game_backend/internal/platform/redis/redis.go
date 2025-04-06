package redis

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

type Config struct {
	Addr     string
	Password string
	DB       string
}

func ConnectRedis(ctx context.Context, cfg Config) (*redis.Client, error) {
	log.Printf("Connecting to Redis at %s, DB %s", cfg.Addr, cfg.DB)

	dbIndex, err := strconv.Atoi(cfg.DB)
	if err != nil {
		log.Printf("Invalid Redis DB index '%s', using default 0. Error: %v", cfg.DB, err)
		dbIndex = 0
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           dbIndex,
		PoolSize:     10,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	statusCmd := rdb.Ping(ctx)
	if err := statusCmd.Err(); err != nil {
		_ = rdb.Close()
		log.Printf("Failed to connect to Redis: %v", err)
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	log.Printf("Connected to Redis: %s", statusCmd.Val())
	return rdb, nil
}
