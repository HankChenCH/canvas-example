package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"example/server/internal/pages"
)

// openTestStore 每用例独立临时库(文件落 TempDir,互不串扰)
func openTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("打开测试库: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// seedDataSource 最小合法数据源内容(schema/data 与 spec §2.3 样例同形)
func seedDataSource() DataSourceContent {
	return DataSourceContent{
		Name:   "结业证书批量打印数据源",
		Schema: json.RawMessage(`{"type":"object"}`),
		Data:   json.RawMessage(`{"org":{"name":"瀚辰培训中心"}}`),
	}
}

// seedPage 页构造器:RawMessage 字段承接原文(页 id/name 引号补齐)
func seedPage(id, name, graph, flow string) pages.Page {
	p := pages.Page{
		ID:    json.RawMessage(`"` + id + `"`),
		Name:  json.RawMessage(`"` + name + `"`),
		Graph: json.RawMessage(graph),
	}
	if flow != "" {
		p.Flow = json.RawMessage(flow)
	}
	return p
}

// seedContent 最小合法模板内容(两页 + 流策略,形状对齐 spec §2.3);绑定由调用方
// 经 DataSourceID 接线(模板只持引用)
func seedContent() TemplateContent {
	return TemplateContent{
		Name: "结业证书 · 批量打印页",
		Pages: []pages.Page{
			seedPage("pg_main", "主页", `{"canvas":{"width":794,"height":1123},"layers":[]}`, `{"mode":"fixed"}`),
			seedPage("pg_cont", "续页", `{"canvas":{"width":794,"height":1123},"layers":[]}`, `{"mode":"paged","omitIfEmpty":true}`),
		},
	}
}

func countRows(t *testing.T, st *Store, table string) int {
	t.Helper()
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("数 %s 行: %v", table, err)
	}
	return n
}

func TestOpenCreatesTables(t *testing.T) {
	st := openTestStore(t)
	for _, table := range []string{"datasources", "templates", "renders", "template_versions"} {
		var name string
		err := st.db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name = ?`, table,
		).Scan(&name)
		if err != nil {
			t.Fatalf("表 %s 未建: %v", table, err)
		}
	}
}

// TestOpen_SelfHealLegacySchema 旧版库(页序列翻转前的 canvases+flow_chain 双列
// 形态,ADR 0025)启动自愈:整库重建 templates/template_versions/renders,demo
// 运行数据由播种补回(editor_state 存量随重建清零属预期)
func TestOpen_SelfHealLegacySchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	// 手工造旧版形态:templates 带 canvases/flow_chain 双列、无 pages 列
	st, err := Open(dbPath) // 先经新版 Open 建出基线库再降级改列
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`DROP TABLE templates;
		CREATE TABLE templates (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			name           TEXT NOT NULL,
			canvases       TEXT NOT NULL,
			flow_chain     TEXT,
			data_source_id INTEGER,
			editor_state   TEXT,
			created_at     TEXT NOT NULL,
			updated_at     TEXT NOT NULL
		);`); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("旧库自愈打开: %v", err)
	}
	defer st2.Close()
	var hasPages int
	if err := st2.db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('templates') WHERE name = 'pages'`,
	).Scan(&hasPages); err != nil || hasPages != 1 {
		t.Fatalf("templates 应重建为单列 pages 形态: count=%d err=%v", hasPages, err)
	}
	// 重建后的三表齐备(可正常读写)
	if _, err := st2.CreateTemplate(context.Background(), seedContent()); err != nil {
		t.Fatalf("重建后应可正常写入: %v", err)
	}
}

func TestSeedDataSourceIfMissing_GetOrCreate(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	id1, err := st.SeedDataSourceIfMissing(ctx, seedDataSource())
	if err != nil {
		t.Fatalf("首播: %v", err)
	}
	// 同名再播:按名取回既有 id,幂等不重插
	id2, err := st.SeedDataSourceIfMissing(ctx, seedDataSource())
	if err != nil {
		t.Fatalf("重播: %v", err)
	}
	if id1 != id2 || countRows(t, st, "datasources") != 1 {
		t.Fatalf("同名播种应幂等: %d vs %d, rows=%d", id1, id2, countRows(t, st, "datasources"))
	}
}

func TestSeedTemplateIfEmpty_InsertsOnce(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	if err := st.SeedTemplateIfEmpty(ctx, seedContent()); err != nil {
		t.Fatalf("首播: %v", err)
	}
	// 重启再播:幂等,不重复插
	if err := st.SeedTemplateIfEmpty(ctx, seedContent()); err != nil {
		t.Fatalf("重播: %v", err)
	}
	if got := countRows(t, st, "templates"); got != 1 {
		t.Fatalf("播种后模板行数 = %d, 期望 1", got)
	}
}

