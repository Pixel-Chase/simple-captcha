package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"simple-captcha/config"
	"simple-captcha/routes"
	"simple-captcha/services"
	"syscall"
	"time"
)

var startTime time.Time

func main() {
	startTime = time.Now()

	// 初始化配置
	cfg := config.LoadConfig()

	// 初始化服务管理器
	serviceManager := services.GetServiceManager()

	// 验证服务
	if err := serviceManager.ValidateServices(); err != nil {
		log.Fatalln("服务验证失败:", err)
	}

	// 设置路由
	router := routes.SetupRouter()

	// 创建HTTP服务器
	server := &http.Server{
		Addr:           fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port),
		Handler:        router,
		ReadTimeout:    cfg.Server.ReadTimeout,
		WriteTimeout:   cfg.Server.WriteTimeout,
		IdleTimeout:    cfg.Server.IdleTimeout,
		MaxHeaderBytes: cfg.Server.MaxHeaderBytes,
	}

	// 启动服务器（在goroutine中）
	go func() {
		var err error
		if cfg.Security.EnableHTTPS {
			if cfg.Security.CertFile == "" || cfg.Security.KeyFile == "" {
				log.Fatalln("HTTPS启用但证书文件未配置")
			}
			err = server.ListenAndServeTLS(cfg.Security.CertFile, cfg.Security.KeyFile)
		} else {
			err = server.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			log.Fatalln("HTTP服务器启动失败:", err)
		}
	}()

	// 等待中断信号以优雅关闭服务器
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// 创建关闭上下文
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.GracefulTimeout)
	defer cancel()

	// 关闭HTTP服务器
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("服务器强制关闭: %v", err)
	}

	// 关闭服务管理器
	serviceManager.Shutdown()
}

// GetUptime 获取运行时间
func GetUptime() time.Duration {
	return time.Since(startTime)
}
