package smtp

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/smtp"
	"time"

	"github.com/mailflow/smtp-loadbalancer/internal/database"
	"github.com/mailflow/smtp-loadbalancer/internal/models"
	"gopkg.in/gomail.v2"
)

const (
	MaxFailures        = 3
	RecoveryDelay      = 30 * time.Minute
	HealthCheckInterval = 5 * time.Minute
)

func StartHealthCheck(ctx context.Context) {
	log.Println("SMTP健康检查服务已启动")
	
	ticker := time.NewTicker(HealthCheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			log.Println("SMTP健康检查服务正在关闭")
			return
		case <-ticker.C:
			checkAllSMTP(ctx)
		}
	}
}

func checkAllSMTP(ctx context.Context) {
	var configs []models.SMTPConfig
	if err := database.DB.Find(&configs).Error; err != nil {
		log.Printf("获取SMTP配置失败: %v", err)
		return
	}
	
	now := time.Now()
	
	for _, config := range configs {
		if config.Status == "paused" {
			continue
		}
		
		if config.Status == "failed" && config.AutoRecoverAt != nil {
			if now.After(*config.AutoRecoverAt) {
				log.Printf("尝试自动恢复SMTP[%s]", config.Name)
				if err := TestSMTPConnection(&config); err == nil {
					config.Status = "active"
					config.FailureCount = 0
					config.AutoRecoverAt = nil
					config.LastFailedAt = nil
					now := time.Now()
					config.LastCheckedAt = &now
					database.DB.Save(&config)
					log.Printf("SMTP[%s]自动恢复成功", config.Name)
				} else {
					recoverAt := now.Add(RecoveryDelay)
					config.AutoRecoverAt = &recoverAt
					now := time.Now()
					config.LastCheckedAt = &now
					database.DB.Save(&config)
					log.Printf("SMTP[%s]自动恢复失败，下次尝试: %s", config.Name, recoverAt.Format("2006-01-02 15:04:05"))
				}
			}
			continue
		}
		
		if config.Status == "active" {
			if err := TestSMTPConnection(&config); err != nil {
				RecordSMTPFailure(ctx, config.ID)
				log.Printf("SMTP[%s]健康检查失败: %v", config.Name, err)
			} else {
				if config.FailureCount > 0 {
					config.FailureCount = 0
					now := time.Now()
					config.LastCheckedAt = &now
					database.DB.Save(&config)
				}
				log.Printf("SMTP[%s]健康检查正常", config.Name)
			}
		}
	}
}

func TestSMTPConnection(config *models.SMTPConfig) error {
	m := gomail.NewMessage()
	m.SetHeader("From", config.FromEmail)
	m.SetHeader("To", config.FromEmail)
	m.SetHeader("Subject", "MailFlow Health Check")
	m.SetBody("text/plain", "This is a health check email from MailFlow.")

	d := gomail.NewDialer(config.Host, config.Port, config.Username, config.Password)

	encryption := config.Encryption
	if encryption == "" {
		encryption = "starttls"
	}

	switch encryption {
	case "ssl":
		d.SSL = true
		d.TLSConfig = &tls.Config{
			ServerName:         config.Host,
			InsecureSkipVerify: false,
		}
	case "tls", "starttls":
		d.SSL = false
		d.TLSConfig = &tls.Config{
			ServerName:         config.Host,
			InsecureSkipVerify: false,
		}
	case "none":
		d.SSL = false
		d.TLSConfig = nil
	default:
		d.SSL = false
		d.TLSConfig = &tls.Config{
			ServerName:         config.Host,
			InsecureSkipVerify: false,
		}
	}

	if config.AuthMethod == "xoauth2" || config.AuthMethod == "oauth2" {
		d.Auth = &xoauth2Auth{
			username: config.Username,
			token:    config.Password,
		}
	}

	conn, err := d.Dial()
	if err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}
	defer conn.Close()

	return nil
}

type xoauth2Auth struct {
	username string
	token    string
}

func (a *xoauth2Auth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	authStr := fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", a.username, a.token)
	return "XOAUTH2", []byte(authStr), nil
}

func (a *xoauth2Auth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		return []byte(""), nil
	}
	return nil, nil
}

func RecordSMTPFailure(ctx context.Context, smtpID uint) {
	var config models.SMTPConfig
	if err := database.DB.First(&config, smtpID).Error; err != nil {
		return
	}
	
	config.FailureCount++
	now := time.Now()
	config.LastFailedAt = &now
	config.LastCheckedAt = &now
	
	if config.FailureCount >= MaxFailures {
		AutoDisableSMTP(smtpID)
	} else {
		database.DB.Save(&config)
	}
}

func AutoDisableSMTP(smtpID uint) {
	var config models.SMTPConfig
	if err := database.DB.First(&config, smtpID).Error; err != nil {
		return
	}
	
	config.Status = "failed"
	recoverAt := time.Now().Add(RecoveryDelay)
	config.AutoRecoverAt = &recoverAt
	
	database.DB.Save(&config)
	log.Printf("SMTP[%s]已自动禁用，连续失败%d次，将在%s尝试恢复", 
		config.Name, config.FailureCount, recoverAt.Format("2006-01-02 15:04:05"))
}

func AutoEnableSMTP(ctx context.Context, smtpID uint) error {
	var config models.SMTPConfig
	if err := database.DB.First(&config, smtpID).Error; err != nil {
		return err
	}
	
	if err := TestSMTPConnection(&config); err != nil {
		return err
	}
	
	config.Status = "active"
	config.FailureCount = 0
	config.AutoRecoverAt = nil
	config.LastFailedAt = nil
	now := time.Now()
	config.LastCheckedAt = &now
	
	return database.DB.Save(&config).Error
}

