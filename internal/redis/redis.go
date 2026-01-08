package redis

import (
	"context"
	"fmt"
	"os"
	"time"

	redisClient "github.com/go-redis/redis/v8"
	"github.com/sukalov/mshkbot/internal/utils"
)

var Client *redisClient.Client

type RedisClient *redisClient.Client

// ValidateConnection checks if redis connection is healthy
func ValidateConnection() error {
	if Client == nil {
		return fmt.Errorf("redis client is nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := Client.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("redis connection failed: %w", err)
	}

	return nil
}

func init() {
	env, err := utils.LoadEnv([]string{"REDIS_URL", "REDIS_PASSWORD"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load redis db env %s.", err)
		os.Exit(1)
	}

	opt, err := redisClient.ParseURL(fmt.Sprintf("rediss://default:%s@%s", env["REDIS_PASSWORD"], env["REDIS_URL"]))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse redis URL: %s.", err)
		os.Exit(1)
	}

	Client = redisClient.NewClient(opt)

	// Ping to verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = Client.Ping(ctx).Result()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to redis: %s.", err)
		os.Exit(1)
	}
}

// Close closes the redis connection
func Close() {
	if Client != nil {
		Client.Close()
	}
}
