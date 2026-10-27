package services

import (
	"context"
	"fmt"
	"simple-captcha/storage"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func countTelemetry(t *testing.T, db *storage.DB) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM telemetry`).Scan(&n))
	return n
}

// 并发写入 1.2 万条：只保留最近 1 万条；重新打开库后最近的记录仍可读
func TestTelemetryStore_RetentionAndReopen(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(dir)
	require.NoError(t, err)
	store := NewTelemetryStore(db, func() int { return 10000 })

	const workers, perWorker = 12, 1000
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				store.Log(map[string]interface{}{"kind": "verify", "worker": w, "seq": i})
				// 通道容量 1024：每个协程每 50 条等一次落库，在途记录最多 600 条，保证 1.2 万条全部写入而不是被丢弃
				if i%50 == 49 {
					store.Flush()
				}
			}
		}(w)
	}
	wg.Wait()
	require.Zero(t, store.Dropped())
	store.Log(map[string]interface{}{"kind": "issue", "key": "latest"})
	store.Close()
	assert.Equal(t, 10000, countTelemetry(t, db))
	require.NoError(t, db.Close())

	reopened, err := storage.Open(dir)
	require.NoError(t, err)
	defer reopened.Close()
	again := NewTelemetryStore(reopened, func() int { return 10000 })
	defer again.Close()
	items, err := again.Recent(3)
	require.NoError(t, err)
	require.Len(t, items, 3)
	assert.Equal(t, "latest", items[0]["key"], "新的在前")
	var kind string
	require.NoError(t, reopened.QueryRowContext(context.Background(), `SELECT kind FROM telemetry ORDER BY id DESC LIMIT 1`).Scan(&kind))
	assert.Equal(t, "issue", kind)
}

// 通道满时 Log 立即返回并计数，不阻塞调用方
func TestTelemetryStore_DropsWhenQueueFull(t *testing.T) {
	db := storage.OpenTemp(t)
	// 不启动后台写入，队列只会被填满
	store := &TelemetryStore{
		db:        db,
		retention: func() int { return 100 },
		queue:     make(chan telemetryRow, 8),
	}

	start := time.Now()
	for i := 0; i < 50; i++ {
		store.Log(map[string]interface{}{"kind": "verify", "seq": i})
	}
	assert.Less(t, time.Since(start), time.Second, "写满后不能阻塞")
	assert.Equal(t, int64(42), store.Dropped())
}

func TestTelemetryStore_CloseFlushesAndZeroRetentionSkips(t *testing.T) {
	db := storage.OpenTemp(t)

	store := NewTelemetryStore(db, func() int { return 100 })
	for i := 0; i < 5; i++ {
		store.Log(map[string]interface{}{"seq": i})
	}
	store.Close()
	assert.Equal(t, 5, countTelemetry(t, db), "关闭时写完剩余记录")

	// 缺少 kind 的记录按校验记录归类
	var kind string
	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT kind FROM telemetry LIMIT 1`).Scan(&kind))
	assert.Equal(t, "verify", kind)

	off := NewTelemetryStore(db, func() int { return 0 })
	defer off.Close()
	off.Log(map[string]interface{}{"seq": "ignored"})
	off.Flush()
	assert.Equal(t, 5, countTelemetry(t, db), "保留条数为 0 时不记录")
}

func TestTelemetryStore_RecentClampsLimit(t *testing.T) {
	db := storage.OpenTemp(t)
	store := NewTelemetryStore(db, func() int { return 1000 })
	defer store.Close()
	for i := 0; i < MaxRecentTelemetry+20; i++ {
		store.Log(map[string]interface{}{"seq": fmt.Sprint(i)})
	}

	items, err := store.Recent(0)
	require.NoError(t, err)
	assert.Len(t, items, 100, "默认 100 条")
	items, err = store.Recent(10_000)
	require.NoError(t, err)
	assert.Len(t, items, MaxRecentTelemetry, "上限 500 条")
	assert.Equal(t, fmt.Sprint(MaxRecentTelemetry+19), items[0]["seq"])
}
