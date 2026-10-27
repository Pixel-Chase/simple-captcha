package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

// migrations 编号迁移脚本 NNNN_说明.sql：只追加新文件，已发布的文件不再修改
//
//go:embed migrations/*.sql
var migrations embed.FS

type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return nil, err
	}
	list := make([]migration, 0, len(entries))
	seen := make(map[int]string)
	for _, e := range entries {
		name := e.Name()
		prefix, _, ok := strings.Cut(name, "_")
		version, err := strconv.Atoi(prefix)
		if !ok || err != nil || version <= 0 {
			return nil, fmt.Errorf("迁移文件名 %s 不符合 NNNN_说明.sql", name)
		}
		if prev, dup := seen[version]; dup {
			return nil, fmt.Errorf("迁移编号重复: %s 与 %s", prev, name)
		}
		seen[version] = name
		data, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			return nil, err
		}
		list = append(list, migration{version: version, name: name, sql: string(data)})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].version < list[j].version })
	return list, nil
}

// migrate 按编号依次执行尚未执行的迁移，每个迁移连同版本登记放在同一事务中
func (db *DB) migrate(ctx context.Context) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("创建迁移记录表失败: %w", err)
	}
	list, err := loadMigrations()
	if err != nil {
		return err
	}
	for _, m := range list {
		err := db.Tx(ctx, func(tx *sql.Tx) error {
			var exists int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM schema_migrations WHERE version = ?`, m.version).Scan(&exists); err != nil {
				return err
			}
			if exists > 0 {
				return nil
			}
			if _, err := tx.ExecContext(ctx, m.sql); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, m.version, time.Now().Unix())
			return err
		})
		if err != nil {
			return fmt.Errorf("执行迁移 %s 失败: %w", m.name, err)
		}
	}
	return nil
}

// AppliedMigrations 已执行的迁移编号（升序），供测试与排障使用
func (db *DB) AppliedMigrations(ctx context.Context) ([]int, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var versions []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}
