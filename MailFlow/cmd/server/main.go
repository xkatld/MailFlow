package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mailflow/smtp-loadbalancer/internal/api"
	"github.com/mailflow/smtp-loadbalancer/internal/config"
	"github.com/mailflow/smtp-loadbalancer/internal/database"
	"github.com/mailflow/smtp-loadbalancer/internal/queue"
	smtphealth "github.com/mailflow/smtp-loadbalancer/internal/smtp"
	"github.com/mailflow/smtp-loadbalancer/internal/stats"
	"github.com/mailflow/smtp-loadbalancer/internal/worker"
)

func main() {
	log.Println("正在启动MailFlow SMTP负载均衡系统...")

	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	log.Println("配置加载成功")

	if err := database.Connect(&cfg.Database); err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	defer database.Close()

	if err := queue.Connect(&cfg.Redis); err != nil {
		log.Fatalf("Redis连接失败: %v", err)
	}
	defer queue.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go stats.FlushStatsToDatabase(ctx)
	log.Println("统计模块已启动")

	go smtphealth.StartHealthCheck(ctx)
	log.Println("SMTP健康检查模块已启动")

	worker.Start(ctx, cfg.Worker.Count)

	r := gin.Default()
	
	api.RegisterPublicAPI(r)
	api.RegisterAPIKeyAPI(r)
	api.RegisterAdminAPI(r, cfg)
	api.RegisterWebUI(r, cfg)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: r,
	}

	go func() {
		log.Printf("HTTP服务器启动在端口 %d", cfg.Server.Port)
		log.Printf("管理后台地址: http://localhost:%d/admin", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务器启动失败: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("正在关闭服务器...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("服务器关闭出错: %v", err)
	}

	log.Println("服务器已优雅关闭")
}

