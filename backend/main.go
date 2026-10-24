package main

import (
	"bufio"
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
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var startTime time.Time

func main() {
	// 子命令：生成管理员密码哈希（ADMIN_PASSWORD_HASH），不启动服务
	if len(os.Args) > 1 && os.Args[1] == "hash-password" {
		os.Exit(runHashPassword(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	}

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

// runHashPassword 子命令 `simple-captcha hash-password [密码]`：
// 生成 ADMIN_PASSWORD_HASH 所需的 bcrypt 哈希，不启动服务。
// 不带参数时从标准输入读取一行，避免密码进入 shell 历史；哈希输出到标准输出，提示写到标准错误，
// 便于 `./simple-captcha hash-password > hash.txt` 这类用法。
func runHashPassword(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var password string
	if len(args) > 0 {
		password = args[0]
	} else {
		fmt.Fprint(stderr, "请输入管理员密码（回车结束）: ")
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && line == "" {
			fmt.Fprintln(stderr, "读取密码失败:", err)
			return 1
		}
		password = strings.TrimRight(line, "\r\n")
	}

	if password == "" {
		fmt.Fprintln(stderr, "密码不能为空")
		return 1
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintln(stderr, "生成哈希失败:", err)
		return 1
	}

	fmt.Fprintln(stdout, string(hash))
	fmt.Fprintln(stderr, "写入 .env 时请用单引号包裹：ADMIN_PASSWORD_HASH='<上面的哈希>'")
	return 0
}
