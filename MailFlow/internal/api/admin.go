package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mailflow/smtp-loadbalancer/internal/auth"
	"github.com/mailflow/smtp-loadbalancer/internal/config"
	"github.com/mailflow/smtp-loadbalancer/internal/database"
	"github.com/mailflow/smtp-loadbalancer/internal/models"
	"github.com/mailflow/smtp-loadbalancer/internal/queue"
	smtphealth "github.com/mailflow/smtp-loadbalancer/internal/smtp"
	"github.com/mailflow/smtp-loadbalancer/internal/stats"
)

func RegisterAdminAPI(r *gin.Engine, cfg *config.Config) {
	admin := r.Group("/admin/api")
	admin.Use(AdminAuthMiddleware(cfg))
	{
		admin.GET("/plans", listPlans)
		admin.POST("/plans", createPlan)
		admin.PUT("/plans/:id", updatePlan)
		admin.DELETE("/plans/:id", deletePlan)
		admin.PUT("/plans/:id/toggle", togglePlanStatus)
		
		admin.GET("/keys", listAPIKeys)
		admin.POST("/keys", createAPIKey)
		admin.PUT("/keys/:id", updateAPIKey)
		admin.DELETE("/keys/:id", deleteAPIKey)
		admin.GET("/keys/:id/quota", getAPIKeyQuota)
		admin.POST("/keys/:id/reset-quota", resetAPIKeyQuota)
		admin.POST("/keys/:id/adjust-quota", adjustAPIKeyQuota)
		admin.POST("/keys/batch-delete", batchDeleteAPIKeys)
		admin.POST("/keys/batch-status", batchUpdateAPIKeysStatus)

		admin.GET("/smtp-configs", listSMTPConfigs)
		admin.POST("/smtp-configs", createSMTPConfig)
		admin.POST("/smtp-configs/batch-import", batchImportSMTPConfigs)
		admin.PUT("/smtp-configs/:id", updateSMTPConfig)
		admin.DELETE("/smtp-configs/:id", deleteSMTPConfig)
		admin.POST("/smtp-configs/:id/test", testSMTPConfig)
		admin.POST("/smtp-configs/:id/pause", pauseSMTPConfig)
		admin.POST("/smtp-configs/:id/resume", resumeSMTPConfig)
		admin.POST("/smtp-configs/:id/reset-quota", resetSMTPQuota)
		admin.GET("/smtp-configs/:id/health", getSMTPHealth)
		admin.POST("/smtp-configs/batch-test", batchTestSMTPConfigs)
		admin.POST("/smtp-configs/batch-delete", batchDeleteSMTPConfigs)
		admin.POST("/smtp-configs/batch-status", batchUpdateSMTPConfigsStatus)

		admin.GET("/stats", getStats)
		admin.GET("/stats/period", getPeriodStats)
		admin.GET("/key-stats", getKeyStats)
		admin.GET("/key-stats-detail", getKeyStatsDetail)
		admin.GET("/smtp-stats", getSMTPStats)
		admin.GET("/trend", getTrend)
		admin.GET("/logs", getLogs)
		
		admin.GET("/admin-tokens", listAdminTokens)
		admin.POST("/admin-tokens", createAdminToken)
		admin.DELETE("/admin-tokens/:id", deleteAdminToken)
		admin.PUT("/admin-tokens/:id/toggle", toggleAdminToken)
	}
}

func listPlans(c *gin.Context) {
	var plans []models.Plan
	if err := database.DB.Order("sort_order ASC, created_at ASC").Find(&plans).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, plans)
}

func createPlan(c *gin.Context) {
	var plan models.Plan
	if err := c.ShouldBindJSON(&plan); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	var existing models.Plan
	if err := database.DB.Where("code = ?", plan.Code).First(&existing).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "套餐代码已存在"})
		return
	}

	if err := database.DB.Create(&plan).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建失败"})
		return
	}

	c.JSON(http.StatusOK, plan)
}

