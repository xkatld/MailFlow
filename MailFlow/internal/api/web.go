package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mailflow/smtp-loadbalancer/internal/config"
)

func RegisterWebUI(r *gin.Engine, cfg *config.Config) {
	r.Static("/static", "./web/static")
	r.LoadHTMLGlob("web/templates/*")

	r.GET("/admin/login", loginPage)
	r.POST("/admin/login", HandleLogin(cfg))
	r.GET("/admin/logout", HandleLogout())

	admin := r.Group("/admin")
	admin.Use(AdminAuthMiddleware(cfg))
	{
		admin.GET("", dashboard)
		admin.GET("/", dashboard)
		admin.GET("/keys", keysPage)
		admin.GET("/smtp", smtpPage)
		admin.GET("/plans", plansPage)
		admin.GET("/admin-tokens", adminTokensPage)
		admin.GET("/logs", logsPage)
		admin.GET("/stats", statsPage)
	}
}

func loginPage(c *gin.Context) {
	c.HTML(http.StatusOK, "login.html", nil)
}

func dashboard(c *gin.Context) {
	c.HTML(http.StatusOK, "dashboard.html", gin.H{
		"title": "仪表盘",
		"page":  "dashboard",
	})
}

func keysPage(c *gin.Context) {
	c.HTML(http.StatusOK, "keys.html", gin.H{
		"title": "API密钥管理",
		"page":  "keys",
	})
}

func smtpPage(c *gin.Context) {
	c.HTML(http.StatusOK, "smtp.html", gin.H{
		"title": "SMTP配置管理",
		"page":  "smtp",
	})
}

func logsPage(c *gin.Context) {
	c.HTML(http.StatusOK, "logs.html", gin.H{
		"title": "发送日志",
		"page":  "logs",
	})
}

func statsPage(c *gin.Context) {
	c.HTML(http.StatusOK, "stats.html", gin.H{
		"title": "统计报表",
		"page":  "stats",
	})
}

func plansPage(c *gin.Context) {
	c.HTML(http.StatusOK, "plans.html", gin.H{
		"title": "套餐管理",
		"page":  "plans",
	})
}

func adminTokensPage(c *gin.Context) {
	c.HTML(http.StatusOK, "admin-tokens.html", gin.H{
		"title": "管理员Token",
		"page":  "admin-tokens",
	})
}

