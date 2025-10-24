package models

import (
	"time"

	"gorm.io/gorm"
)

type Plan struct {
	ID           uint      `gorm:"primarykey" json:"id"`
	Code         string    `gorm:"uniqueIndex;not null" json:"code"`
	Name         string    `gorm:"not null" json:"name"`
	Description  string    `json:"description"`
	MinuteLimit  int       `gorm:"default:0" json:"minute_limit"`
	DailyLimit   int       `gorm:"default:0" json:"daily_limit"`
	WeeklyLimit  int       `gorm:"default:0" json:"weekly_limit"`
	MonthlyLimit int       `gorm:"default:0" json:"monthly_limit"`
	IsActive     bool      `gorm:"default:true" json:"is_active"`
	SortOrder    int       `gorm:"default:0" json:"sort_order"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type APIKey struct {
	ID           uint      `gorm:"primarykey" json:"id"`
	Key          string    `gorm:"uniqueIndex;not null" json:"key"`
	Name         string    `gorm:"not null" json:"name"`
	PlanID       *uint     `gorm:"index" json:"plan_id"`
	Plan         string    `gorm:"default:basic" json:"plan"`
	IsCustom     bool      `gorm:"default:false" json:"is_custom"`
	MinuteLimit  int       `gorm:"default:100" json:"minute_limit"`
	DailyLimit   int       `gorm:"default:10000" json:"daily_limit"`
	WeeklyLimit  int       `gorm:"default:50000" json:"weekly_limit"`
	MonthlyLimit int       `gorm:"default:200000" json:"monthly_limit"`
	TotalLimit   int       `gorm:"default:0" json:"total_limit"`
	TotalUsed    int       `gorm:"default:0" json:"total_used"`
	Status       string    `gorm:"default:active" json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type SMTPConfig struct {
	ID             uint       `gorm:"primarykey" json:"id"`
	Name           string     `gorm:"not null" json:"name"`
	Host           string     `gorm:"not null" json:"host"`
	Port           int        `gorm:"not null" json:"port"`
	Username       string     `gorm:"not null" json:"username"`
	Password       string     `gorm:"not null" json:"password"`
	AuthMethod     string     `gorm:"default:plain" json:"auth_method"`
	Encryption     string     `gorm:"default:starttls" json:"encryption"`
	FromEmail      string     `gorm:"not null" json:"from_email"`
	FromName       string     `json:"from_name"`
	MaxPerHour     int        `gorm:"default:100" json:"max_per_hour"`
	MaxPerDay      int        `gorm:"default:0" json:"max_per_day"`
	Priority       int        `gorm:"default:1" json:"priority"`
	Status         string     `gorm:"default:active" json:"status"`
	FailureCount   int        `gorm:"default:0" json:"failure_count"`
	LastFailedAt   *time.Time `json:"last_failed_at"`
	LastCheckedAt  *time.Time `json:"last_checked_at"`
	AutoRecoverAt  *time.Time `json:"auto_recover_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type SendLog struct {
	ID           uint      `gorm:"primarykey" json:"id"`
	APIKeyID     uint      `gorm:"index" json:"api_key_id"`
	To           string    `gorm:"not null" json:"to"`
	Subject      string    `json:"subject"`
	Status       string    `gorm:"index" json:"status"`
	ErrorMsg     string    `json:"error_msg"`
	SMTPConfigID uint      `json:"smtp_config_id"`
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

type UsageStats struct {
	ID          uint      `gorm:"primarykey" json:"id"`
	APIKeyID    uint      `gorm:"uniqueIndex:idx_apikey_date" json:"api_key_id"`
	Date        time.Time `gorm:"uniqueIndex:idx_apikey_date;type:date" json:"date"`
	SentCount   int       `gorm:"default:0" json:"sent_count"`
	FailedCount int       `gorm:"default:0" json:"failed_count"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type SMTPStats struct {
	ID           uint      `gorm:"primarykey" json:"id"`
	SMTPConfigID uint      `gorm:"uniqueIndex:idx_smtp_hour" json:"smtp_config_id"`
	Hour         time.Time `gorm:"uniqueIndex:idx_smtp_hour;type:timestamp" json:"hour"`
	SentCount    int       `gorm:"default:0" json:"sent_count"`
	FailedCount  int       `gorm:"default:0" json:"failed_count"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type AdminToken struct {
	ID          uint       `gorm:"primarykey" json:"id"`
	Token       string     `gorm:"uniqueIndex;not null" json:"token"`
	Name        string     `gorm:"not null" json:"name"`
	Description string     `json:"description"`
	IsActive    bool       `gorm:"default:true" json:"is_active"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&Plan{},
		&APIKey{},
		&SMTPConfig{},
		&SendLog{},
		&UsageStats{},
		&SMTPStats{},
		&AdminToken{},
	)
}