func updatePlan(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	var plan models.Plan
	if err := database.DB.First(&plan, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "套餐不存在"})
		return
	}

	var req models.Plan
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	if req.Code != plan.Code {
		var existing models.Plan
		if err := database.DB.Where("code = ? AND id != ?", req.Code, id).First(&existing).Error; err == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "套餐代码已存在"})
			return
		}
	}

	plan.Code = req.Code
	plan.Name = req.Name
	plan.Description = req.Description
	plan.MinuteLimit = req.MinuteLimit
	plan.DailyLimit = req.DailyLimit
	plan.WeeklyLimit = req.WeeklyLimit
	plan.MonthlyLimit = req.MonthlyLimit
	plan.IsActive = req.IsActive
	plan.SortOrder = req.SortOrder

	if err := database.DB.Save(&plan).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, plan)
}

func deletePlan(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	var count int64
	if err := database.DB.Model(&models.APIKey{}).Where("plan_id = ?", id).Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}

	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该套餐正在被使用，无法删除"})
		return
	}

	if err := database.DB.Delete(&models.Plan{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

func togglePlanStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	var plan models.Plan
	if err := database.DB.First(&plan, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "套餐不存在"})
		return
	}

	plan.IsActive = !plan.IsActive
	if err := database.DB.Save(&plan).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, plan)
}

func listAPIKeys(c *gin.Context) {
	var keys []models.APIKey
	if err := database.DB.Order("created_at DESC").Find(&keys).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, keys)
}

func createAPIKey(c *gin.Context) {
	var req struct {
		Name         string `json:"name" binding:"required"`
		PlanID       *uint  `json:"plan_id"`
		MinuteLimit  *int   `json:"minute_limit"`
		DailyLimit   *int   `json:"daily_limit"`
		WeeklyLimit  *int   `json:"weekly_limit"`
		MonthlyLimit *int   `json:"monthly_limit"`
		TotalLimit   int    `json:"total_limit"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	key := models.APIKey{
		Key:        uuid.New().String(),
		Name:       req.Name,
		TotalLimit: req.TotalLimit,
		TotalUsed:  0,
		Status:     "active",
	}

	if req.PlanID != nil {
		var plan models.Plan
		if err := database.DB.First(&plan, *req.PlanID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "套餐不存在"})
			return
		}
		
		key.PlanID = req.PlanID
		key.Plan = plan.Code
		key.IsCustom = false
		key.MinuteLimit = plan.MinuteLimit
		key.DailyLimit = plan.DailyLimit
		key.WeeklyLimit = plan.WeeklyLimit
		key.MonthlyLimit = plan.MonthlyLimit
	} else {
		if req.MinuteLimit == nil || req.DailyLimit == nil || req.WeeklyLimit == nil || req.MonthlyLimit == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "自定义配置需要提供所有限额参数"})
			return
		}
		
		key.PlanID = nil
		key.Plan = "custom"
		key.IsCustom = true
		key.MinuteLimit = *req.MinuteLimit
		key.DailyLimit = *req.DailyLimit
		key.WeeklyLimit = *req.WeeklyLimit
		key.MonthlyLimit = *req.MonthlyLimit
	}

	if err := database.DB.Create(&key).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建失败"})
		return
	}

	c.JSON(http.StatusOK, key)
}

func updateAPIKey(c *gin.Context) {
	id := c.Param("id")
	
	var key models.APIKey
	if err := database.DB.First(&key, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "API Key不存在"})
		return
	}

	var req struct {
		Name         *string `json:"name"`
		PlanID       *uint   `json:"plan_id"`
		IsCustom     *bool   `json:"is_custom"`
		MinuteLimit  *int    `json:"minute_limit"`
		DailyLimit   *int    `json:"daily_limit"`
		WeeklyLimit  *int    `json:"weekly_limit"`
		MonthlyLimit *int    `json:"monthly_limit"`
		TotalLimit   *int    `json:"total_limit"`
		Status       *string `json:"status"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	if req.Name != nil {
		key.Name = *req.Name
	}

	if req.PlanID != nil {
		var plan models.Plan
		if err := database.DB.First(&plan, *req.PlanID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "套餐不存在"})
			return
		}
		
		key.PlanID = req.PlanID
		key.Plan = plan.Code
		key.IsCustom = false
		key.MinuteLimit = plan.MinuteLimit
		key.DailyLimit = plan.DailyLimit
		key.WeeklyLimit = plan.WeeklyLimit
		key.MonthlyLimit = plan.MonthlyLimit
	} else if req.IsCustom != nil && *req.IsCustom {
		if req.MinuteLimit == nil || req.DailyLimit == nil || req.WeeklyLimit == nil || req.MonthlyLimit == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "自定义配置需要提供所有限额参数"})
			return
		}
		
		key.PlanID = nil
		key.Plan = "custom"
		key.IsCustom = true
		key.MinuteLimit = *req.MinuteLimit
		key.DailyLimit = *req.DailyLimit
		key.WeeklyLimit = *req.WeeklyLimit
		key.MonthlyLimit = *req.MonthlyLimit
	} else {
		if req.MinuteLimit != nil {
			key.MinuteLimit = *req.MinuteLimit
		}
		if req.DailyLimit != nil {
			key.DailyLimit = *req.DailyLimit
		}
		if req.WeeklyLimit != nil {
			key.WeeklyLimit = *req.WeeklyLimit
		}
		if req.MonthlyLimit != nil {
			key.MonthlyLimit = *req.MonthlyLimit
		}
	}

	if req.TotalLimit != nil {
		key.TotalLimit = *req.TotalLimit
	}
	if req.Status != nil {
		key.Status = *req.Status
	}

	if err := database.DB.Save(&key).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}

	auth.InvalidateAPIKeyCache(c.Request.Context(), key.Key)

	c.JSON(http.StatusOK, key)
}

