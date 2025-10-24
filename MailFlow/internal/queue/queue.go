package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/mailflow/smtp-loadbalancer/internal/config"
	"github.com/redis/go-redis/v9"
)

const QueueKey = "mailflow:email_queue"

var Client *redis.Client

type EmailTask struct {
	APIKeyID uint     `json:"api_key_id"`
	To       []string `json:"to"`
	Subject  string   `json:"subject"`
	HTML     string   `json:"html"`
	Text     string   `json:"text"`
}

func Connect(cfg *config.RedisConfig) error {
	Client = redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := Client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("连接Redis失败: %w", err)
	}

	log.Println("Redis连接成功")
	return nil
}

func PushEmail(ctx context.Context, task *EmailTask) error {
	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("序列化邮件任务失败: %w", err)
	}

	return Client.LPush(ctx, QueueKey, data).Err()
}

func PopEmail(ctx context.Context, timeout time.Duration) (*EmailTask, error) {
	result, err := Client.BRPop(ctx, timeout, QueueKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	if len(result) < 2 {
		return nil, fmt.Errorf("无效的队列数据")
	}

	var task EmailTask
	if err := json.Unmarshal([]byte(result[1]), &task); err != nil {
		return nil, fmt.Errorf("反序列化邮件任务失败: %w", err)
	}

	return &task, nil
}

func Close() error {
	if Client != nil {
		return Client.Close()
	}
	return nil
}

