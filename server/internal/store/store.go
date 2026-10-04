// Package store:SQLite 持久化(templates/renders 两表,内容列存 JSON 文本,spec §1.1)。
// 内容 JSON 以 RawMessage 原样进出——wire 形态正确性由写端点预检(12 票)与
// go-canvas 解码管线保证,存储层不复制校验逻辑。
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动,CGO_ENABLED=0 可构建
)

// ErrNotFound 寻址失败(id 不存在),handler 层映射 404 template_not_found(spec §2.4)
var ErrNotFound = errors.New("template not found")

// Store SQLite 存储。demo 规模下单连接串行化即可,且天然规避 SQLITE_BUSY
type Store struct {
	db *sql.DB
}

// Open 打开(必要时创建)库并建表;DDL 幂等,启动时调用
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭底层连接
func (s *Store) Close() error { return s.db.Close() }

// migrate 建表:内容均为 JSON 文本列,时间戳 RFC3339 UTC 字符串(spec §2.1)
func (s *Store) migrate() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS templates (
	id             INTEGER PRIMARY KEY AUTOINCREMENT,
	name           TEXT NOT NULL,
	canvases       TEXT NOT NULL,
	flow_chain     TEXT,
	dataset_schema TEXT,
	dataset        TEXT,
	created_at     TEXT NOT NULL,
	updated_at     TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS renders (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	template_id INTEGER NOT NULL,
	images      TEXT NOT NULL,
	created_at  TEXT NOT NULL
);`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("建表: %w", err)
	}
	return nil
}

// CanvasEntry 单画布条目:name = 帧名,graph = 单画布 graph(spec §2.3)
type CanvasEntry struct {
	Name  string          `json:"name"`
	Graph json.RawMessage `json:"graph"`
}

// TemplateContent 模板内容五件套,不含 id 与时间戳——播种与写端点(12 票)共用
type TemplateContent struct {
	Name          string
	Canvases      []CanvasEntry
	FlowChain     json.RawMessage // nil = null = 空链(spec §3.1)
	DatasetSchema json.RawMessage // nil = null
	Dataset       json.RawMessage // nil = null
}

// TemplateRecord 全量模板记录,即 HTTP 响应形状(spec §2.3)
type TemplateRecord struct {
	ID            int64           `json:"id"`
	Name          string          `json:"name"`
	Canvases      []CanvasEntry   `json:"canvases"`
	FlowChain     json.RawMessage `json:"flowChain"`
	DatasetSchema json.RawMessage `json:"datasetSchema"`
	Dataset       json.RawMessage `json:"dataset"`
	CreatedAt     string          `json:"createdAt"`
	UpdatedAt     string          `json:"updatedAt"`
}

// TemplateSummary 列表摘要,仅三字段(spec §2.4 #2)
type TemplateSummary struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	UpdatedAt string `json:"updatedAt"`
}

// SeedTemplateIfEmpty 播种:templates 表空则插入默认模板(幂等——重启不重复插,spec §5.2)
func (s *Store) SeedTemplateIfEmpty(ctx context.Context, content TemplateContent) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM templates`).Scan(&n); err != nil {
		return fmt.Errorf("数模板行: %w", err)
	}
	if n > 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.insertTemplate(ctx, content, now, now)
	return err
}

// insertTemplate 写入一条模板(时间戳由调用方给定,供播种与测试控制排序)
func (s *Store) insertTemplate(ctx context.Context, content TemplateContent, createdAt, updatedAt string) (int64, error) {
	canvases, err := json.Marshal(content.Canvases)
	if err != nil {
		return 0, fmt.Errorf("序列化 canvases: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO templates (name, canvases, flow_chain, dataset_schema, dataset, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		content.Name, string(canvases),
		nullableText(content.FlowChain), nullableText(content.DatasetSchema), nullableText(content.Dataset),
		createdAt, updatedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("插入模板: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("取自增 id: %w", err)
	}
	return id, nil
}

// nullableText nil RawMessage → SQL NULL,否则原文字节入库
func nullableText(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return string(raw)
}

// ListTemplates 摘要数组,updatedAt 降序(spec §2.4 #2);同秒并列以 id 降序稳定排序
func (s *Store) ListTemplates(ctx context.Context) ([]TemplateSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, updated_at FROM templates ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("查询模板列表: %w", err)
	}
	defer rows.Close()
	items := []TemplateSummary{} // 空表也回 [],非 null(spec §2.4 #2 响应为数组)
	for rows.Next() {
		var item TemplateSummary
		if err := rows.Scan(&item.ID, &item.Name, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描模板摘要: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetTemplate 按 id 取全量记录(含 datasetSchema/dataset);不存在 → ErrNotFound
func (s *Store) GetTemplate(ctx context.Context, id int64) (*TemplateRecord, error) {
	var rec TemplateRecord
	var canvases, flowChain, schema, dataset sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, canvases, flow_chain, dataset_schema, dataset, created_at, updated_at
		 FROM templates WHERE id = ?`, id,
	).Scan(&rec.ID, &rec.Name, &canvases, &flowChain, &schema, &dataset, &rec.CreatedAt, &rec.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询模板 %d: %w", id, err)
	}
	if err := json.Unmarshal([]byte(canvases.String), &rec.Canvases); err != nil {
		return nil, fmt.Errorf("解析模板 %d canvases: %w", id, err)
	}
	rec.FlowChain = rawOrNull(flowChain)
	rec.DatasetSchema = rawOrNull(schema)
	rec.Dataset = rawOrNull(dataset)
	return &rec, nil
}

// rawOrNull NULL 列 → nil RawMessage(序列化为 JSON null),否则原文
func rawOrNull(ns sql.NullString) json.RawMessage {
	if !ns.Valid {
		return nil
	}
	return json.RawMessage(ns.String)
}
