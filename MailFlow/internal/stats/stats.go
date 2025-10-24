package stats

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/mailflow/smtp-loadbalancer/internal/database"
	"github.com/mailflow/smtp-loadbalancer/internal/models"
	"github.com/mailflow/smtp-loadbalancer/internal/queue"
)

func IncrementSent(ctx context.Context, apiKeyID uint) error {
	key := fmt.Sprintf("mailflow:stats:sent:%d:%s", apiKeyID, time.Now().Format("2006-01-02"))
	return queue.Client.Incr(ctx, key).Err()
}

func IncrementFailed(ctx context.Context, apiKeyID uint) error {
	key := fmt.Sprintf("mailflow:stats:failed:%d:%s", apiKeyID, time.Now().Format("2006-01-02"))
	return queue.Client.Incr(ctx, key).Err()
}

func FlushStatsToDatabase(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			flushStats()
		}
	}
}

func flushStats() {
	date := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	flushDateStats(date)
	flushDateStats(yesterday)
}

func flushDateStats(date string) {
	pattern := fmt.Sprintf("mailflow:stats:*:*:%s", date)
	keys, err := queue.Client.Keys(context.Background(), pattern).Result()
	if err != nil {
		log.Printf("获取统计键失败: %v", err)
		return
	}

	statsMap := make(map[uint]map[string]int64)
	
	for _, key := range keys {
		parts := strings.Split(key, ":")
		if len(parts) != 5 {
			continue
		}
		
		statType := parts[2]
		var apiKeyID uint
		keyDate := parts[4]
		
		if _, err := fmt.Sscanf(parts[3], "%d", &apiKeyID); err != nil {
			continue
		}
		
		if keyDate != date {
			continue
		}

		count, err := queue.Client.Get(context.Background(), key).Int64()
		if err != nil || count == 0 {
			continue
		}

		if statsMap[apiKeyID] == nil {
			statsMap[apiKeyID] = make(map[string]int64)
		}
		statsMap[apiKeyID][statType] = count
	}

	dateTime, err := time.Parse("2006-01-02", date)
	if err != nil {
		log.Printf("日期解析失败: %v", err)
		return
	}

	for apiKeyID, counts := range statsMap {
		var stat models.UsageStats
		result := database.DB.Where("api_key_id = ? AND date = ?", apiKeyID, dateTime).First(&stat)
		
		if result.Error != nil {
			stat = models.UsageStats{
				APIKeyID: apiKeyID,
				Date:     dateTime,
			}
		}

		if sentCount, ok := counts["sent"]; ok {
			stat.SentCount = int(sentCount)
		}
		if failedCount, ok := counts["failed"]; ok {
			stat.FailedCount = int(failedCount)
		}

		if err := database.DB.Save(&stat).Error; err != nil {
			log.Printf("保存统计数据失败 [API Key ID: %d, Date: %s]: %v", apiKeyID, date, err)
		}
	}
}

type DailyStats struct {
	Date        string `json:"date"`
	SentCount   int    `json:"sent_count"`
	FailedCount int    `json:"failed_count"`
}

func GetStatsByAPIKey(apiKeyID uint, days int) ([]DailyStats, error) {
	startDate := time.Now().AddDate(0, 0, -days)
	
	var stats []models.UsageStats
	err := database.DB.Where("api_key_id = ? AND date >= ?", apiKeyID, startDate).
		Order("date DESC").
		Find(&stats).Error
	
	if err != nil {
		return nil, err
	}

	result := make([]DailyStats, len(stats))
	for i, s := range stats {
		result[i] = DailyStats{
			Date:        s.Date.Format("2006-01-02"),
			SentCount:   s.SentCount,
			FailedCount: s.FailedCount,
		}
	}

	return result, nil
}