func TestSeedTemplateIfEmpty_NonEmptyTableNoop(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	other := seedContent()
	other.Name = "手工模板"
	if err := st.SeedTemplateIfEmpty(ctx, other); err != nil {
		t.Fatalf("预置模板: %v", err)
	}
	if err := st.SeedTemplateIfEmpty(ctx, seedContent()); err != nil {
		t.Fatalf("表非空时播种应静默跳过: %v", err)
	}
	if got := countRows(t, st, "templates"); got != 1 {
		t.Fatalf("模板行数 = %d, 期望 1(表非空不插)", got)
	}
	rec, err := st.GetTemplate(ctx, 1)
	if err != nil {
		t.Fatalf("取模板: %v", err)
	}
	if rec.Name != "手工模板" {
		t.Fatalf("表非空时播种不得写入: name = %q", rec.Name)
	}
}

func TestListTemplates_UpdatedAtDesc(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	// 直插控制时间戳:验证排序口径(updatedAt 降序,同秒 id 降序稳定)
	if _, err := st.insertTemplate(ctx, seedContent(), "2026-10-04T12:00:00Z", "2026-10-04T12:00:00Z"); err != nil {
		t.Fatalf("插模板 1: %v", err)
	}
	if _, err := st.insertTemplate(ctx, seedContent(), "2026-10-04T13:00:00Z", "2026-10-04T13:00:00Z"); err != nil {
		t.Fatalf("插模板 2: %v", err)
	}
	if _, err := st.insertTemplate(ctx, seedContent(), "2026-10-04T12:00:00Z", "2026-10-04T12:00:00Z"); err != nil {
		t.Fatalf("插模板 3: %v", err)
	}
	items, err := st.ListTemplates(ctx)
	if err != nil {
		t.Fatalf("列表: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("列表长度 = %d, 期望 3", len(items))
	}
	got := []int64{items[0].ID, items[1].ID, items[2].ID}
	want := []int64{2, 3, 1} // 13:00 最新在前;同秒 12:00 两行 id 降序
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("排序 = %v, 期望 %v (updatedAt 降序 + id 降序稳定)", got, want)
		}
	}
}

func TestGetTemplate_Roundtrip(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	dsID, err := st.SeedDataSourceIfMissing(ctx, seedDataSource())
	if err != nil {
		t.Fatalf("播种数据源: %v", err)
	}
	content := seedContent()
	content.DataSourceID = &dsID
	if err := st.SeedTemplateIfEmpty(ctx, content); err != nil {
		t.Fatalf("播种: %v", err)
	}
	rec, err := st.GetTemplate(ctx, 1)
	if err != nil {
		t.Fatalf("取模板: %v", err)
	}
	if rec.ID != 1 || rec.Name != "结业证书 · 批量打印页" {
		t.Fatalf("id/name = %d/%q", rec.ID, rec.Name)
	}
	if rec.DataSourceID == nil || *rec.DataSourceID != dsID {
		t.Fatalf("dataSourceId 应为播种绑定的引用: %v", rec.DataSourceID)
	}
	// pages 往返:页 id/name/graph/flow 原样(页序 = 存储序)
	if len(rec.Pages) != 2 {
		t.Fatalf("pages 应两页: %+v", rec.Pages)
	}
	var id0, name0 string
	if err := json.Unmarshal(rec.Pages[0].ID, &id0); err != nil || id0 != "pg_main" {
		t.Fatalf("页 0 id 应为 pg_main: %s (%v)", rec.Pages[0].ID, err)
	}
	if err := json.Unmarshal(rec.Pages[0].Name, &name0); err != nil || name0 != "主页" {
		t.Fatalf("页 0 name 应为主页: %s (%v)", rec.Pages[0].Name, err)
	}
	if !strings.Contains(string(rec.Pages[0].Graph), `"width":794`) {
		t.Fatalf("页 0 graph 内容不符: %s", rec.Pages[0].Graph)
	}
	if string(rec.Pages[0].Flow) != `{"mode":"fixed"}` {
		t.Fatalf("页 0 flow 应为 fixed: %s", rec.Pages[0].Flow)
	}
	if string(rec.Pages[1].Flow) != `{"mode":"paged","omitIfEmpty":true}` {
		t.Fatalf("页 1 flow 应为 paged+omitIfEmpty: %s", rec.Pages[1].Flow)
	}
	if rec.CreatedAt == "" || rec.CreatedAt != rec.UpdatedAt {
		t.Fatalf("播种记录 createdAt = updatedAt,实得 %q / %q", rec.CreatedAt, rec.UpdatedAt)
	}
	// RFC3339 UTC 形态(spec §2.1),如 2026-10-04T12:00:00Z
	if !strings.HasSuffix(rec.UpdatedAt, "Z") || strings.Contains(rec.UpdatedAt, "+") {
		t.Fatalf("时间戳应为 RFC3339 UTC: %q", rec.UpdatedAt)
	}
}

