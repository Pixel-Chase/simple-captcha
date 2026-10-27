package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenRunsMigrationsOnce(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	first, err := Open(dir)
	require.NoError(t, err)
	applied, err := first.AppliedMigrations(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, applied)
	var appliedAt int64
	require.NoError(t, first.QueryRowContext(ctx, `SELECT applied_at FROM schema_migrations WHERE version = 1`).Scan(&appliedAt))
	require.NoError(t, first.Close())

	second, err := Open(dir)
	require.NoError(t, err)
	defer second.Close()
	again, err := second.AppliedMigrations(ctx)
	require.NoError(t, err)
	assert.Equal(t, applied, again)

	// 第二次打开没有重新执行：版本只登记一次，执行时间不变
	var count int
	var appliedAgain int64
	require.NoError(t, second.QueryRowContext(ctx, `SELECT COUNT(1), MAX(applied_at) FROM schema_migrations WHERE version = 1`).Scan(&count, &appliedAgain))
	assert.Equal(t, 1, count)
	assert.Equal(t, appliedAt, appliedAgain)
}

func TestOpenPragmasAndPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	db, err := Open(dir)
	require.NoError(t, err)
	defer db.Close()
	ctx := context.Background()

	tests := []struct {
		pragma string
		want   string
	}{
		{"journal_mode", "wal"},
		{"synchronous", "1"}, // NORMAL
		{"busy_timeout", "5000"},
		{"foreign_keys", "1"},
	}
	for _, tt := range tests {
		t.Run(tt.pragma, func(t *testing.T) {
			var got string
			require.NoError(t, db.QueryRowContext(ctx, "PRAGMA "+tt.pragma).Scan(&got))
			assert.Equal(t, tt.want, got)
		})
	}

	info, err := os.Stat(filepath.Join(dir, FileName))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	dirInfo, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
}

// 已存在的数据目录（例如手工创建或挂载进来的宽权限目录）在打开时同样收紧到 0700
func TestOpenTightensExistingDirMode(t *testing.T) {
	tests := []struct {
		name string
		mode os.FileMode
	}{
		{"所有人可写", 0o777},
		{"其他用户可读", 0o755},
		{"已是 0700", 0o700},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "data")
			require.NoError(t, os.Mkdir(dir, 0o700))
			// Chmod 不受 umask 影响，确保目录确实是宽权限
			require.NoError(t, os.Chmod(dir, tt.mode))

			db, err := Open(dir)
			require.NoError(t, err)
			defer db.Close()

			info, err := os.Stat(dir)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
		})
	}
}

func TestConcurrentWritesDoNotBusy(t *testing.T) {
	db := OpenTemp(t)
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT, worker INTEGER, n INTEGER)`)
	require.NoError(t, err)

	const workers, perWorker = 20, 50
	var wg sync.WaitGroup
	errs := make(chan error, workers*perWorker)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				if _, err := db.ExecContext(ctx, `INSERT INTO t (worker, n) VALUES (?, ?)`, w, i); err != nil {
					errs <- err
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("并发写入失败: %v", err)
	}

	var rows int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(1) FROM t`).Scan(&rows))
	assert.Equal(t, workers*perWorker, rows)
}

func TestTxRollsBackOnError(t *testing.T) {
	db := OpenTemp(t)
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `CREATE TABLE t (v INTEGER)`)
	require.NoError(t, err)

	err = db.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO t (v) VALUES (1)`); err != nil {
			return err
		}
		return fmt.Errorf("中途失败")
	})
	require.Error(t, err)
	var rows int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(1) FROM t`).Scan(&rows))
	assert.Zero(t, rows)
}

func TestOpenRejectsCorruptFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, FileName), []byte("这不是 SQLite 数据库，长度也要超过文件头的一百字节才会被识别为损坏而不是空库……………………………………"), 0o600))
	_, err := Open(dir)
	require.Error(t, err)
}

func TestOpenRejectsUnwritableDir(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocker, nil, 0o600))
	_, err := Open(blocker)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "数据目录")
}
