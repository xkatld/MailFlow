package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
	"github.com/mailflow/smtp-loadbalancer/internal/config"
	"github.com/mailflow/smtp-loadbalancer/internal/database"
	"github.com/mailflow/smtp-loadbalancer/internal/models"
)

var store = sessions.NewCookieStore([]byte("mailflow-secret-key-change-in-production"))

func init() {
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
	}
}

func AdminAuthMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if token := c.GetHeader("X-Admin-Token"); token != "" {
			if validateAdminToken(token) {
				c.Next()
				return
			}
		}
		
		session, _ := store.Get(c.Request, "mailflow-session")
		if auth, ok := session.Values["authenticated"].(bool); ok && auth {
			c.Next()
			return
		}
		
		if c.ContentType() == "application/json" || c.GetHeader("Accept") == "application/json" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		} else {
			c.Redirect(http.StatusFound, "/admin/login")
		}
		c.Abort()
	}
}

func validateAdminToken(token string) bool {
	var adminToken models.AdminToken
	if err := database.DB.Where("token = ? AND is_active = ?", token, true).First(&adminToken).Error; err != nil {
		return false
	}

	now := time.Now()
	adminToken.LastUsedAt = &now
	database.DB.Save(&adminToken)

	return true
}

func HandleLogin(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求"})
			return
		}
		
		if req.Username == cfg.Admin.Username && req.Password == cfg.Admin.Password {
			session, _ := store.Get(c.Request, "mailflow-session")
			session.Values["authenticated"] = true
			session.Save(c.Request, c.Writer)
			
			c.JSON(http.StatusOK, gin.H{"message": "登录成功"})
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		}
	}
}

func HandleLogout() gin.HandlerFunc {
	return func(c *gin.Context) {
		session, _ := store.Get(c.Request, "mailflow-session")
		session.Values["authenticated"] = false
		session.Options.MaxAge = -1
		session.Save(c.Request, c.Writer)
		
		c.Redirect(http.StatusFound, "/admin/login")
	}
}

