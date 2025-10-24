package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mailflow/smtp-loadbalancer/internal/database"
	"github.com/mailflow/smtp-loadbalancer/internal/models"
	"github.com/mailflow/smtp-loadbalancer/internal/queue"
)

const (
	APIKeyCacheTTL = 5 * time.Minute
	RateLimitWindow = 60 * time.Second
)

type CachedAPIKey struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	MinuteLimit  int    `json:"minute_limit"`
	DailyLimit   int    `json:"daily_limit"`
	WeeklyLimit  int    `json:"weekly_limit"`
	MonthlyLimit int    `json:"monthly_limit"`
	TotalLimit   int    `json:"total_limit"`
	Status       string `json:"status"`
}

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			c.JSON(401, gin.H{"error": "缺少API Key"})
			c.Abort()
			return
		}

		key, err := validateAPIKey(c.Request.Context(), apiKey)
		if err != nil {
			c.JSON(401, gin.H{"error": err.Error()})
			c.Abort()
			return
		}

		if key.Status != "active" {
			c.JSON(403, gin.H{"error": "API Key已被禁用"})
			c.Abort()
			return
		}

		if err := checkRateLimit(c.Request.Context(), key); err != nil {
			c.JSON(429, gin.H{"error": err.Error()})
			c.Abort()
			return
		}

		c.Set("api_key_id", key.ID)
		c.Set("api_key_name", key.Name)
		c.Next()
	}
}

func validateAPIKey(ctx context.Context, apiKey string) (*CachedAPIKey, error) {
	cacheKey := fmt.Sprintf("mailflow:apikey:%s", apiKey)
	
	val, err := queue.Client.Get(ctx, cacheKey).Result()
	if err == nil {
		var cached CachedAPIKey
		if err := json.Unmarshal([]byte(val), &cached); err == nil {
			return &cached, nil
		}
	}

	var key models.APIKey
	if err := database.DB.Where("key = ?", apiKey).First(&key).Error; err != nil {
		return nil, fmt.Errorf("无效的API Key")
	}

	cached := &CachedAPIKey{
		ID:           key.ID,
		Name:         key.Name,
		MinuteLimit:  key.MinuteLimit,
		DailyLimit:   key.DailyLimit,
		WeeklyLimit:  key.WeeklyLimit,
		MonthlyLimit: key.MonthlyLimit,
		TotalLimit:   key.TotalLimit,
		Status:       key.Status,
	}

	data, _ := json.Marshal(cached)
	queue.Client.Set(ctx, cacheKey, data, APIKeyCacheTTL)

	return cached, nil
}

func checkRateLimit(ctx context.Context, key *CachedAPIKey) error {
	can, msg, err := PreCheckQuota(ctx, key)
	if err != nil {
		return err
	}
	if !can {
		return fmt.Errorf(msg)
	}
	return nil
}