// TestGetTemplate_EmptyPagesMarshalEmptyArray nil 页序列归一为 "[]" 入库,响应
// 序列化为 "pages":[](响应数组恒 [] 非 null);未绑引用列照旧 JSON null
func TestGetTemplate_EmptyPagesMarshalEmptyArray(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	content := seedContent()
	content.Pages = nil
	if _, err := st.insertTemplate(ctx, content, "2026-10-04T12:00:00Z", "2026-10-04T12:00:00Z"); err != nil {
		t.Fatalf("插模板: %v", err)
	}
	rec, err := st.GetTemplate(ctx, 1)
	if err != nil {
		t.Fatalf("取模板: %v", err)
	}
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("序列化: %v", err)
	}
	for _, key := range []string{`"pages":[]`, `"dataSourceId":null`} {
		if !strings.Contains(string(b), key) {
			t.Fatalf("列形状应序列化为 %s(缺失): %s", key, b)
		}
	}
}

func TestGetTemplate_NotFound(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.GetTemplate(context.Background(), 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("期望 ErrNotFound, 实得 %v", err)
	}
}

func TestCreateTemplate_AutoIncrementAndTimestamps(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	// POST 载荷缺省未绑(spec §2.4 #3)——引用列留空
	id1, err := st.CreateTemplate(ctx, seedContent())
	if err != nil {
		t.Fatalf("创建模板 1: %v", err)
	}
	id2, err := st.CreateTemplate(ctx, seedContent())
	if err != nil {
		t.Fatalf("创建模板 2: %v", err)
	}
	if id1 != 1 || id2 != id1+1 {
		t.Fatalf("id 应自增分配: 实得 %d → %d", id1, id2)
	}

	rec, err := st.GetTemplate(ctx, id1)
	if err != nil {
		t.Fatalf("回读: %v", err)
	}
	// 未带引用的创建落 NULL,createdAt = updatedAt
	if rec.DataSourceID != nil {
		t.Fatalf("未绑数据源的创建应落 NULL: %v", rec.DataSourceID)
	}
	if rec.CreatedAt == "" || rec.CreatedAt != rec.UpdatedAt {
		t.Fatalf("createdAt 应等于 updatedAt,实得 %q / %q", rec.CreatedAt, rec.UpdatedAt)
	}
	if !strings.HasSuffix(rec.UpdatedAt, "Z") {
		t.Fatalf("时间戳应为 RFC3339 UTC: %q", rec.UpdatedAt)
	}
}

// TestUpdateTemplateContent_PreservesBinding 整存替换只动 name/pages,
// 不触碰 data_source_id(绑定只经数据源绑定通道变更,spec §2.4 #5)
func TestUpdateTemplateContent_PreservesBinding(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	dsID, err := st.SeedDataSourceIfMissing(ctx, seedDataSource())
	if err != nil {
		t.Fatalf("播种数据源: %v", err)
	}
	content := seedContent()
	content.DataSourceID = &dsID
	if err := st.SeedTemplateIfEmpty(ctx, content); err != nil {
		t.Fatalf("播种: %v", err)
	}

	updated := TemplateContent{
		Name: "改名后的模板",
		Pages: []pages.Page{
			seedPage("pg_new", "新页", `{"canvas":{"width":100,"height":100},"layers":[]}`, ""),
		},
	}
	if err := st.UpdateTemplateContent(ctx, 1, updated); err != nil {
		t.Fatalf("更新模板: %v", err)
	}

	rec, err := st.GetTemplate(ctx, 1)
	if err != nil {
		t.Fatalf("回读: %v", err)
	}
	if rec.Name != "改名后的模板" || len(rec.Pages) != 1 || string(rec.Pages[0].Name) != `"新页"` {
		t.Fatalf("name/pages 应整存替换: %+v", rec)
	}
	if rec.Pages[0].Flow != nil {
		t.Fatalf("无 flow 页应存储为键缺席: %s", rec.Pages[0].Flow)
	}
	if rec.DataSourceID == nil || *rec.DataSourceID != dsID {
		t.Fatalf("data_source_id 不得被触碰: %v", rec.DataSourceID)
	}
}

func TestUpdateTemplateContent_NotFound(t *testing.T) {
	st := openTestStore(t)
	if err := st.UpdateTemplateContent(context.Background(), 999, seedContent()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("期望 ErrNotFound, 实得 %v", err)
	}
}

