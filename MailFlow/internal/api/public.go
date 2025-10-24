package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mailflow/smtp-loadbalancer/internal/database"
	"github.com/mailflow/smtp-loadbalancer/internal/models"
)

func RegisterPublicAPI(r *gin.Engine) {
	public := r.Group("/api/public")
	{
		public.GET("/plans", getPublicPlans)
	}
}

func getPublicPlans(c *gin.Context) {
	var plans []models.Plan
	if err := database.DB.Where("is_active = ?", true).Order("sort_order ASC").Find(&plans).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, plans)
}

