// Package store:SQLite 持久化(datasources/templates/renders 三表,内容列存 JSON
// 文本,spec §1.1/§2.3)。数据源是独立实体(独立表 + 独立 CRUD),模板经
// data_source_id 引用——模板不再内嵌 dataset/datasetSchema 拷贝。
// 内容 JSON 以 RawMessage 原样进出——wire 形态正确性由写端点预检与 go-canvas
// 解码管线保证,存储层不复制校验逻辑。
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动,CGO_ENABLED=0 可构建
)

// ErrNotFound 模板寻址失败(id 不存在),handler 层映射 404 template_not_found(spec §2.4)
var ErrNotFound = errors.New("template not found")

// ErrDataSourceNotFound 数据源寻址失败(id 不存在),handler 层映射 404
// data_source_not_found(spec §2.4 数据源段)
var ErrDataSourceNotFound = errors.New("data source not found")

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

// migrate 建表:内容均为 JSON 文本列,时间戳 RFC3339 UTC 字符串(spec §2.1)。
// 自愈迁移:demo 运行库(app.db)若还是「dataset 内嵌模板」的旧三表形态,直接
// 丢弃 templates/renders 重建(运行数据可弃,重启播种自动补回),数据源表新建。
func (s *Store) migrate() error {
	legacy, err := s.hasLegacyTemplateTable()
	if err != nil {
		return err
	}
	if legacy {
		log.Printf("检测到旧版 templates 表(dataset 内嵌形态),重建 templates/renders(数据源改独立实体,运行数据由播种补回)")
		if _, err := s.db.Exec(`DROP TABLE IF EXISTS templates; DROP TABLE IF EXISTS renders;`); err != nil {
			return fmt.Errorf("重建旧版模板表: %w", err)
		}
	}
	const ddl = `
CREATE TABLE IF NOT EXISTS datasources (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	name       TEXT NOT NULL,
	schema     TEXT NOT NULL,
	data       TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS templates (
	id             INTEGER PRIMARY KEY AUTOINCREMENT,
	name           TEXT NOT NULL,
	canvases       TEXT NOT NULL,
	flow_chain     TEXT,
	data_source_id INTEGER,
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

// hasLegacyTemplateTable templates 表存在但无 data_source_id 列 = 旧版形态
func (s *Store) hasLegacyTemplateTable() (bool, error) {
	rows, err := s.db.Query(`PRAGMA table_info(templates)`)
	if err != nil {
		return false, fmt.Errorf("探测 templates 表: %w", err)
	}
	defer rows.Close()
	hasTable, hasRef := false, false
	for rows.Next() {
		hasTable = true
		var cid int
		var name, ctype string
		var notNull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return false, fmt.Errorf("读 templates 列: %w", err)
		}
		if name == "data_source_id" {
			hasRef = true
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("遍历 templates 列: %w", err)
	}
	return hasTable && !hasRef, nil
}

// CanvasEntry 单画布条目:name = 帧名,graph = 单画布 graph(spec §2.3)
type CanvasEntry struct {
	Name  string          `json:"name"`
	Graph json.RawMessage `json:"graph"`
}

// DataSourceContent 数据源实体内容三件套,不含 id 与时间戳——播种与写端点共用
type DataSourceContent struct {
	Name   string
	Schema json.RawMessage
	Data   json.RawMessage
}

// DataSourceRecord 全量数据源记录,即 HTTP 响应形状(spec §2.3)
type DataSourceRecord struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Schema    json.RawMessage `json:"schema"`
	Data      json.RawMessage `json:"data"`
	CreatedAt string          `json:"createdAt"`
	UpdatedAt string          `json:"updatedAt"`
}

// DataSourceSummary 数据源列表摘要:id/name + 引用它的模板数 + updatedAt
// (spec §2.4 数据源段;引用计数供抽屉「共享影响面」注记)
type DataSourceSummary struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	TemplateCount int    `json:"templateCount"`
	UpdatedAt     string `json:"updatedAt"`
}

// TemplateContent 模板内容,不含 id 与时间戳——播种与写端点共用。
// DataSourceID = 绑定的数据源引用(nil = 未绑,渲染直通零行空壳页);
// 数据源内容不在此——模板只持引用(spec §2.3)
type TemplateContent struct {
	Name         string
	Canvases     []CanvasEntry
	FlowChain    json.RawMessage // nil = null = 空链(spec §3.1)
	DataSourceID *int64
}

// TemplateRecord 全量模板记录,即 HTTP 响应形状(spec §2.3):
// dataSourceId 为引用列(null = 未绑数据源)
type TemplateRecord struct {
	ID           int64           `json:"id"`
	Name         string          `json:"name"`
	Canvases     []CanvasEntry   `json:"canvases"`
	FlowChain    json.RawMessage `json:"flowChain"`
	DataSourceID *int64          `json:"dataSourceId"`
	CreatedAt    string          `json:"createdAt"`
	UpdatedAt    string          `json:"updatedAt"`
}

// TemplateSummary 列表摘要,仅三字段(spec §2.4 #2)
type TemplateSummary struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	UpdatedAt string `json:"updatedAt"`
}

// nowRFC3339 当前时刻的 RFC3339 UTC 字符串(spec §2.1 时间戳口径)
func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// SeedDataSourceIfMissing 数据源播种:按名取回既有 id,不存在才插入(幂等,
// spec §5.2)——模板播种依赖其返回 id
func (s *Store) SeedDataSourceIfMissing(ctx context.Context, content DataSourceContent) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM datasources WHERE name = ? ORDER BY id LIMIT 1`, content.Name,
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("查数据源 %q: %w", content.Name, err)
	}
	now := nowRFC3339()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO datasources (name, schema, data, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		content.Name, string(content.Schema), string(content.Data), now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("插入数据源: %w", err)
	}
	id, err = res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("取数据源自增 id: %w", err)
	}
	return id, nil
}

