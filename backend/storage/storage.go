// Package storage 管理持久数据所在的内嵌 SQLite（数据目录下的 captcha.db）：
// 打开连接、收紧文件权限、执行编号迁移。只考虑单实例部署，带过期时间的短期数据不放这里（见 cache 包）。
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite" // 纯 Go 实现，适配 CGO_ENABLED=0 的镜像构建
)

const (
	// FileName 数据库文件名，固定放在数据目录下，不做成配置项
	FileName = "captcha.db"

	dirMode  = 0o700
	fileMode = 0o600
)

// pragmas 每个连接打开时执行：WAL 让读不阻塞写；NORMAL 在 WAL 下只在检查点刷盘，
// 掉电最多丢最近一次提交，换来约十倍的写入吞吐；busy_timeout 让并发写入排队而不是立即报 SQLITE_BUSY
var pragmas = []string{
	"journal_mode(WAL)",
	"synchronous(NORMAL)",
	"busy_timeout(5000)",
	"foreign_keys(ON)",
}

// DB 持久数据存储，内部只保留一个连接：SQLite 写入本就串行，单连接避免了连接间的锁竞争
type DB struct {
	*sql.DB
	path string
}

// Open 确保数据目录存在且可写，打开（不存在则创建）captcha.db 并执行全部未执行的迁移
func Open(dataDir string) (*DB, error) {
	if err := ensureDataDir(dataDir); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, FileName)

	// 事务一律 BEGIN IMMEDIATE：开始即取得写锁。reset-admin 子命令会与运行中的服务同时访问同一个库文件，
	// 延迟加锁的事务"先读后写"时若别的进程已提交，会直接得到 SQLITE_BUSY 而不会排队等待
	dsn := "file:" + path + "?_txlock=immediate"
	for _, p := range pragmas {
		dsn += "&_pragma=" + p
	}
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库 %s 失败: %w", path, err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)
	sqlDB.SetConnMaxIdleTime(0)

	db := &DB{DB: sqlDB, path: path}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("数据库 %s 无法读取（文件可能已损坏或不是 SQLite 数据库）: %w", path, err)
	}
	// 数据库文件与 WAL 附属文件都只允许属主读写
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Chmod(path+suffix, fileMode); err != nil && !errors.Is(err, os.ErrNotExist) {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("收紧数据库文件权限失败: %w", err)
		}
	}
	if err := db.migrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}

// Path 数据库文件的路径
func (db *DB) Path() string {
	return db.path
}

// Tx 在一个事务中执行 fn：fn 返回错误或 panic 时回滚，否则提交
func (db *DB) Tx(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// OpenTemp 在测试临时目录中打开一个全新的数据库，测试结束时自动关闭
func OpenTemp(t testing.TB) *DB {
	t.Helper()
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("打开临时数据库失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// ensureDataDir 创建数据目录、收紧为只允许属主访问并确认可写，部署时挂载卷权限不对能在启动阶段直接暴露。
// MkdirAll 不会修改已存在目录的权限，必须另行 Chmod：目录对其他用户可写时，即使库文件是 0600 也能被删除或替换
func ensureDataDir(dir string) error {
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("创建数据目录 %s 失败: %w", dir, err)
	}
	if err := os.Chmod(dir, dirMode); err != nil {
		return fmt.Errorf("收紧数据目录 %s 权限失败（需由运行服务的用户拥有该目录）: %w", dir, err)
	}
	probe, err := os.CreateTemp(dir, ".probe-*")
	if err != nil {
		return fmt.Errorf("数据目录 %s 不可写: %w", dir, err)
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return nil
}