func deleteAPIKey(c *gin.Context) {
	id := c.Param("id")
	
	if err := database.DB.Delete(&models.APIKey{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

func listSMTPConfigs(c *gin.Context) {
	var configs []models.SMTPConfig
	if err := database.DB.Order("priority DESC, created_at DESC").Find(&configs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, configs)
}

func createSMTPConfig(c *gin.Context) {
	var config models.SMTPConfig

	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	if config.MaxPerHour == 0 {
		config.MaxPerHour = 100
	}
	if config.Priority == 0 {
		config.Priority = 1
	}
	config.Status = "active"

	if err := database.DB.Create(&config).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建失败"})
		return
	}

	c.JSON(http.StatusOK, config)
}

func updateSMTPConfig(c *gin.Context) {
	id := c.Param("id")
	
	var config models.SMTPConfig
	if err := database.DB.First(&config, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "SMTP配置不存在"})
		return
	}

	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	if err := database.DB.Save(&config).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, config)
}

func deleteSMTPConfig(c *gin.Context) {
	id := c.Param("id")
	
	if err := database.DB.Delete(&models.SMTPConfig{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

func getStats(c *gin.Context) {
	totalStats, err := stats.GetTotalStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取统计失败"})
		return
	}

	c.JSON(http.StatusOK, totalStats)
}

func getKeyStats(c *gin.Context) {
	keyStats, err := stats.GetAPIKeyStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取API Key统计失败"})
		return
	}

	c.JSON(http.StatusOK, keyStats)
}

func getPeriodStats(c *gin.Context) {
	period := c.DefaultQuery("type", "today")
	
	periodStats, err := stats.GetPeriodStats(period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取时间段统计失败"})
		return
	}

	c.JSON(http.StatusOK, periodStats)
}

func getKeyStatsDetail(c *gin.Context) {
	keyStatsDetail, err := stats.GetAPIKeyDetailStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取API Key详细统计失败"})
		return
	}

	c.JSON(http.StatusOK, keyStatsDetail)
}

func getSMTPStats(c *gin.Context) {
	smtpStats, err := stats.GetSMTPStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取SMTP统计失败"})
		return
	}

	c.JSON(http.StatusOK, smtpStats)
}

func getTrend(c *gin.Context) {
	startDate := c.Query("start")
	endDate := c.Query("end")
	apiKeyID, _ := strconv.ParseUint(c.DefaultQuery("key_id", "0"), 10, 32)
	
	if startDate == "" || endDate == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少开始或结束日期"})
		return
	}
	
	trendData, err := stats.GetHistoricalTrend(startDate, endDate, uint(apiKeyID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, trendData)
}

func getLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	status := c.Query("status")
	search := c.Query("search")
	
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 500 {
		pageSize = 50
	}

	query := database.DB.Model(&models.SendLog{})
	
	if status != "" {
		query = query.Where("status = ?", status)
	}
	
	if search != "" {
		query = query.Where("\"to\" ILIKE ? OR subject ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	var total int64
	query.Count(&total)

	var logs []models.SendLog
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"data":      logs,
	})
}