// insertTemplate 写入一条模板(时间戳由调用方给定,供播种与测试控制排序)
func (s *Store) insertTemplate(ctx context.Context, content TemplateContent, createdAt, updatedAt string) (int64, error) {
	canvases, err := json.Marshal(content.Canvases)
	if err != nil {
		return 0, fmt.Errorf("序列化 canvases: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO templates (name, canvases, flow_chain, data_source_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		content.Name, string(canvases), nullableText(content.FlowChain), nullableInt(content.DataSourceID),
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

// CreateTemplate 写入新模板并返回自增 id,createdAt = updatedAt(spec §2.4 #3:
// POST /api/templates,id 自增分配;dataSourceId 随载荷绑定,写端点先验存在)
func (s *Store) CreateTemplate(ctx context.Context, content TemplateContent) (int64, error) {
	now := nowRFC3339()
	return s.insertTemplate(ctx, content, now, now)
}

// SeedTemplateIfEmpty 播种:templates 表空则插入默认模板(幂等——重启不重复插,
// spec §5.2;绑定 id 由启动序在数据源播种后接线)
func (s *Store) SeedTemplateIfEmpty(ctx context.Context, content TemplateContent) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM templates`).Scan(&n); err != nil {
		return fmt.Errorf("数模板行: %w", err)
	}
	if n > 0 {
		return nil
	}
	now := nowRFC3339()
	_, err := s.insertTemplate(ctx, content, now, now)
	return err
}

// UpdateTemplateContent 整存替换 name/canvases/flow_chain 三列并刷新 updatedAt,
// 不触碰 data_source_id(绑定只经数据源通道变更,spec §2.4 #5)。id 不存在 → ErrNotFound
func (s *Store) UpdateTemplateContent(ctx context.Context, id int64, content TemplateContent) error {
	canvases, err := json.Marshal(content.Canvases)
	if err != nil {
		return fmt.Errorf("序列化 canvases: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE templates SET name = ?, canvases = ?, flow_chain = ?, updated_at = ? WHERE id = ?`,
		content.Name, string(canvases), nullableText(content.FlowChain),
		nowRFC3339(), id,
	)
	if err != nil {
		return fmt.Errorf("更新模板 %d: %w", id, err)
	}
	return requireRow(res, "更新模板", id, ErrNotFound)
}

// UpdateTemplateDataSource 整存替换 data_source_id 引用列并刷新 updatedAt
// (spec §2.4 #6 绑定通道;null = 解绑。引用存在性由 handler 先验)。
// id 不存在 → ErrNotFound
func (s *Store) UpdateTemplateDataSource(ctx context.Context, id int64, dataSourceID *int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE templates SET data_source_id = ?, updated_at = ? WHERE id = ?`,
		nullableInt(dataSourceID), nowRFC3339(), id,
	)
	if err != nil {
		return fmt.Errorf("更新模板 %d 数据源绑定: %w", id, err)
	}
	return requireRow(res, "更新模板数据源绑定", id, ErrNotFound)
}

// DeleteTemplate 删除模板行并级联清其渲染记录行(无渲染历史端点,模板删后
// 记录行不可达);渲染产物 PNG 文件保留——keep-all 快照语义,/renders 直链仍可用。
// 数据源实体不受影响(独立资源,引用随模板行整行消失,不悬空)。
// 事务保证级联面原子:先删 renders 再删 templates,id 不存在 → ErrNotFound
// (回滚,已执行的级联删除一并撤销)
func (s *Store) DeleteTemplate(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启模板删除事务: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Commit 后为 no-op
	if _, err := tx.ExecContext(ctx, `DELETE FROM renders WHERE template_id = ?`, id); err != nil {
		return fmt.Errorf("级联删除模板 %d 渲染记录: %w", id, err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM templates WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除模板 %d: %w", id, err)
	}
	if err := requireRow(res, "删除模板", id, ErrNotFound); err != nil {
		return err
	}
	return tx.Commit()
}

// requireRow UPDATE 影响行数为 0 即寻址失败(notFound 决定映射哪个 404 码)
func requireRow(res sql.Result, op string, id int64, notFound error) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s %d: 取影响行数: %w", op, id, err)
	}
	if n == 0 {
		return notFound
	}
	return nil
}

// nullableText nil RawMessage → SQL NULL,否则原文字节入库
func nullableText(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return string(raw)
}

// nullableInt nil 指针 → SQL NULL,否则整数值入库
func nullableInt(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
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

// GetTemplate 按 id 取全量记录;不存在 → ErrNotFound
func (s *Store) GetTemplate(ctx context.Context, id int64) (*TemplateRecord, error) {
	var rec TemplateRecord
	var canvases, flowChain sql.NullString
	var dataSourceID sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, canvases, flow_chain, data_source_id, created_at, updated_at
		 FROM templates WHERE id = ?`, id,
	).Scan(&rec.ID, &rec.Name, &canvases, &flowChain, &dataSourceID, &rec.CreatedAt, &rec.UpdatedAt)
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
	if dataSourceID.Valid {
		rec.DataSourceID = &dataSourceID.Int64
	}
	return &rec, nil
}