func TestUpdateTemplateDataSource_BindUnbind(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	dsID, err := st.SeedDataSourceIfMissing(ctx, seedDataSource())
	if err != nil {
		t.Fatalf("播种数据源: %v", err)
	}
	if err := st.SeedTemplateIfEmpty(ctx, seedContent()); err != nil {
		t.Fatalf("播种模板: %v", err)
	}

	// 绑定:引用列整存替换,模板内容不触碰
	if err := st.UpdateTemplateDataSource(ctx, 1, &dsID); err != nil {
		t.Fatalf("绑定: %v", err)
	}
	rec, err := st.GetTemplate(ctx, 1)
	if err != nil {
		t.Fatalf("回读: %v", err)
	}
	if rec.DataSourceID == nil || *rec.DataSourceID != dsID {
		t.Fatalf("绑定后 dataSourceId = %v, 期望 %d", rec.DataSourceID, dsID)
	}
	if rec.Name != seedContent().Name || len(rec.Pages) != 2 {
		t.Fatalf("绑定不得触碰模板内容: %+v", rec)
	}

	// 解绑:null 引用
	if err := st.UpdateTemplateDataSource(ctx, 1, nil); err != nil {
		t.Fatalf("解绑: %v", err)
	}
	rec, err = st.GetTemplate(ctx, 1)
	if err != nil {
		t.Fatalf("回读: %v", err)
	}
	if rec.DataSourceID != nil {
		t.Fatalf("解绑后 dataSourceId 应为 null: %v", rec.DataSourceID)
	}
}

func TestUpdateTemplateDataSource_NotFound(t *testing.T) {
	st := openTestStore(t)
	if err := st.UpdateTemplateDataSource(context.Background(), 999, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("期望 ErrNotFound, 实得 %v", err)
	}
}

// --- 工程数据通道(editor_state 列,工程 spec §2/§4/§8) ---

// TestGetTemplate_EditorStateNullDefault 从未保存工程数据的模板 editor_state 为
// NULL,响应序列化为 "editorState":null(全默认态,spec §4)
func TestGetTemplate_EditorStateNullDefault(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := st.CreateTemplate(ctx, seedContent()); err != nil {
		t.Fatalf("创建模板: %v", err)
	}
	rec, err := st.GetTemplate(ctx, 1)
	if err != nil {
		t.Fatalf("取模板: %v", err)
	}
	if rec.EditorState != nil {
		t.Fatalf("未保存状态应为 nil: %s", rec.EditorState)
	}
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("序列化: %v", err)
	}
	if !strings.Contains(string(b), `"editorState":null`) {
		t.Fatalf("NULL 列应序列化为 \"editorState\":null: %s", b)
	}
}

// TestUpdateTemplateEditorState_LWW 写分道整列替换:载荷原样字节存取,后写覆盖
// 先写;name/pages/data_source_id/created_at/updated_at 一概不动
// (拖参考线不污染「最近编辑」排序,spec §4)
func TestUpdateTemplateEditorState_LWW(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	dsID, err := st.SeedDataSourceIfMissing(ctx, seedDataSource())
	if err != nil {
		t.Fatalf("播种数据源: %v", err)
	}
	content := seedContent()
	content.DataSourceID = &dsID
	if err := st.SeedTemplateIfEmpty(ctx, content); err != nil {
		t.Fatalf("播种模板: %v", err)
	}
	before, err := st.GetTemplate(ctx, 1)
	if err != nil {
		t.Fatalf("取模板: %v", err)
	}

	snapshotA := json.RawMessage(`{"schemaVersion": 1, "frames": {"主页": {"guides": [{"id": "g1", "orientation": "vertical", "position": 120}], "lockedPaths": ["主页#标题"]}}, "prefs": {"rulersVisible": true}}`)
	if err := st.UpdateTemplateEditorState(ctx, 1, snapshotA); err != nil {
		t.Fatalf("首次写状态: %v", err)
	}
	afterA, err := st.GetTemplate(ctx, 1)
	if err != nil {
		t.Fatalf("回读: %v", err)
	}
	if string(afterA.EditorState) != string(snapshotA) {
		t.Fatalf("载荷应原样字节透传: %s", afterA.EditorState)
	}
	if afterA.Name != before.Name || afterA.CreatedAt != before.CreatedAt || afterA.UpdatedAt != before.UpdatedAt {
		t.Fatalf("状态写入不得触碰 name/时间戳: %+v vs %+v", afterA, before)
	}
	if len(afterA.Pages) != len(before.Pages) || string(afterA.Pages[0].Graph) != string(before.Pages[0].Graph) || afterA.DataSourceID == nil || *afterA.DataSourceID != dsID {
		t.Fatalf("状态写入不得触碰 pages/dataSourceId: %+v vs %+v", afterA, before)
	}

	// LWW:后写整列覆盖先写
	snapshotB := json.RawMessage(`{"schemaVersion":1,"frames":{},"prefs":{"anchorExpanded":true}}`)
	if err := st.UpdateTemplateEditorState(ctx, 1, snapshotB); err != nil {
		t.Fatalf("覆盖写状态: %v", err)
	}
	afterB, err := st.GetTemplate(ctx, 1)
	if err != nil {
		t.Fatalf("回读: %v", err)
	}
	if string(afterB.EditorState) != string(snapshotB) {
		t.Fatalf("后写应整列覆盖先写: %s", afterB.EditorState)
	}
	if afterB.UpdatedAt != before.UpdatedAt {
		t.Fatalf("状态写入不得刷新 updatedAt: %q vs %q", afterB.UpdatedAt, before.UpdatedAt)
	}
}

