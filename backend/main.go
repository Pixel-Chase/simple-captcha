package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"simple-captcha/config"
	"simple-captcha/routes"
	"simple-captcha/services"
	"simple-captcha/storage"
	"syscall"
)

func main() {
	// 子命令：重置管理员账号（遗失密码时使用），不启动服务
	if len(os.Args) > 1 && os.Args[1] == "reset-admin" {
		os.Exit(runResetAdmin(config.LoadConfig().Admin.DataDir, os.Stdout, os.Stderr))
	}

	// 初始化配置
	cfg := config.LoadConfig()

	// 持久数据存储（数据目录下的 captcha.db）：打不开时直接退出，原因写在错误里（目录不可写、文件损坏等）
	db, err := storage.Open(cfg.Admin.DataDir)
	if err != nil {
		log.Fatalln("打开数据存储失败:", err)
	}
	log.Printf("数据存储: %s", db.Path())
	services.UseStorage(db)

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

// runResetAdmin 子命令 `simple-captcha reset-admin`：删除管理员账号与全部会话，生成新的初始化口令并打印，
// 之后在控制台初始化页面重新设置用户名与密码。数据目录与服务一致（工作目录下的 data）；
// 服务运行中执行也立即生效，无需重启；接入应用、策略与背景图不受影响。容器部署时用 docker compose exec 在容器内执行
func runResetAdmin(dataDir string, stdout, stderr io.Writer) int {
	db, err := storage.Open(dataDir)
	if err != nil {
		fmt.Fprintln(stderr, "重置管理员账号失败:", err)
		return 1
	}
	defer db.Close()
	code, err := services.ResetAdmin(context.Background(), db)
	if err != nil {
		fmt.Fprintln(stderr, "重置管理员账号失败:", err)
		return 1
	}
	fmt.Fprint(stdout, services.SetupNotice(code))
	fmt.Fprintln(stdout, "管理员账号与全部会话已删除，服务无需重启；请用上面的口令在控制台重新初始化。")
	return 0
}