// rawOrNull NULL 列 → nil RawMessage(序列化为 JSON null),否则原文
func rawOrNull(ns sql.NullString) json.RawMessage {
	if !ns.Valid {
		return nil
	}
	return json.RawMessage(ns.String)
}

// --- 数据源实体 CRUD(spec §2.4 数据源段) ---

// insertDataSource 写入一条数据源(时间戳由调用方给定,供测试控制)
func (s *Store) insertDataSource(ctx context.Context, content DataSourceContent, createdAt, updatedAt string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO datasources (name, schema, data, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		content.Name, string(content.Schema), string(content.Data), createdAt, updatedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("插入数据源: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("取数据源自增 id: %w", err)
	}
	return id, nil
}

// CreateDataSource 写入新数据源并返回自增 id,createdAt = updatedAt
func (s *Store) CreateDataSource(ctx context.Context, content DataSourceContent) (int64, error) {
	now := nowRFC3339()
	return s.insertDataSource(ctx, content, now, now)
}

// UpdateDataSource 整存替换 name/schema/data 三列并刷新 updatedAt。
// id 不存在 → ErrDataSourceNotFound
func (s *Store) UpdateDataSource(ctx context.Context, id int64, content DataSourceContent) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE datasources SET name = ?, schema = ?, data = ?, updated_at = ? WHERE id = ?`,
		content.Name, string(content.Schema), string(content.Data), nowRFC3339(), id,
	)
	if err != nil {
		return fmt.Errorf("更新数据源 %d: %w", id, err)
	}
	return requireRow(res, "更新数据源", id, ErrDataSourceNotFound)
}

// GetDataSource 按 id 取全量数据源记录;不存在 → ErrDataSourceNotFound
func (s *Store) GetDataSource(ctx context.Context, id int64) (*DataSourceRecord, error) {
	var rec DataSourceRecord
	// schema/data 列经 string 中转再转 RawMessage(本工具链下 json.RawMessage
	// 别名 jsontext.Value,driver 扫描不走 []byte 转换)
	var schema, data string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, schema, data, created_at, updated_at FROM datasources WHERE id = ?`, id,
	).Scan(&rec.ID, &rec.Name, &schema, &data, &rec.CreatedAt, &rec.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDataSourceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询数据源 %d: %w", id, err)
	}
	rec.Schema, rec.Data = json.RawMessage(schema), json.RawMessage(data)
	return &rec, nil
}

