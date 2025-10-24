package database

import (
	"log"

	"github.com/mailflow/smtp-loadbalancer/internal/models"
)

func InitDefaultPlans() error {
	var count int64
	if err := DB.Model(&models.Plan{}).Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		log.Println("套餐已存在，跳过初始化")
		return nil
	}

	defaultPlans := []models.Plan{
		{
			Code:         "free",
			Name:         "免费版",
			Description:  "适合个人用户测试使用",
			MinuteLimit:  10,
			DailyLimit:   100,
			WeeklyLimit:  500,
			MonthlyLimit: 2000,
			IsActive:     true,
			SortOrder:    1,
		},
		{
			Code:         "basic",
			Name:         "基础版",
			Description:  "适合小型应用和初创团队",
			MinuteLimit:  100,
			DailyLimit:   5000,
			WeeklyLimit:  30000,
			MonthlyLimit: 100000,
			IsActive:     true,
			SortOrder:    2,
		},
		{
			Code:         "standard",
			Name:         "标准版",
			Description:  "适合中型企业日常使用",
			MinuteLimit:  500,
			DailyLimit:   20000,
			WeeklyLimit:  120000,
			MonthlyLimit: 500000,
			IsActive:     true,
			SortOrder:    3,
		},
		{
			Code:         "professional",
			Name:         "专业版",
			Description:  "适合大型企业高频场景",
			MinuteLimit:  1000,
			DailyLimit:   50000,
			WeeklyLimit:  300000,
			MonthlyLimit: 1000000,
			IsActive:     true,
			SortOrder:    4,
		},
		{
			Code:         "enterprise",
			Name:         "企业版",
			Description:  "无限制企业级方案",
			MinuteLimit:  0,
			DailyLimit:   0,
			WeeklyLimit:  0,
			MonthlyLimit: 0,
			IsActive:     true,
			SortOrder:    5,
		},
	}

	if err := DB.Create(&defaultPlans).Error; err != nil {
		return err
	}

	log.Println("默认套餐初始化成功")
	return nil
}