func GetTotalStats() (map[string]interface{}, error) {
	ctx := context.Background()
	today := time.Now()
	todayStart := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	todayEnd := todayStart.AddDate(0, 0, 1)
	
	var totalSuccess, totalFailed int64
	var todaySuccessDB, todayFailedDB int64
	
	database.DB.Model(&models.SendLog{}).Where("status = ?", "success").Count(&totalSuccess)
	database.DB.Model(&models.SendLog{}).Where("status = ?", "failed").Count(&totalFailed)
	
	database.DB.Model(&models.SendLog{}).
		Where("status = ? AND created_at >= ? AND created_at < ?", "success", todayStart, todayEnd).
		Count(&todaySuccessDB)
	
	database.DB.Model(&models.SendLog{}).
		Where("status = ? AND created_at >= ? AND created_at < ?", "failed", todayStart, todayEnd).
		Count(&todayFailedDB)
	
	var redisTodaySuccess, redisTodayFailed int64
	pattern := fmt.Sprintf("mailflow:stats:*:*:%s", today.Format("2006-01-02"))
	keys, err := queue.Client.Keys(ctx, pattern).Result()
	if err == nil {
		for _, key := range keys {
			count, err := queue.Client.Get(ctx, key).Int64()
			if err != nil || count == 0 {
				continue
			}
			
			if strings.Contains(key, ":sent:") {
				redisTodaySuccess += count
			} else if strings.Contains(key, ":failed:") {
				redisTodayFailed += count
			}
		}
	}
	
	todaySuccess := todaySuccessDB + redisTodaySuccess
	todayFailed := todayFailedDB + redisTodayFailed
	todayTotal := todaySuccess + todayFailed
	totalCount := totalSuccess + totalFailed

	return map[string]interface{}{
		"total_success": totalSuccess,
		"total_failed":  totalFailed,
		"total_count":   totalCount,
		"today_success": todaySuccess,
		"today_failed":  todayFailed,
		"today_total":   todayTotal,
	}, nil
}

type APIKeyStats struct {
	APIKeyID    uint   `json:"api_key_id"`
	APIKeyName  string `json:"api_key_name"`
	TodaySent   int64  `json:"today_sent"`
	TodayFailed int64  `json:"today_failed"`
	TotalSent   int64  `json:"total_sent"`
	TotalFailed int64  `json:"total_failed"`
	TotalUsed   int    `json:"total_used"`
}

func GetAPIKeyStats() ([]APIKeyStats, error) {
	ctx := context.Background()
	today := time.Now()
	todayStart := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	todayEnd := todayStart.AddDate(0, 0, 1)
	todayStr := today.Format("2006-01-02")
	
	var keys []models.APIKey
	if err := database.DB.Order("created_at DESC").Find(&keys).Error; err != nil {
		return nil, err
	}
	
	result := make([]APIKeyStats, 0, len(keys))
	
	for _, key := range keys {
		stat := APIKeyStats{
			APIKeyID:   key.ID,
			APIKeyName: key.Name,
			TotalUsed:  key.TotalUsed,
		}
		
		database.DB.Model(&models.SendLog{}).
			Where("api_key_id = ? AND status = ?", key.ID, "success").
			Count(&stat.TotalSent)
		
		database.DB.Model(&models.SendLog{}).
			Where("api_key_id = ? AND status = ?", key.ID, "failed").
			Count(&stat.TotalFailed)
		
		database.DB.Model(&models.SendLog{}).
			Where("api_key_id = ? AND status = ? AND created_at >= ? AND created_at < ?", 
				key.ID, "success", todayStart, todayEnd).
			Count(&stat.TodaySent)
		
		database.DB.Model(&models.SendLog{}).
			Where("api_key_id = ? AND status = ? AND created_at >= ? AND created_at < ?", 
				key.ID, "failed", todayStart, todayEnd).
			Count(&stat.TodayFailed)
		
		sentKey := fmt.Sprintf("mailflow:stats:sent:%d:%s", key.ID, todayStr)
		if count, err := queue.Client.Get(ctx, sentKey).Int64(); err == nil && count > 0 {
			stat.TodaySent += count
		}
		
		failedKey := fmt.Sprintf("mailflow:stats:failed:%d:%s", key.ID, todayStr)
		if count, err := queue.Client.Get(ctx, failedKey).Int64(); err == nil && count > 0 {
			stat.TodayFailed += count
		}
		
		result = append(result, stat)
	}
	
	return result, nil
}

func getWeekStart(t time.Time) time.Time {
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return time.Date(t.Year(), t.Month(), t.Day()-weekday+1, 0, 0, 0, 0, t.Location())
}

func getMonthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

func getAPIKeyUsage(ctx context.Context, apiKeyID uint) map[string]int64 {
	now := time.Now()
	usage := make(map[string]int64)
	
	minuteKey := fmt.Sprintf("mailflow:minute:%d", apiKeyID)
	if count, err := queue.Client.Get(ctx, minuteKey).Int64(); err == nil {
		usage["minute"] = count
	}
	
	dailyKey := fmt.Sprintf("mailflow:daily:%d:%s", apiKeyID, now.Format("2006-01-02"))
	if count, err := queue.Client.Get(ctx, dailyKey).Int64(); err == nil {
		usage["daily"] = count
	}
	
	weekKey := fmt.Sprintf("mailflow:week:%d:%s", apiKeyID, now.Format("2006-W%V"))
	if count, err := queue.Client.Get(ctx, weekKey).Int64(); err == nil {
		usage["week"] = count
	}
	
	monthKey := fmt.Sprintf("mailflow:month:%d:%s", apiKeyID, now.Format("2006-01"))
	if count, err := queue.Client.Get(ctx, monthKey).Int64(); err == nil {
		usage["month"] = count
	}
	
	totalKey := fmt.Sprintf("mailflow:total:%d", apiKeyID)
	if count, err := queue.Client.Get(ctx, totalKey).Int64(); err == nil {
		usage["total"] = count
	}
	
	return usage
}

func getSMTPCurrentUsage(ctx context.Context, smtpID uint) int64 {
	hourKey := fmt.Sprintf("mailflow:smtp_hour:%d:%s", smtpID, time.Now().Format("2006-01-02-15"))
	count, _ := queue.Client.Get(ctx, hourKey).Int64()
	return count
}

type PeriodStats struct {
	Total       int64   `json:"total"`
	Success     int64   `json:"success"`
	Failed      int64   `json:"failed"`
	SuccessRate float64 `json:"success_rate"`
}

func GetPeriodStats(period string) (*PeriodStats, error) {
	ctx := context.Background()
	now := time.Now()
	var startTime, endTime time.Time
	
	switch period {
	case "today":
		startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		endTime = startTime.AddDate(0, 0, 1)
	case "week":
		startTime = getWeekStart(now)
		endTime = startTime.AddDate(0, 0, 7)
	case "month":
		startTime = getMonthStart(now)
		endTime = startTime.AddDate(0, 1, 0)
	case "all":
		startTime = time.Time{}
		endTime = now.AddDate(0, 0, 1)
	default:
		startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		endTime = startTime.AddDate(0, 0, 1)
	}
	
	var success, failed int64
	
	if period == "all" {
		database.DB.Model(&models.SendLog{}).Where("status = ?", "success").Count(&success)
		database.DB.Model(&models.SendLog{}).Where("status = ?", "failed").Count(&failed)
	} else {
		database.DB.Model(&models.SendLog{}).
			Where("status = ? AND created_at >= ? AND created_at < ?", "success", startTime, endTime).
			Count(&success)
		database.DB.Model(&models.SendLog{}).
			Where("status = ? AND created_at >= ? AND created_at < ?", "failed", startTime, endTime).
			Count(&failed)
	}
	
	if period == "today" {
		dateStr := now.Format("2006-01-02")
		pattern := fmt.Sprintf("mailflow:stats:*:*:%s", dateStr)
		keys, err := queue.Client.Keys(ctx, pattern).Result()
		if err == nil {
			for _, key := range keys {
				count, err := queue.Client.Get(ctx, key).Int64()
				if err == nil && count > 0 {
					if strings.Contains(key, ":sent:") {
						success += count
					} else if strings.Contains(key, ":failed:") {
						failed += count
					}
				}
			}
		}
	}
	
	total := success + failed
	var successRate float64
	if total > 0 {
		successRate = float64(success) / float64(total) * 100
	}
	
	return &PeriodStats{
		Total:       total,
		Success:     success,
		Failed:      failed,
		SuccessRate: successRate,
	}, nil
}

type LimitInfo struct {
	Limit   int     `json:"limit"`
	Used    int64   `json:"used"`
	Percent float64 `json:"percent"`
}