// ListDataSources 摘要数组(含引用计数),updatedAt 降序、同秒 id 降序稳定;
// 空表也回 [] 非 null
func (s *Store) ListDataSources(ctx context.Context) ([]DataSourceSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT d.id, d.name, d.updated_at, COUNT(t.id)
		 FROM datasources d LEFT JOIN templates t ON t.data_source_id = d.id
		 GROUP BY d.id ORDER BY d.updated_at DESC, d.id DESC`)
	if err != nil {
		return nil, fmt.Errorf("查询数据源列表: %w", err)
	}
	defer rows.Close()
	items := []DataSourceSummary{}
	for rows.Next() {
		var item DataSourceSummary
		if err := rows.Scan(&item.ID, &item.Name, &item.UpdatedAt, &item.TemplateCount); err != nil {
			return nil, fmt.Errorf("扫描数据源摘要: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// --- 渲染记录(13 票,spec §2.4 #7)---

// RenderImage 渲染产物条目:Path 为磁盘相对形态 "renders/<id>/<n>.png"
// (DB 存储与落盘形态,spec §2.2;响应侧转前导斜杠 url)
type RenderImage struct {
	Frame int    `json:"frame"`
	Name  string `json:"name"`
	Path  string `json:"path"`
}

// RenderRecord 渲染记录(spec §2.3):images = 帧序×页序扁平页序列
type RenderRecord struct {
	ID         int64         `json:"id"`
	TemplateID int64         `json:"templateId"`
	CreatedAt  string        `json:"createdAt"`
	Images     []RenderImage `json:"images"`
}

// CreateRenderRecord 先落一条 stub 记录(images 空数组)取自增 id——产物文件名
// renders/<id>/<seq>.png 依赖 id 先行;落盘成功后经 UpdateRenderImages 回填。
// 落库失败由 handler 映射 500 internal_error(spec §2.5)
func (s *Store) CreateRenderRecord(ctx context.Context, templateID int64) (*RenderRecord, error) {
	now := nowRFC3339()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO renders (template_id, images, created_at) VALUES (?, '[]', ?)`,
		templateID, now,
	)
	if err != nil {
		return nil, fmt.Errorf("插入渲染记录: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("取渲染记录自增 id: %w", err)
	}
	return &RenderRecord{ID: id, TemplateID: templateID, CreatedAt: now, Images: []RenderImage{}}, nil
}

// UpdateRenderImages 回填 images JSON(产物全部落盘成功后调用)
func (s *Store) UpdateRenderImages(ctx context.Context, id int64, images []RenderImage) error {
	blob, err := json.Marshal(images)
	if err != nil {
		return fmt.Errorf("序列化渲染 images: %w", err)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE renders SET images = ? WHERE id = ?`, string(blob), id)
	if err != nil {
		return fmt.Errorf("回填渲染记录 %d: %w", id, err)
	}
	return requireRow(res, "回填渲染记录", id, ErrNotFound)
}

// DeleteRenderRecord 删除 stub 记录(产物落盘失败时回滚;AUTOINCREMENT 保证
// id 不复用,残留半成品文件不会与新渲染的产物路径相撞)
func (s *Store) DeleteRenderRecord(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM renders WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除渲染记录 %d: %w", id, err)
	}
	return requireRow(res, "删除渲染记录", id, ErrNotFound)
}