func batchDeleteAPIKeys(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	if err := database.DB.Delete(&models.APIKey{}, req.IDs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "批量删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "批量删除成功"})
}

func batchUpdateAPIKeysStatus(c *gin.Context) {
	var req struct {
		IDs    []uint `json:"ids" binding:"required"`
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	if err := database.DB.Model(&models.APIKey{}).Where("id IN ?", req.IDs).Update("status", req.Status).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "批量更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "批量更新成功"})
}

func batchImportSMTPConfigs(c *gin.Context) {
	var configs []struct {
		Name       string `json:"name" binding:"required"`
		Host       string `json:"host" binding:"required"`
		Port       int    `json:"port" binding:"required"`
		Username   string `json:"username" binding:"required"`
		Password   string `json:"password" binding:"required"`
		AuthMethod string `json:"auth_method"`
		Encryption string `json:"encryption"`
		FromEmail  string `json:"from_email" binding:"required"`
		FromName   string `json:"from_name"`
		Priority   int    `json:"priority"`
		MaxPerHour int    `json:"max_per_hour"`
	}

	if err := c.ShouldBindJSON(&configs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的JSON格式"})
		return
	}

	var smtpConfigs []models.SMTPConfig
	for _, cfg := range configs {
		authMethod := cfg.AuthMethod
		if authMethod == "" {
			authMethod = "plain"
		}
		encryption := cfg.Encryption
		if encryption == "" {
			encryption = "starttls"
		}
		smtpConfigs = append(smtpConfigs, models.SMTPConfig{
			Name:       cfg.Name,
			Host:       cfg.Host,
			Port:       cfg.Port,
			Username:   cfg.Username,
			Password:   cfg.Password,
			AuthMethod: authMethod,
			Encryption: encryption,
			FromEmail:  cfg.FromEmail,
			FromName:   cfg.FromName,
			Priority:   cfg.Priority,
			MaxPerHour: cfg.MaxPerHour,
			Status:     "active",
		})
	}

	if err := database.DB.Create(&smtpConfigs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "批量导入失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "批量导入成功",
		"count":   len(smtpConfigs),
	})
}

func batchDeleteSMTPConfigs(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	if err := database.DB.Delete(&models.SMTPConfig{}, req.IDs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "批量删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "批量删除成功"})
}

func batchUpdateSMTPConfigsStatus(c *gin.Context) {
	var req struct {
		IDs    []uint `json:"ids" binding:"required"`
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	if err := database.DB.Model(&models.SMTPConfig{}).Where("id IN ?", req.IDs).Update("status", req.Status).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "批量更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "批量更新成功"})
}

func getAPIKeyQuota(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	quota, err := auth.GetRemainingQuota(c.Request.Context(), uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配额失败"})
		return
	}

	c.JSON(http.StatusOK, quota)
}

func resetAPIKeyQuota(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	var req struct {
		QuotaType string `json:"quota_type" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	if err := auth.ResetAPIKeyQuota(c.Request.Context(), uint(id), req.QuotaType); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "配额重置成功"})
}

func adjustAPIKeyQuota(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	var req struct {
		MinuteLimit  *int `json:"minute_limit"`
		DailyLimit   *int `json:"daily_limit"`
		WeeklyLimit  *int `json:"weekly_limit"`
		MonthlyLimit *int `json:"monthly_limit"`
		TotalLimit   *int `json:"total_limit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	var key models.APIKey
	if err := database.DB.First(&key, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "API Key不存在"})
		return
	}

	if req.MinuteLimit != nil && *req.MinuteLimit >= 0 {
		key.MinuteLimit = *req.MinuteLimit
	}
	if req.DailyLimit != nil && *req.DailyLimit >= 0 {
		key.DailyLimit = *req.DailyLimit
	}
	if req.WeeklyLimit != nil && *req.WeeklyLimit >= 0 {
		key.WeeklyLimit = *req.WeeklyLimit
	}
	if req.MonthlyLimit != nil && *req.MonthlyLimit >= 0 {
		key.MonthlyLimit = *req.MonthlyLimit
	}
	if req.TotalLimit != nil && *req.TotalLimit >= 0 {
		key.TotalLimit = *req.TotalLimit
	}

	if err := database.DB.Save(&key).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "调整配额失败"})
		return
	}

	auth.InvalidateAPIKeyCache(c.Request.Context(), key.Key)

	c.JSON(http.StatusOK, gin.H{
		"message": "配额调整成功",
		"key":     key,
	})
}