type APIKeyDetailStats struct {
	APIKeyID     uint                 `json:"api_key_id"`
	Name         string               `json:"name"`
	Limits       map[string]LimitInfo `json:"limits"`
	TodaySuccess int64                `json:"today_success"`
	TodayFailed  int64                `json:"today_failed"`
	WeekTotal    int64                `json:"week_total"`
	MonthTotal   int64                `json:"month_total"`
	TotalUsed    int                  `json:"total_used"`
}

func GetAPIKeyDetailStats() ([]APIKeyDetailStats, error) {
	ctx := context.Background()
	now := time.Now()
	
	var keys []models.APIKey
	if err := database.DB.Order("created_at DESC").Find(&keys).Error; err != nil {
		return nil, err
	}
	
	result := make([]APIKeyDetailStats, 0, len(keys))
	
	for _, key := range keys {
		usage := getAPIKeyUsage(ctx, key.ID)
		
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		todayEnd := todayStart.AddDate(0, 0, 1)
		weekStart := getWeekStart(now)
		weekEnd := weekStart.AddDate(0, 0, 7)
		monthStart := getMonthStart(now)
		monthEnd := monthStart.AddDate(0, 1, 0)
		
		var todaySuccess, todayFailed, weekTotal, monthTotal int64
		
		database.DB.Model(&models.SendLog{}).
			Where("api_key_id = ? AND status = ? AND created_at >= ? AND created_at < ?",
				key.ID, "success", todayStart, todayEnd).Count(&todaySuccess)
		
		database.DB.Model(&models.SendLog{}).
			Where("api_key_id = ? AND status = ? AND created_at >= ? AND created_at < ?",
				key.ID, "failed", todayStart, todayEnd).Count(&todayFailed)
		
		database.DB.Model(&models.SendLog{}).
			Where("api_key_id = ? AND created_at >= ? AND created_at < ?",
				key.ID, weekStart, weekEnd).Count(&weekTotal)
		
		database.DB.Model(&models.SendLog{}).
			Where("api_key_id = ? AND created_at >= ? AND created_at < ?",
				key.ID, monthStart, monthEnd).Count(&monthTotal)
		
		todayStr := now.Format("2006-01-02")
		sentKey := fmt.Sprintf("mailflow:stats:sent:%d:%s", key.ID, todayStr)
		if count, err := queue.Client.Get(ctx, sentKey).Int64(); err == nil && count > 0 {
			todaySuccess += count
		}
		
		failedKey := fmt.Sprintf("mailflow:stats:failed:%d:%s", key.ID, todayStr)
		if count, err := queue.Client.Get(ctx, failedKey).Int64(); err == nil && count > 0 {
			todayFailed += count
		}
		
		limits := make(map[string]LimitInfo)
		
		if key.MinuteLimit > 0 {
			used := usage["minute"]
			limits["minute"] = LimitInfo{
				Limit:   key.MinuteLimit,
				Used:    used,
				Percent: float64(used) / float64(key.MinuteLimit) * 100,
			}
		}
		
		if key.DailyLimit > 0 {
			used := usage["daily"]
			limits["daily"] = LimitInfo{
				Limit:   key.DailyLimit,
				Used:    used,
				Percent: float64(used) / float64(key.DailyLimit) * 100,
			}
		}
		
		if key.WeeklyLimit > 0 {
			used := usage["week"]
			limits["weekly"] = LimitInfo{
				Limit:   key.WeeklyLimit,
				Used:    used,
				Percent: float64(used) / float64(key.WeeklyLimit) * 100,
			}
		}
		
		if key.MonthlyLimit > 0 {
			used := usage["month"]
			limits["monthly"] = LimitInfo{
				Limit:   key.MonthlyLimit,
				Used:    used,
				Percent: float64(used) / float64(key.MonthlyLimit) * 100,
			}
		}
		
		if key.TotalLimit > 0 {
			used := usage["total"]
			limits["total"] = LimitInfo{
				Limit:   key.TotalLimit,
				Used:    used,
				Percent: float64(used) / float64(key.TotalLimit) * 100,
			}
		}
		
		result = append(result, APIKeyDetailStats{
			APIKeyID:     key.ID,
			Name:         key.Name,
			Limits:       limits,
			TodaySuccess: todaySuccess,
			TodayFailed:  todayFailed,
			WeekTotal:    weekTotal,
			MonthTotal:   monthTotal,
			TotalUsed:    key.TotalUsed,
		})
	}
	
	return result, nil
}