func PreCheckQuota(ctx context.Context, key *CachedAPIKey) (bool, string, error) {
	now := time.Now()
	
	if key.MinuteLimit > 0 {
		minuteKey := fmt.Sprintf("mailflow:minute:%d", key.ID)
		count, _ := queue.Client.Get(ctx, minuteKey).Int64()
		if count >= int64(key.MinuteLimit) {
			return false, fmt.Sprintf("超过每分钟限制: %d，将在1分钟后恢复", key.MinuteLimit), nil
		}
	}

	if key.DailyLimit > 0 {
		dailyKey := fmt.Sprintf("mailflow:daily:%d:%s", key.ID, now.Format("2006-01-02"))
		count, _ := queue.Client.Get(ctx, dailyKey).Int64()
		if count >= int64(key.DailyLimit) {
			tomorrow := now.Add(24 * time.Hour)
			resetTime := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, tomorrow.Location())
			return false, fmt.Sprintf("超过每日限额: %d，将在%s恢复", key.DailyLimit, resetTime.Format("2006-01-02 15:04:05")), nil
		}
	}

	if key.WeeklyLimit > 0 {
		weekKey := fmt.Sprintf("mailflow:week:%d:%s", key.ID, now.Format("2006-W%V"))
		count, _ := queue.Client.Get(ctx, weekKey).Int64()
		if count >= int64(key.WeeklyLimit) {
			return false, fmt.Sprintf("超过每周限额: %d，将在下周一00:00恢复", key.WeeklyLimit), nil
		}
	}

	if key.MonthlyLimit > 0 {
		monthKey := fmt.Sprintf("mailflow:month:%d:%s", key.ID, now.Format("2006-01"))
		count, _ := queue.Client.Get(ctx, monthKey).Int64()
		if count >= int64(key.MonthlyLimit) {
			return false, fmt.Sprintf("超过每月限额: %d，将在下月1日00:00恢复", key.MonthlyLimit), nil
		}
	}

	if key.TotalLimit > 0 {
		totalKey := fmt.Sprintf("mailflow:total:%d", key.ID)
		count, _ := queue.Client.Get(ctx, totalKey).Int64()
		if count >= int64(key.TotalLimit) {
			return false, fmt.Sprintf("超过总限额: %d", key.TotalLimit), nil
		}
	}

	return true, "", nil
}

func ConsumeQuota(ctx context.Context, apiKeyID uint) error {
	now := time.Now()
	
	minuteKey := fmt.Sprintf("mailflow:minute:%d", apiKeyID)
	count, _ := queue.Client.Incr(ctx, minuteKey).Result()
	if count == 1 {
		queue.Client.Expire(ctx, minuteKey, 60*time.Second)
	}
	
	dailyKey := fmt.Sprintf("mailflow:daily:%d:%s", apiKeyID, now.Format("2006-01-02"))
	count, _ = queue.Client.Incr(ctx, dailyKey).Result()
	if count == 1 {
		queue.Client.Expire(ctx, dailyKey, 48*time.Hour)
	}
	
	weekKey := fmt.Sprintf("mailflow:week:%d:%s", apiKeyID, now.Format("2006-W%V"))
	count, _ = queue.Client.Incr(ctx, weekKey).Result()
	if count == 1 {
		queue.Client.Expire(ctx, weekKey, 8*24*time.Hour)
	}
	
	monthKey := fmt.Sprintf("mailflow:month:%d:%s", apiKeyID, now.Format("2006-01"))
	count, _ = queue.Client.Incr(ctx, monthKey).Result()
	if count == 1 {
		queue.Client.Expire(ctx, monthKey, 32*24*time.Hour)
	}
	
	totalKey := fmt.Sprintf("mailflow:total:%d", apiKeyID)
	queue.Client.Incr(ctx, totalKey)
	
	return nil
}

