package worker

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/smtp"
	"time"

	"github.com/mailflow/smtp-loadbalancer/internal/auth"
	"github.com/mailflow/smtp-loadbalancer/internal/database"
	"github.com/mailflow/smtp-loadbalancer/internal/loadbalancer"
	"github.com/mailflow/smtp-loadbalancer/internal/models"
	"github.com/mailflow/smtp-loadbalancer/internal/queue"
	"github.com/mailflow/smtp-loadbalancer/internal/stats"
	"gopkg.in/gomail.v2"
	"gorm.io/gorm"
)

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

func Start(ctx context.Context, workerCount int) {
	for i := 0; i < workerCount; i++ {
		go worker(ctx, i)
	}
	log.Printf("启动了 %d 个邮件发送Worker", workerCount)
}

func worker(ctx context.Context, id int) {
	log.Printf("Worker %d 已启动", id)
	
	for {
		select {
		case <-ctx.Done():
			log.Printf("Worker %d 正在关闭", id)
			return
		default:
			task, err := queue.PopEmail(ctx, 5*time.Second)
			if err != nil {
				log.Printf("Worker %d 获取任务失败: %v", id, err)
				continue
			}

			if task == nil {
				continue
			}

			if err := processEmail(ctx, task); err != nil {
				log.Printf("Worker %d 处理任务失败: %v", id, err)
			}
		}
	}
}

func processEmail(ctx context.Context, task *queue.EmailTask) error {
	const maxRetries = 3
	
	for _, recipient := range task.To {
		var lastErr error
		var successSMTP *models.SMTPConfig
		
		for attempt := 0; attempt < maxRetries; attempt++ {
			smtpConfig, err := loadbalancer.SelectSMTP(ctx)
			if err != nil {
				lastErr = err
				log.Printf("尝试 %d/%d: 无法获取SMTP服务器 [%s]: %v", attempt+1, maxRetries, recipient, err)
				time.Sleep(time.Duration(attempt+1) * time.Second)
				continue
			}

			if err := sendEmail(smtpConfig, recipient, task); err != nil {
				lastErr = err
				log.Printf("尝试 %d/%d: SMTP[%s] 发送失败 [%s]: %v", attempt+1, maxRetries, smtpConfig.Name, recipient, err)
				time.Sleep(time.Duration(attempt+1) * time.Second)
				continue
			}
			
			successSMTP = smtpConfig
			break
		}

		if successSMTP != nil {
			logSuccess(task, successSMTP.ID, recipient)
			stats.IncrementSent(ctx, task.APIKeyID)
			loadbalancer.IncrementSMTPCount(ctx, successSMTP.ID)
			auth.ConsumeQuota(ctx, task.APIKeyID)
			database.DB.Model(&models.APIKey{}).Where("id = ?", task.APIKeyID).UpdateColumn("total_used", gorm.Expr("total_used + ?", 1))
			log.Printf("邮件发送成功 [SMTP: %s] [收件人: %s]", successSMTP.Name, recipient)
		} else {
			errorMsg := fmt.Sprintf("重试%d次后失败: %v", maxRetries, lastErr)
			logFailure(task, 0, errorMsg)
			stats.IncrementFailed(ctx, task.APIKeyID)
			log.Printf("邮件发送彻底失败 [%s]: %s", recipient, errorMsg)
		}
	}

	return nil
}

func sendEmail(config *models.SMTPConfig, to string, task *queue.EmailTask) error {
	m := gomail.NewMessage()
	
	if config.FromName != "" {
		m.SetHeader("From", m.FormatAddress(config.FromEmail, config.FromName))
	} else {
		m.SetHeader("From", config.FromEmail)
	}
	
	m.SetHeader("To", to)
	m.SetHeader("Subject", task.Subject)

	if task.HTML != "" {
		m.SetBody("text/html", task.HTML)
		if task.Text != "" {
			m.AddAlternative("text/plain", task.Text)
		}
	} else {
		m.SetBody("text/plain", task.Text)
	}

	d := gomail.NewDialer(config.Host, config.Port, config.Username, config.Password)

	encryption := config.Encryption
	if encryption == "" {
		encryption = "starttls"
	}

	switch encryption {
	case "ssl":
		// SSL: 直接加密连接 (通常465端口)
		d.SSL = true
		d.TLSConfig = &tls.Config{
			ServerName:         config.Host,
			InsecureSkipVerify: false,
		}
	case "tls", "starttls":
		// TLS/STARTTLS: 明文连接后升级加密 (通常587端口)
		d.SSL = false
		d.TLSConfig = &tls.Config{
			ServerName:         config.Host,
			InsecureSkipVerify: false,
		}
	case "none":
		d.SSL = false
		d.TLSConfig = nil
	default:
		// 默认使用STARTTLS
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

	if err := d.DialAndSend(m); err != nil {
		return fmt.Errorf("SMTP发送失败: %w", err)
	}

	return nil
}

func logSuccess(task *queue.EmailTask, smtpID uint, recipient string) {
	log := models.SendLog{
		APIKeyID:     task.APIKeyID,
		To:           recipient,
		Subject:      task.Subject,
		Status:       "success",
		SMTPConfigID: smtpID,
		CreatedAt:    time.Now(),
	}
	database.DB.Create(&log)
}

func logFailure(task *queue.EmailTask, smtpID uint, errorMsg string) {
	for _, recipient := range task.To {
		log := models.SendLog{
			APIKeyID:     task.APIKeyID,
			To:           recipient,
			Subject:      task.Subject,
			Status:       "failed",
			ErrorMsg:     errorMsg,
			SMTPConfigID: smtpID,
			CreatedAt:    time.Now(),
		}
		database.DB.Create(&log)
	}
}