func testSMTPConfig(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	var config models.SMTPConfig
	if err := database.DB.First(&config, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "SMTP配置不存在"})
		return
	}

	if err := smtphealth.TestSMTPConnection(&config); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "连接失败",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "连接成功",
	})
}

func pauseSMTPConfig(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	var config models.SMTPConfig
	if err := database.DB.First(&config, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "SMTP配置不存在"})
		return
	}

	config.Status = "paused"
	if err := database.DB.Save(&config).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "暂停失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "已暂停"})
}

func resumeSMTPConfig(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	var config models.SMTPConfig
	if err := database.DB.First(&config, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "SMTP配置不存在"})
		return
	}

	config.Status = "active"
	config.FailureCount = 0
	config.AutoRecoverAt = nil
	config.LastFailedAt = nil
	if err := database.DB.Save(&config).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "恢复失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "已恢复"})
}

func resetSMTPQuota(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	ctx := c.Request.Context()
	now := time.Now()
	
	hourKey := "mailflow:smtp_hour:" + strconv.FormatUint(id, 10) + ":" + now.Format("2006-01-02-15")
	dayKey := "mailflow:smtp_day:" + strconv.FormatUint(id, 10) + ":" + now.Format("2006-01-02")
	
	pipe := queue.Client.Pipeline()
	pipe.Del(ctx, hourKey)
	pipe.Del(ctx, dayKey)
	if _, err := pipe.Exec(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重置配额失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "配额重置成功"})
}

func getSMTPHealth(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	var config models.SMTPConfig
	if err := database.DB.First(&config, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "SMTP配置不存在"})
		return
	}

	ctx := c.Request.Context()
	now := time.Now()
	
	hourKey := "mailflow:smtp_hour:" + strconv.FormatUint(id, 10) + ":" + now.Format("2006-01-02-15")
	dayKey := "mailflow:smtp_day:" + strconv.FormatUint(id, 10) + ":" + now.Format("2006-01-02")
	
	hourCount, _ := queue.Client.Get(ctx, hourKey).Int64()
	dayCount, _ := queue.Client.Get(ctx, dayKey).Int64()

	c.JSON(http.StatusOK, gin.H{
		"id":               config.ID,
		"name":             config.Name,
		"status":           config.Status,
		"failure_count":    config.FailureCount,
		"last_failed_at":   config.LastFailedAt,
		"last_checked_at":  config.LastCheckedAt,
		"auto_recover_at":  config.AutoRecoverAt,
		"hour_count":       hourCount,
		"hour_limit":       config.MaxPerHour,
		"day_count":        dayCount,
		"day_limit":        config.MaxPerDay,
	})
}

func batchTestSMTPConfigs(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	results := make([]map[string]interface{}, 0, len(req.IDs))
	
	for _, id := range req.IDs {
		var config models.SMTPConfig
		if err := database.DB.First(&config, id).Error; err != nil {
			results = append(results, map[string]interface{}{
				"id":      id,
				"success": false,
				"error":   "配置不存在",
			})
			continue
		}

		err := smtphealth.TestSMTPConnection(&config)
		results = append(results, map[string]interface{}{
			"id":      id,
			"name":    config.Name,
			"success": err == nil,
			"error":   func() string { if err != nil { return err.Error() }; return "" }(),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"results": results,
	})
}

func listAdminTokens(c *gin.Context) {
	var tokens []models.AdminToken
	if err := database.DB.Order("created_at DESC").Find(&tokens).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, tokens)
}

func createAdminToken(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	token := models.AdminToken{
		Token:       uuid.New().String(),
		Name:        req.Name,
		Description: req.Description,
		IsActive:    true,
	}

	if err := database.DB.Create(&token).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建失败"})
		return
	}

	c.JSON(http.StatusOK, token)
}

func deleteAdminToken(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	if err := database.DB.Delete(&models.AdminToken{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

func toggleAdminToken(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	var token models.AdminToken
	if err := database.DB.First(&token, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Token不存在"})
		return
	}

	token.IsActive = !token.IsActive
	if err := database.DB.Save(&token).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, token)
}
