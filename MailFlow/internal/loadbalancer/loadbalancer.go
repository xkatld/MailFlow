package loadbalancer

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/mailflow/smtp-loadbalancer/internal/database"
	"github.com/mailflow/smtp-loadbalancer/internal/models"
	"github.com/mailflow/smtp-loadbalancer/internal/queue"
)

var (
	mu           sync.RWMutex
	roundRobinIdx = make(map[int]int)
)

func SelectSMTP(ctx context.Context) (*models.SMTPConfig, error) {
	var configs []models.SMTPConfig
	if err := database.DB.Where("status = ?", "active").Order("priority DESC").Find(&configs).Error; err != nil {
		return nil, fmt.Errorf("查询SMTP配置失败: %w", err)
	}

	if len(configs) == 0 {
		return nil, fmt.Errorf("没有可用的SMTP配置")
	}

	grouped := groupByPriority(configs)
	
	for _, priority := range getSortedPriorities(grouped) {
		group := grouped[priority]
		
		for i := 0; i < len(group); i++ {
			mu.Lock()
			idx := roundRobinIdx[priority] % len(group)
			roundRobinIdx[priority]++
			mu.Unlock()

			config := group[idx]
			
			if checkHourlyLimit(ctx, &config) {
				return &config, nil
			}
		}
	}

	return nil, fmt.Errorf("所有SMTP服务器都已达到小时限额")
}

func groupByPriority(configs []models.SMTPConfig) map[int][]models.SMTPConfig {
	grouped := make(map[int][]models.SMTPConfig)
	for _, config := range configs {
		grouped[config.Priority] = append(grouped[config.Priority], config)
	}
	return grouped
}

func getSortedPriorities(grouped map[int][]models.SMTPConfig) []int {
	priorities := make([]int, 0, len(grouped))
	for p := range grouped {
		priorities = append(priorities, p)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(priorities)))
	return priorities
}

func checkHourlyLimit(ctx context.Context, config *models.SMTPConfig) bool {
	hourKey := fmt.Sprintf("mailflow:smtp_hour:%d:%s", config.ID, time.Now().Format("2006-01-02-15"))
	
	hourCount, err := queue.Client.Get(ctx, hourKey).Int64()
	if err == nil && hourCount >= int64(config.MaxPerHour) {
		return false
	}
	
	if config.MaxPerDay > 0 {
		dayKey := fmt.Sprintf("mailflow:smtp_day:%d:%s", config.ID, time.Now().Format("2006-01-02"))
		dayCount, err := queue.Client.Get(ctx, dayKey).Int64()
		if err == nil && dayCount >= int64(config.MaxPerDay) {
			return false
		}
	}

	return true
}

func IncrementSMTPCount(ctx context.Context, smtpID uint) error {
	now := time.Now()
	hourKey := fmt.Sprintf("mailflow:smtp_hour:%d:%s", smtpID, now.Format("2006-01-02-15"))
	dayKey := fmt.Sprintf("mailflow:smtp_day:%d:%s", smtpID, now.Format("2006-01-02"))
	
	pipe := queue.Client.Pipeline()
	hourCmd := pipe.Incr(ctx, hourKey)
	dayCmd := pipe.Incr(ctx, dayKey)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return err
	}

	if hourCount, _ := hourCmd.Result(); hourCount == 1 {
		queue.Client.Expire(ctx, hourKey, 2*time.Hour)
	}
	if dayCount, _ := dayCmd.Result(); dayCount == 1 {
		queue.Client.Expire(ctx, dayKey, 48*time.Hour)
	}

	return nil
}

