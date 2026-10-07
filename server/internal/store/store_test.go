package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
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

// seedContent 最小合法模板内容(两帧 + 流链,形状对齐 spec §2.3);绑定由调用方
// 经 DataSourceID 接线(模板只持引用)
func seedContent() TemplateContent {
	return TemplateContent{
		Name: "结业证书 · 批量打印页",
		Canvases: []CanvasEntry{
			{Name: "主页", Graph: json.RawMessage(`{"canvas":{"width":794,"height":1123},"layers":[]}`)},
			{Name: "续页", Graph: json.RawMessage(`{"canvas":{"width":794,"height":1123},"layers":[]}`)},
		},
		FlowChain: json.RawMessage(`[{"frame":0,"mode":"fixed"},{"frame":1,"mode":"paged","omitIfEmpty":true}]`),
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
	for _, table := range []string{"datasources", "templates", "renders"} {
		var name string
		err := st.db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name = ?`, table,
		).Scan(&name)
		if err != nil {
			t.Fatalf("表 %s 未建: %v", table, err)
		}
	}
}

// TestOpen_SelfHealLegacySchema 旧版库(dataset 内嵌模板形态)启动自愈:重建
// templates/renders,demo 运行数据由播种补回
func TestOpen_SelfHealLegacySchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	// 手工造旧版形态:templates 带 dataset/datasetSchema 列、无 data_source_id
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
			dataset_schema TEXT,
			dataset        TEXT,
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
	var hasRef int
	if err := st2.db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('templates') WHERE name = 'data_source_id'`,
	).Scan(&hasRef); err != nil || hasRef != 1 {
		t.Fatalf("templates 应重建为带 data_source_id 引用列: count=%d err=%v", hasRef, err)
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
	if len(rec.Canvases) != 2 || rec.Canvases[0].Name != "主页" || rec.Canvases[1].Name != "续页" {
		t.Fatalf("canvases 形状不符: %+v", rec.Canvases)
	}
	if !strings.Contains(string(rec.Canvases[0].Graph), `"width":794`) {
		t.Fatalf("graph 内容不符: %s", rec.Canvases[0].Graph)
	}
	var chain []map[string]any
	if err := json.Unmarshal(rec.FlowChain, &chain); err != nil || len(chain) != 2 {
		t.Fatalf("flowChain 应为两节点数组: %s (%v)", rec.FlowChain, err)
	}
	if rec.CreatedAt == "" || rec.CreatedAt != rec.UpdatedAt {
		t.Fatalf("播种记录 createdAt = updatedAt,实得 %q / %q", rec.CreatedAt, rec.UpdatedAt)
	}
	// RFC3339 UTC 形态(spec §2.1),如 2026-10-04T12:00:00Z
	if !strings.HasSuffix(rec.UpdatedAt, "Z") || strings.Contains(rec.UpdatedAt, "+") {
		t.Fatalf("时间戳应为 RFC3339 UTC: %q", rec.UpdatedAt)
	}
}

func TestGetTemplate_NullColumnsMarshalJSONNull(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	content := seedContent()
	content.FlowChain = nil
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
	for _, key := range []string{`"flowChain":null`, `"dataSourceId":null`} {
		if !strings.Contains(string(b), key) {
			t.Fatalf("NULL 列应序列化为 JSON null(%s 缺失): %s", key, b)
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

// TestUpdateTemplateContent_PreservesBinding 整存替换只动 name/canvases/flow_chain,
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
		Canvases: []CanvasEntry{
			{Name: "新帧", Graph: json.RawMessage(`{"canvas":{"width":100,"height":100},"layers":[]}`)},
		},
		FlowChain: nil, // 缺省 = null = 空链
	}
	if err := st.UpdateTemplateContent(ctx, 1, updated); err != nil {
		t.Fatalf("更新模板: %v", err)
	}

	rec, err := st.GetTemplate(ctx, 1)
	if err != nil {
		t.Fatalf("回读: %v", err)
	}
	if rec.Name != "改名后的模板" || len(rec.Canvases) != 1 || rec.Canvases[0].Name != "新帧" {
		t.Fatalf("name/canvases 应整存替换: %+v", rec)
	}
	if rec.FlowChain != nil {
		t.Fatalf("flowChain 应替换为 NULL: %s", rec.FlowChain)
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
	if rec.Name != seedContent().Name || len(rec.Canvases) != 2 {
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

	rec, err := st.CreateRenderRecord(ctx, 1)
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
	rec, err := st.CreateRenderRecord(ctx, 1)
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
	rec, err := st.CreateRenderRecord(ctx, 1)
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
	recA, err := st.CreateRenderRecord(ctx, idA)
	if err != nil {
		t.Fatalf("创建渲染记录 A: %v", err)
	}
	if _, err := st.CreateRenderRecord(ctx, idB); err != nil {
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