// TestUpdateTemplateEditorState_EmptyIsNULL 空 RawMessage 归 SQL NULL(nil =
// 从未保存 = 全默认态);null 字面量的归一在 handler 层(normalizeNullJSON)
func TestUpdateTemplateEditorState_EmptyIsNULL(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := st.CreateTemplate(ctx, seedContent()); err != nil {
		t.Fatalf("创建模板: %v", err)
	}
	snapshot := json.RawMessage(`{"schemaVersion":1,"frames":{},"prefs":{}}`)
	if err := st.UpdateTemplateEditorState(ctx, 1, snapshot); err != nil {
		t.Fatalf("写状态: %v", err)
	}
	if err := st.UpdateTemplateEditorState(ctx, 1, nil); err != nil {
		t.Fatalf("写空: %v", err)
	}
	rec, err := st.GetTemplate(ctx, 1)
	if err != nil {
		t.Fatalf("回读: %v", err)
	}
	if rec.EditorState != nil {
		t.Fatalf("空载荷应归 SQL NULL: %s", rec.EditorState)
	}
}

func TestUpdateTemplateEditorState_NotFound(t *testing.T) {
	st := openTestStore(t)
	if err := st.UpdateTemplateEditorState(context.Background(), 999, json.RawMessage(`{}`)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("期望 ErrNotFound, 实得 %v", err)
	}
}

// TestUpdateTemplateContent_PreservesEditorState 内容保存只动 name/pages 与
// updatedAt,不冲掉 editor_state(写分道互不越界,spec §4)
func TestUpdateTemplateContent_PreservesEditorState(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.SeedTemplateIfEmpty(ctx, seedContent()); err != nil {
		t.Fatalf("播种: %v", err)
	}
	snapshot := json.RawMessage(`{"schemaVersion":1,"frames":{},"prefs":{}}`)
	if err := st.UpdateTemplateEditorState(ctx, 1, snapshot); err != nil {
		t.Fatalf("写状态: %v", err)
	}

	if err := st.UpdateTemplateContent(ctx, 1, TemplateContent{
		Name:  "改名后的模板",
		Pages: []pages.Page{seedPage("pg_new", "新页", `{"canvas":{"width":100,"height":100},"layers":[]}`, "")},
	}); err != nil {
		t.Fatalf("更新内容: %v", err)
	}
	rec, err := st.GetTemplate(ctx, 1)
	if err != nil {
		t.Fatalf("回读: %v", err)
	}
	if rec.Name != "改名后的模板" || len(rec.Pages) != 1 {
		t.Fatalf("内容应整存替换: %+v", rec)
	}
	if string(rec.EditorState) != string(snapshot) {
		t.Fatalf("内容保存不得冲掉 editor_state: %s", rec.EditorState)
	}
}

// --- 数据源实体 CRUD ---

func TestDataSourceCRUD(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	dsID, err := st.CreateDataSource(ctx, seedDataSource())
	if err != nil {
		t.Fatalf("创建数据源: %v", err)
	}
	rec, err := st.GetDataSource(ctx, dsID)
	if err != nil {
		t.Fatalf("取数据源: %v", err)
	}
	if rec.Name != seedDataSource().Name || string(rec.Schema) != `{"type":"object"}` {
		t.Fatalf("数据源记录不符: %+v", rec)
	}
	if rec.CreatedAt == "" || rec.CreatedAt != rec.UpdatedAt || !strings.HasSuffix(rec.UpdatedAt, "Z") {
		t.Fatalf("createdAt = updatedAt 且 RFC3339 UTC,实得 %q / %q", rec.CreatedAt, rec.UpdatedAt)
	}

	// 整存替换三列
	if err := st.UpdateDataSource(ctx, dsID, DataSourceContent{
		Name:   "改名数据源",
		Schema: json.RawMessage(`{"type":"array"}`),
		Data:   json.RawMessage(`[1,2]`),
	}); err != nil {
		t.Fatalf("更新数据源: %v", err)
	}
	rec, err = st.GetDataSource(ctx, dsID)
	if err != nil {
		t.Fatalf("回读: %v", err)
	}
	if rec.Name != "改名数据源" || string(rec.Schema) != `{"type":"array"}` || string(rec.Data) != `[1,2]` {
		t.Fatalf("三列应整存替换: %+v", rec)
	}
}

