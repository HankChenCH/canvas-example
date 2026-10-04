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

// seedContent 最小合法模板内容(两帧 + 流链 + 数据源,形状对齐 spec §2.3)
func seedContent() TemplateContent {
	return TemplateContent{
		Name: "结业证书 · 批量打印页",
		Canvases: []CanvasEntry{
			{Name: "主页", Graph: json.RawMessage(`{"canvas":{"width":794,"height":1123},"layers":[]}`)},
			{Name: "续页", Graph: json.RawMessage(`{"canvas":{"width":794,"height":1123},"layers":[]}`)},
		},
		FlowChain:     json.RawMessage(`[{"frame":0,"mode":"fixed"},{"frame":1,"mode":"paged","omitIfEmpty":true}]`),
		DatasetSchema: json.RawMessage(`{"type":"object"}`),
		Dataset:       json.RawMessage(`{"org":{"name":"瀚辰培训中心"}}`),
	}
}

func countTemplates(t *testing.T, st *Store) int {
	t.Helper()
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM templates`).Scan(&n); err != nil {
		t.Fatalf("数模板行: %v", err)
	}
	return n
}

func TestOpenCreatesTables(t *testing.T) {
	st := openTestStore(t)
	for _, table := range []string{"templates", "renders"} {
		var name string
		err := st.db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name = ?`, table,
		).Scan(&name)
		if err != nil {
			t.Fatalf("表 %s 未建: %v", table, err)
		}
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
	if got := countTemplates(t, st); got != 1 {
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
	if got := countTemplates(t, st); got != 1 {
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
	if err := st.SeedTemplateIfEmpty(ctx, seedContent()); err != nil {
		t.Fatalf("播种: %v", err)
	}
	rec, err := st.GetTemplate(ctx, 1)
	if err != nil {
		t.Fatalf("取模板: %v", err)
	}
	if rec.ID != 1 || rec.Name != "结业证书 · 批量打印页" {
		t.Fatalf("id/name = %d/%q", rec.ID, rec.Name)
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
	content.DatasetSchema = nil
	content.Dataset = nil
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
	for _, key := range []string{`"flowChain":null`, `"datasetSchema":null`, `"dataset":null`} {
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

	// POST 载荷不含 dataset(spec §2.4 #3)——创建内容两数据列留空
	content := seedContent()
	content.Dataset = nil
	content.DatasetSchema = nil
	id1, err := st.CreateTemplate(ctx, content)
	if err != nil {
		t.Fatalf("创建模板 1: %v", err)
	}
	id2, err := st.CreateTemplate(ctx, content)
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
	// POST 载荷不含 dataset(spec §2.4 #3):落库即 null,createdAt = updatedAt
	if rec.Dataset != nil || rec.DatasetSchema != nil {
		t.Fatalf("未带数据源的创建应落 NULL: %v / %v", rec.Dataset, rec.DatasetSchema)
	}
	if rec.CreatedAt == "" || rec.CreatedAt != rec.UpdatedAt {
		t.Fatalf("createdAt 应等于 updatedAt,实得 %q / %q", rec.CreatedAt, rec.UpdatedAt)
	}
	if !strings.HasSuffix(rec.UpdatedAt, "Z") {
		t.Fatalf("时间戳应为 RFC3339 UTC: %q", rec.UpdatedAt)
	}
}

// TestUpdateTemplateContent_PreservesDataset 整存替换只动 name/canvases/flowChain,
// 不触碰 dataset/datasetSchema(spec §2.4 #5——12 票票面「先 PUT dataset 再 PUT
// 模板验证仍在」的存储层口径)
func TestUpdateTemplateContent_PreservesDataset(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.SeedTemplateIfEmpty(ctx, seedContent()); err != nil {
		t.Fatalf("播种: %v", err)
	}

	if err := st.UpdateDataset(ctx, 1,
		json.RawMessage(`{"type":"object"}`), json.RawMessage(`{"kept":true}`)); err != nil {
		t.Fatalf("先 PUT dataset: %v", err)
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
	if rec.DatasetSchema == nil || string(rec.DatasetSchema) != `{"type":"object"}` {
		t.Fatalf("datasetSchema 不得被触碰: %v", rec.DatasetSchema)
	}
	if rec.Dataset == nil || string(rec.Dataset) != `{"kept":true}` {
		t.Fatalf("dataset 不得被触碰: %v", rec.Dataset)
	}
}

func TestUpdateTemplateContent_NotFound(t *testing.T) {
	st := openTestStore(t)
	if err := st.UpdateTemplateContent(context.Background(), 999, seedContent()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("期望 ErrNotFound, 实得 %v", err)
	}
}

func TestUpdateDataset_ReplacesBothColumns(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.SeedTemplateIfEmpty(ctx, seedContent()); err != nil {
		t.Fatalf("播种: %v", err)
	}

	if err := st.UpdateDataset(ctx, 1,
		json.RawMessage(`{"type":"array"}`), json.RawMessage(`[1,2]`)); err != nil {
		t.Fatalf("更新数据源: %v", err)
	}

	rec, err := st.GetTemplate(ctx, 1)
	if err != nil {
		t.Fatalf("回读: %v", err)
	}
	if string(rec.DatasetSchema) != `{"type":"array"}` || string(rec.Dataset) != `[1,2]` {
		t.Fatalf("两列应整存替换: %s / %s", rec.DatasetSchema, rec.Dataset)
	}
	// 未触碰面:模板内容原样
	if rec.Name != seedContent().Name || len(rec.Canvases) != 2 {
		t.Fatalf("模板内容不得被触碰: %+v", rec)
	}
}

func TestUpdateDataset_NotFound(t *testing.T) {
	st := openTestStore(t)
	err := st.UpdateDataset(context.Background(), 999, json.RawMessage(`{}`), json.RawMessage(`{}`))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("期望 ErrNotFound, 实得 %v", err)
	}
}
