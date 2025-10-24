package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mailflow/smtp-loadbalancer/internal/auth"
	"github.com/mailflow/smtp-loadbalancer/internal/database"
	"github.com/mailflow/smtp-loadbalancer/internal/models"
	"github.com/mailflow/smtp-loadbalancer/internal/queue"
	"github.com/mailflow/smtp-loadbalancer/internal/stats"
)

type SendEmailRequest struct {
	To      []string `json:"to" binding:"required"`
	Subject string   `json:"subject" binding:"required"`
	HTML    string   `json:"html"`
	Text    string   `json:"text"`
}

func RegisterAPIKeyAPI(r *gin.Engine) {
	apikey := r.Group("/api/v1")
	apikey.Use(auth.AuthMiddleware())
	{
		apikey.POST("/send", handleSendEmail)
		apikey.GET("/quota", getMyQuota)
		apikey.GET("/usage", getMyUsage)
		apikey.GET("/logs", getMyLogs)
	}
}

func handleSendEmail(c *gin.Context) {
	var req SendEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	if req.HTML == "" && req.Text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "必须提供html或text内容"})
		return
	}

	apiKeyID, _ := c.Get("api_key_id")

	task := &queue.EmailTask{
		APIKeyID: apiKeyID.(uint),
		To:       req.To,
		Subject:  req.Subject,
		HTML:     req.HTML,
		Text:     req.Text,
	}

	if err := queue.PushEmail(c.Request.Context(), task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "邮件入队失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "邮件已加入发送队列",
		"count":   len(req.To),
	})
}

func getMyQuota(c *gin.Context) {
	apiKeyID, exists := c.Get("api_key_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	quota, err := auth.GetRemainingQuota(c.Request.Context(), apiKeyID.(uint))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配额失败"})
		return
	}

	c.JSON(http.StatusOK, quota)
}

func getMyUsage(c *gin.Context) {
	apiKeyID, exists := c.Get("api_key_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	keyStats, err := stats.GetAPIKeyDetailStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取统计失败"})
		return
	}

	for _, stat := range keyStats {
		if stat.APIKeyID == apiKeyID.(uint) {
			c.JSON(http.StatusOK, stat)
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"api_key_id":   apiKeyID,
		"today_sent":   0,
		"today_failed": 0,
		"week_total":   0,
		"month_total":  0,
	})
}

func getMyLogs(c *gin.Context) {
	apiKeyID, exists := c.Get("api_key_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	status := c.Query("status")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}

	query := database.DB.Model(&models.SendLog{}).Where("api_key_id = ?", apiKeyID)

	if status != "" {
		query = query.Where("status = ?", status)
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