func TestGetDataSource_NotFound(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.GetDataSource(context.Background(), 999); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatalf("期望 ErrDataSourceNotFound, 实得 %v", err)
	}
	if err := st.UpdateDataSource(context.Background(), 999, seedDataSource()); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatalf("更新期望 ErrDataSourceNotFound, 实得 %v", err)
	}
}

func TestListDataSources_TemplateCount(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	ds1, err := st.SeedDataSourceIfMissing(ctx, seedDataSource())
	if err != nil {
		t.Fatal(err)
	}
	ds2, err := st.CreateDataSource(ctx, DataSourceContent{
		Name:   "第二个数据源",
		Schema: json.RawMessage(`{"type":"object"}`),
		Data:   json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	// 引用计数:ds1 被两模板引用,ds2 零引用(LEFT JOIN 补 0)
	bound := seedContent()
	bound.DataSourceID = &ds1
	if err := st.SeedTemplateIfEmpty(ctx, bound); err != nil {
		t.Fatal(err)
	}
	second := seedContent()
	second.Name = "第二模板"
	second.DataSourceID = &ds1
	if _, err := st.CreateTemplate(ctx, second); err != nil {
		t.Fatal(err)
	}

	items, err := st.ListDataSources(ctx)
	if err != nil {
		t.Fatalf("列表: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("列表长度 = %d, 期望 2", len(items))
	}
	byID := map[int64]DataSourceSummary{}
	for _, it := range items {
		byID[it.ID] = it
	}
	if byID[ds1].TemplateCount != 2 || byID[ds2].TemplateCount != 0 {
		t.Fatalf("引用计数不符: %+v", items)
	}
	if byID[ds2].Name != "第二个数据源" {
		t.Fatalf("摘要字段不符: %+v", byID[ds2])
	}
}

// --- 渲染记录(13 票,spec §2.4 #7)---

func TestCreateRenderRecord_StubRow(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.SeedTemplateIfEmpty(ctx, seedContent()); err != nil {
		t.Fatalf("播种: %v", err)
	}

	rec, err := st.CreateRenderRecord(ctx, 1, nil)
	if err != nil {
		t.Fatalf("创建渲染记录: %v", err)
	}
	if rec.ID <= 0 || rec.TemplateID != 1 {
		t.Fatalf("id/templateId = %d/%d", rec.ID, rec.TemplateID)
	}
	if rec.Images == nil || len(rec.Images) != 0 {
		t.Fatalf("stub 记录 images 应为空数组: %v", rec.Images)
	}
	if !strings.HasSuffix(rec.CreatedAt, "Z") {
		t.Fatalf("时间戳应为 RFC3339 UTC: %q", rec.CreatedAt)
	}
}

func TestUpdateRenderImages_Roundtrip(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	rec, err := st.CreateRenderRecord(ctx, 1, nil)
	if err != nil {
		t.Fatalf("创建渲染记录: %v", err)
	}

	images := []RenderImage{
		{Frame: 0, Name: "主页", Path: "renders/1/1.png"},
		{Frame: 1, Name: "续页", Path: "renders/1/2.png"},
	}
	if err := st.UpdateRenderImages(ctx, rec.ID, images); err != nil {
		t.Fatalf("回填 images: %v", err)
	}

	// DB 存相对 path(spec §2.4 #7),JSON 形状 [{frame,name,path}]
	var raw []map[string]any
	row := ""
	if err := st.db.QueryRow(`SELECT images FROM renders WHERE id = ?`, rec.ID).Scan(&row); err != nil {
		t.Fatalf("读 images 列: %v", err)
	}
	if err := json.Unmarshal([]byte(row), &raw); err != nil {
		t.Fatalf("images 列非 JSON 数组: %s (%v)", row, err)
	}
	if len(raw) != 2 || raw[0]["path"] != "renders/1/1.png" || raw[1]["frame"] != float64(1) {
		t.Fatalf("images 列内容不符: %s", row)
	}
}

func TestDeleteRenderRecord_RemovesStub(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	rec, err := st.CreateRenderRecord(ctx, 1, nil)
	if err != nil {
		t.Fatalf("创建渲染记录: %v", err)
	}
	if err := st.DeleteRenderRecord(ctx, rec.ID); err != nil {
		t.Fatalf("删除 stub: %v", err)
	}
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM renders`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("删除后 renders 行数 = %d (%v), 期望 0", n, err)
	}
}

// --- DeleteTemplate(卡片操作修订,spec §2.4 #10) ---

// TestDeleteTemplate_CascadesRenderRows 删除模板:行级联清其渲染记录行(其余
// 模板的渲染记录不受牵连);数据源实体不受影响(独立资源);再删同 id → ErrNotFound
func TestDeleteTemplate_CascadesRenderRows(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	if _, err := st.SeedDataSourceIfMissing(ctx, seedDataSource()); err != nil {
		t.Fatalf("播种数据源: %v", err)
	}
	idA, err := st.CreateTemplate(ctx, seedContent())
	if err != nil {
		t.Fatalf("创建模板 A: %v", err)
	}
	idB, err := st.CreateTemplate(ctx, seedContent())
	if err != nil {
		t.Fatalf("创建模板 B: %v", err)
	}
	recA, err := st.CreateRenderRecord(ctx, idA, nil)
	if err != nil {
		t.Fatalf("创建渲染记录 A: %v", err)
	}
	if _, err := st.CreateRenderRecord(ctx, idB, nil); err != nil {
		t.Fatalf("创建渲染记录 B: %v", err)
	}
	if err := st.UpdateRenderImages(ctx, recA.ID, []RenderImage{{Frame: 0, Name: "主页", Path: "renders/1/1.png"}}); err != nil {
		t.Fatalf("回填渲染记录 A: %v", err)
	}

	if err := st.DeleteTemplate(ctx, idA); err != nil {
		t.Fatalf("删除模板 A: %v", err)
	}

	if _, err := st.GetTemplate(ctx, idA); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetTemplate(已删) err = %v, 期望 ErrNotFound", err)
	}
	var nA int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM renders WHERE template_id = ?`, idA).Scan(&nA); err != nil || nA != 0 {
		t.Fatalf("模板 A 渲染记录行 = %d (%v), 期望级联清 0", nA, err)
	}
	var nB int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM renders WHERE template_id = ?`, idB).Scan(&nB); err != nil || nB != 1 {
		t.Fatalf("模板 B 渲染记录行 = %d (%v), 期望不受牵连为 1", nB, err)
	}
	sources, err := st.ListDataSources(ctx)
	if err != nil || len(sources) != 1 || sources[0].TemplateCount != 0 {
		t.Fatalf("数据源应不受影响(TemplateCount 回落 0): %v %v", sources, err)
	}

	// 再删同 id:寻址失败 → ErrNotFound(事务回滚,级联面一并撤销)
	if err := st.DeleteTemplate(ctx, idA); !errors.Is(err, ErrNotFound) {
		t.Fatalf("再删已删模板 err = %v, 期望 ErrNotFound", err)
	}
}

// TestDeleteDataSource_GuardsReferences 删除数据源(28 票,spec §2.4 #6e):
// 零引用删成功且行消失;被模板引用 → ErrDataSourceInUse 且实体原样在
// (引用不悬空,先解绑再删);不存在 → ErrDataSourceNotFound
func TestDeleteDataSource_GuardsReferences(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	// 零引用:删除成功且行消失
	dsID, err := st.CreateDataSource(ctx, seedDataSource())
	if err != nil {
		t.Fatalf("创建数据源: %v", err)
	}
	if err := st.DeleteDataSource(ctx, dsID); err != nil {
		t.Fatalf("删除零引用数据源: %v", err)
	}
	if _, err := st.GetDataSource(ctx, dsID); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatalf("已删数据源期望 ErrDataSourceNotFound, 实得 %v", err)
	}

	// 被引用:拒绝且实体不动
	dsID, err = st.CreateDataSource(ctx, seedDataSource())
	if err != nil {
		t.Fatalf("重建数据源: %v", err)
	}
	bound := seedContent()
	bound.DataSourceID = &dsID
	if _, err := st.CreateTemplate(ctx, bound); err != nil {
		t.Fatalf("创建引用模板: %v", err)
	}
	if err := st.DeleteDataSource(ctx, dsID); !errors.Is(err, ErrDataSourceInUse) {
		t.Fatalf("被引用删除 err = %v, 期望 ErrDataSourceInUse", err)
	}
	if _, err := st.GetDataSource(ctx, dsID); err != nil {
		t.Fatalf("被拒删除后实体应原样在: %v", err)
	}

	// 不存在 → ErrDataSourceNotFound
	if err := st.DeleteDataSource(ctx, 999); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatalf("删除不存在 err = %v, 期望 ErrDataSourceNotFound", err)
	}
}

// TestListRenderRecords_OrderLimitIsolation 渲染历史列表:id 降序(最新在前)、
// limit 截断取最新、模板间隔离、version 透传(draft 行 nil)、images 原样
// (DB 相对 path 形态,响应侧转换归 api 层)
func TestListRenderRecords_OrderLimitIsolation(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	idA, err := st.CreateTemplate(ctx, seedContent())
	if err != nil {
		t.Fatalf("创建模板 A: %v", err)
	}
	other := seedContent()
	other.Name = "另一模板"
	idB, err := st.CreateTemplate(ctx, other)
	if err != nil {
		t.Fatalf("创建模板 B: %v", err)
	}

	// A 三条:草稿(nil)→ v1 → v2,id 自增即时间序
	rDraft, err := st.CreateRenderRecord(ctx, idA, nil)
	if err != nil {
		t.Fatalf("造 draft 渲染记录: %v", err)
	}
	v1 := int64(1)
	rV1, err := st.CreateRenderRecord(ctx, idA, &v1)
	if err != nil {
		t.Fatalf("造 v1 渲染记录: %v", err)
	}
	v2 := int64(2)
	rV2, err := st.CreateRenderRecord(ctx, idA, &v2)
	if err != nil {
		t.Fatalf("造 v2 渲染记录: %v", err)
	}
	// draft 行回填 images(其余 stub 空数组),断言 images 原样透出
	draftImages := []RenderImage{{Frame: 0, Name: "主页", Path: "renders/" + strconv.FormatInt(rDraft.ID, 10) + "/1.png"}}
	if err := st.UpdateRenderImages(ctx, rDraft.ID, draftImages); err != nil {
		t.Fatalf("回填 draft images: %v", err)
	}
	// B 一条(隔离对照)
	if _, err := st.CreateRenderRecord(ctx, idB, nil); err != nil {
		t.Fatalf("造模板 B 渲染记录: %v", err)
	}

	recs, err := st.ListRenderRecords(ctx, idA, 20)
	if err != nil {
		t.Fatalf("列渲染记录: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("模板 A 应 3 条记录, 实得 %d: %+v", len(recs), recs)
	}
	wantOrder := []int64{rV2.ID, rV1.ID, rDraft.ID}
	for i, rec := range recs {
		if rec.ID != wantOrder[i] {
			t.Fatalf("顺序不符: recs[%d].id = %d, 期望 %d(id 降序)", i, rec.ID, wantOrder[i])
		}
		if rec.TemplateID != idA {
			t.Fatalf("recs[%d].TemplateID = %d, 期望 %d", i, rec.TemplateID, idA)
		}
	}
	if recs[0].Version == nil || *recs[0].Version != 2 {
		t.Fatalf("最新记录应为 v2: %+v", recs[0].Version)
	}
	if recs[1].Version == nil || *recs[1].Version != 1 {
		t.Fatalf("次新记录应为 v1: %+v", recs[1].Version)
	}
	if recs[2].Version != nil {
		t.Fatalf("draft 记录 version 应 nil 透传: %+v", recs[2].Version)
	}
	if len(recs[2].Images) != 1 || recs[2].Images[0].Path != draftImages[0].Path {
		t.Fatalf("images 应原样透出(DB 相对 path): %+v", recs[2].Images)
	}
	if recs[0].CreatedAt == "" {
		t.Fatalf("createdAt 应非空: %+v", recs[0])
	}

	// limit 截断:取最新 2 条
	limited, err := st.ListRenderRecords(ctx, idA, 2)
	if err != nil {
		t.Fatalf("limit 列渲染记录: %v", err)
	}
	if len(limited) != 2 || limited[0].ID != rV2.ID || limited[1].ID != rV1.ID {
		t.Fatalf("limit=2 应取最新两条 [v2 v1]: %+v", limited)
	}

	// 模板隔离 + 无记录模板回空数组(非 nil)
	onlyB, err := st.ListRenderRecords(ctx, idB, 20)
	if err != nil {
		t.Fatalf("列模板 B 渲染记录: %v", err)
	}
	if len(onlyB) != 1 {
		t.Fatalf("模板 B 应恰 1 条, 实得 %d", len(onlyB))
	}
	empty, err := st.ListRenderRecords(ctx, 999, 20)
	if err != nil {
		t.Fatalf("列不存在模板渲染记录: %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("无记录应回空数组(非 nil): %+v", empty)
	}
}