type SMTPStatsInfo struct {
	SMTPID       uint    `json:"smtp_id"`
	Name         string  `json:"name"`
	CurrentUsed  int64   `json:"current_used"`
	HourlyLimit  int     `json:"hourly_limit"`
	UsagePercent float64 `json:"usage_percent"`
	TodaySent    int64   `json:"today_sent"`
	TodayFailed  int64   `json:"today_failed"`
	Status       string  `json:"status"`
}

func GetSMTPStats() ([]SMTPStatsInfo, error) {
	ctx := context.Background()
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	todayEnd := todayStart.AddDate(0, 0, 1)
	
	var configs []models.SMTPConfig
	if err := database.DB.Order("priority DESC, created_at DESC").Find(&configs).Error; err != nil {
		return nil, err
	}
	
	result := make([]SMTPStatsInfo, 0, len(configs))
	
	for _, config := range configs {
		currentUsed := getSMTPCurrentUsage(ctx, config.ID)
		usagePercent := float64(0)
		if config.MaxPerHour > 0 {
			usagePercent = float64(currentUsed) / float64(config.MaxPerHour) * 100
		}
		
		var status string
		if usagePercent >= 100 {
			status = "已满"
		} else if usagePercent >= 90 {
			status = "接近限额"
		} else {
			status = "正常"
		}
		
		var todaySent, todayFailed int64
		database.DB.Model(&models.SendLog{}).
			Where("smtp_config_id = ? AND status = ? AND created_at >= ? AND created_at < ?",
				config.ID, "success", todayStart, todayEnd).Count(&todaySent)
		
		database.DB.Model(&models.SendLog{}).
			Where("smtp_config_id = ? AND status = ? AND created_at >= ? AND created_at < ?",
				config.ID, "failed", todayStart, todayEnd).Count(&todayFailed)
		
		result = append(result, SMTPStatsInfo{
			SMTPID:       config.ID,
			Name:         config.Name,
			CurrentUsed:  currentUsed,
			HourlyLimit:  config.MaxPerHour,
			UsagePercent: usagePercent,
			TodaySent:    todaySent,
			TodayFailed:  todayFailed,
			Status:       status,
		})
	}
	
	return result, nil
}

type TrendData struct {
	Labels  []string `json:"labels"`
	Success []int64  `json:"success"`
	Failed  []int64  `json:"failed"`
}

func GetHistoricalTrend(startDate, endDate string, apiKeyID uint) (*TrendData, error) {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, fmt.Errorf("无效的开始日期")
	}
	
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, fmt.Errorf("无效的结束日期")
	}
	
	end = end.AddDate(0, 0, 1)
	
	var stats []models.UsageStats
	query := database.DB.Where("date >= ? AND date < ?", start, end)
	
	if apiKeyID > 0 {
		query = query.Where("api_key_id = ?", apiKeyID)
	}
	
	if err := query.Order("date ASC").Find(&stats).Error; err != nil {
		return nil, err
	}
	
	dateMap := make(map[string]struct {
		success int64
		failed  int64
	})
	
	for _, stat := range stats {
		dateStr := stat.Date.Format("2006-01-02")
		entry := dateMap[dateStr]
		entry.success += int64(stat.SentCount)
		entry.failed += int64(stat.FailedCount)
		dateMap[dateStr] = entry
	}
	
	labels := []string{}
	success := []int64{}
	failed := []int64{}
	
	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		labels = append(labels, dateStr)
		
		if entry, ok := dateMap[dateStr]; ok {
			success = append(success, entry.success)
			failed = append(failed, entry.failed)
		} else {
			success = append(success, 0)
			failed = append(failed, 0)
		}
	}
	
	return &TrendData{
		Labels:  labels,
		Success: success,
		Failed:  failed,
	}, nil
}

