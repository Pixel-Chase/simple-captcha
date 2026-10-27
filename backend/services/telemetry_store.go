package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"simple-captcha/storage"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// MaxRecentTelemetry 管理接口一次最多返回的遥测记录条数
	MaxRecentTelemetry = 500

	telemetryQueueSize     = 1024
	telemetryBatchSize     = 200
	telemetryFlushInterval = time.Second
)

// telemetryRow 一条待写入的风控观察记录
type telemetryRow struct {
	createdAt int64
	kind      string
	appID     string
	payload   []byte
}

// TelemetryStore 风控观察记录：验证请求只把记录放进带缓冲的通道，后台每秒或攒满一批时在一个事务内写入 SQLite，
// 并按条数只保留最近的记录。通道满时丢弃新记录并计数，绝不阻塞验证请求
type TelemetryStore struct {
	db        *storage.DB
	retention func() int // 保留条数，<= 0 表示不记录

	queue    chan telemetryRow
	flushReq chan chan struct{}
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once

	dropped  atomic.Int64
	lastWarn atomic.Int64 // 上次打印丢弃告警的 Unix 秒
}

// NewTelemetryStore 创建并启动后台写入
func NewTelemetryStore(db *storage.DB, retention func() int) *TelemetryStore {
	t := &TelemetryStore{
		db:        db,
		retention: retention,
		queue:     make(chan telemetryRow, telemetryQueueSize),
		flushReq:  make(chan chan struct{}),
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
	go t.run()
	return t
}

// Log 追加一条记录（非阻塞）；entry 序列化为 JSON 原样保存，kind 与 app 字段另外单独成列
func (t *TelemetryStore) Log(entry interface{}) {
	if t == nil || t.retention() <= 0 {
		return
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return
	}
	var head struct {
		Kind string `json:"kind"`
		App  string `json:"app"`
	}
	_ = json.Unmarshal(payload, &head)
	if head.Kind == "" {
		head.Kind = "verify"
	}
	row := telemetryRow{createdAt: time.Now().Unix(), kind: head.Kind, appID: head.App, payload: payload}
	select {
	case t.queue <- row:
	default:
		t.drop()
	}
}

// drop 通道已满：丢弃并计数，告警每分钟最多一次
func (t *TelemetryStore) drop() {
	total := t.dropped.Add(1)
	now := time.Now().Unix()
	last := t.lastWarn.Load()
	if now-last >= 60 && t.lastWarn.CompareAndSwap(last, now) {
		log.Printf("风控观察写入积压，已丢弃新记录: dropped_total=%d", total)
	}
}

// Dropped 因积压被丢弃的记录累计数
func (t *TelemetryStore) Dropped() int64 {
	return t.dropped.Load()
}

// Flush 等待已入队的记录全部写入，供读取前保证"读到自己刚写的"
func (t *TelemetryStore) Flush() {
	ack := make(chan struct{})
	select {
	case t.flushReq <- ack:
		<-ack
	case <-t.done:
	}
}

// Close 停止后台写入，并把剩余记录写完
func (t *TelemetryStore) Close() {
	t.stopOnce.Do(func() { close(t.stop) })
	<-t.done
}

// Recent 读取最近 limit 条记录（新的在前），无法解析的条目跳过；limit 裁剪到 [1, MaxRecentTelemetry]
func (t *TelemetryStore) Recent(limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > MaxRecentTelemetry {
		limit = MaxRecentTelemetry
	}
	t.Flush()

	rows, err := t.db.QueryContext(context.Background(), `SELECT payload FROM telemetry ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var entry map[string]interface{}
		if json.Unmarshal([]byte(payload), &entry) != nil {
			continue
		}
		items = append(items, entry)
	}
	return items, rows.Err()
}

func (t *TelemetryStore) run() {
	defer close(t.done)
	ticker := time.NewTicker(telemetryFlushInterval)
	defer ticker.Stop()

	batch := make([]telemetryRow, 0, telemetryBatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := t.write(batch); err != nil {
			log.Printf("写入风控观察记录失败，本批丢弃: count=%d, error=%v", len(batch), err)
		}
		batch = batch[:0]
	}
	drain := func() {
		for {
			select {
			case row := <-t.queue:
				batch = append(batch, row)
				if len(batch) >= telemetryBatchSize {
					flush()
				}
			default:
				return
			}
		}
	}

	for {
		select {
		case row := <-t.queue:
			batch = append(batch, row)
			if len(batch) >= telemetryBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case ack := <-t.flushReq:
			drain()
			flush()
			close(ack)
		case <-t.stop:
			drain()
			flush()
			return
		}
	}
}

// write 一个事务内写入整批记录，随后按保留条数删除更早的行
func (t *TelemetryStore) write(batch []telemetryRow) error {
	ctx := context.Background()
	return t.db.Tx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO telemetry (created_at, kind, app_id, payload) VALUES (?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, row := range batch {
			var appID interface{}
			if row.appID != "" {
				appID = row.appID
			}
			if _, err := stmt.ExecContext(ctx, row.createdAt, row.kind, appID, string(row.payload)); err != nil {
				return err
			}
		}
		if keep := t.retention(); keep > 0 {
			_, err = tx.ExecContext(ctx, `DELETE FROM telemetry WHERE id <= (SELECT MAX(id) FROM telemetry) - ?`, keep)
			return err
		}
		return nil
	})
}

// sharedTelemetry 全局服务管理器创建的遥测存储；ClientRiskStore 在各处创建，统一写到这里
var sharedTelemetry atomic.Pointer[TelemetryStore]