func GetRemainingQuota(ctx context.Context, apiKeyID uint) (map[string]interface{}, error) {
	now := time.Now()
	
	var key models.APIKey
	if err := database.DB.First(&key, apiKeyID).Error; err != nil {
		return nil, err
	}
	
	result := make(map[string]interface{})
	
	if key.MinuteLimit > 0 {
		minuteKey := fmt.Sprintf("mailflow:minute:%d", apiKeyID)
		used, _ := queue.Client.Get(ctx, minuteKey).Int64()
		ttl, _ := queue.Client.TTL(ctx, minuteKey).Result()
		result["minute"] = map[string]interface{}{
			"limit":     key.MinuteLimit,
			"used":      used,
			"remaining": int64(key.MinuteLimit) - used,
			"reset_in":  int(ttl.Seconds()),
		}
	}
	
	if key.DailyLimit > 0 {
		dailyKey := fmt.Sprintf("mailflow:daily:%d:%s", apiKeyID, now.Format("2006-01-02"))
		used, _ := queue.Client.Get(ctx, dailyKey).Int64()
		tomorrow := now.Add(24 * time.Hour)
		resetTime := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, tomorrow.Location())
		result["daily"] = map[string]interface{}{
			"limit":      key.DailyLimit,
			"used":       used,
			"remaining":  int64(key.DailyLimit) - used,
			"reset_at":   resetTime.Format("2006-01-02 15:04:05"),
			"reset_in":   int(resetTime.Sub(now).Seconds()),
		}
	}
	
	if key.WeeklyLimit > 0 {
		weekKey := fmt.Sprintf("mailflow:week:%d:%s", apiKeyID, now.Format("2006-W%V"))
		used, _ := queue.Client.Get(ctx, weekKey).Int64()
		result["weekly"] = map[string]interface{}{
			"limit":     key.WeeklyLimit,
			"used":      used,
			"remaining": int64(key.WeeklyLimit) - used,
		}
	}
	
	if key.MonthlyLimit > 0 {
		monthKey := fmt.Sprintf("mailflow:month:%d:%s", apiKeyID, now.Format("2006-01"))
		used, _ := queue.Client.Get(ctx, monthKey).Int64()
		result["monthly"] = map[string]interface{}{
			"limit":     key.MonthlyLimit,
			"used":      used,
			"remaining": int64(key.MonthlyLimit) - used,
		}
	}
	
	if key.TotalLimit > 0 {
		totalKey := fmt.Sprintf("mailflow:total:%d", apiKeyID)
		used, _ := queue.Client.Get(ctx, totalKey).Int64()
		result["total"] = map[string]interface{}{
			"limit":     key.TotalLimit,
			"used":      used,
			"remaining": int64(key.TotalLimit) - used,
		}
	}
	
	return result, nil
}

func ResetAPIKeyQuota(ctx context.Context, apiKeyID uint, quotaType string) error {
	now := time.Now()
	
	switch quotaType {
	case "minute":
		minuteKey := fmt.Sprintf("mailflow:minute:%d", apiKeyID)
		return queue.Client.Del(ctx, minuteKey).Err()
	case "daily":
		dailyKey := fmt.Sprintf("mailflow:daily:%d:%s", apiKeyID, now.Format("2006-01-02"))
		return queue.Client.Del(ctx, dailyKey).Err()
	case "weekly":
		weekKey := fmt.Sprintf("mailflow:week:%d:%s", apiKeyID, now.Format("2006-W%V"))
		return queue.Client.Del(ctx, weekKey).Err()
	case "monthly":
		monthKey := fmt.Sprintf("mailflow:month:%d:%s", apiKeyID, now.Format("2006-01"))
		return queue.Client.Del(ctx, monthKey).Err()
	case "total":
		totalKey := fmt.Sprintf("mailflow:total:%d", apiKeyID)
		return queue.Client.Del(ctx, totalKey).Err()
	case "all":
		minuteKey := fmt.Sprintf("mailflow:minute:%d", apiKeyID)
		dailyKey := fmt.Sprintf("mailflow:daily:%d:%s", apiKeyID, now.Format("2006-01-02"))
		weekKey := fmt.Sprintf("mailflow:week:%d:%s", apiKeyID, now.Format("2006-W%V"))
		monthKey := fmt.Sprintf("mailflow:month:%d:%s", apiKeyID, now.Format("2006-01"))
		totalKey := fmt.Sprintf("mailflow:total:%d", apiKeyID)
		pipe := queue.Client.Pipeline()
		pipe.Del(ctx, minuteKey)
		pipe.Del(ctx, dailyKey)
		pipe.Del(ctx, weekKey)
		pipe.Del(ctx, monthKey)
		pipe.Del(ctx, totalKey)
		_, err := pipe.Exec(ctx)
		return err
	default:
		return fmt.Errorf("无效的配额类型: %s", quotaType)
	}
}

func InvalidateAPIKeyCache(ctx context.Context, apiKey string) error {
	cacheKey := fmt.Sprintf("mailflow:apikey:%s", apiKey)
	return queue.Client.Del(ctx, cacheKey).Err()
}

