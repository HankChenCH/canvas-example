package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example/server/internal/pages"
)

// --- 模板草稿-发布-版本化(工单 01/02,spec §2/§3):template_versions 表、
// 存量回填 v1、版本读写(INSERT-only,事务内 MAX+1)、删模板级联 ---

// TestOpenCreatesTemplateVersionsTable 新表随启动 DDL 建立(CREATE TABLE IF NOT
// EXISTS 惯例,不走 ensure-column);UNIQUE(template_id, version) 兜底在位
func TestOpenCreatesTemplateVersionsTable(t *testing.T) {
	st := openTestStore(t)
	var sql string
	err := st.db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type='table' AND name = 'template_versions'`,
	).Scan(&sql)
	if err != nil {
		t.Fatalf("template_versions 表未建: %v", err)
	}
	for _, frag := range []string{"UNIQUE", "template_id", "version"} {
		if !strings.Contains(sql, frag) {
			t.Fatalf("建表 DDL 应含 %s: %s", frag, sql)
		}
	}
}

// TestBackfillTemplateVersionsV1 存量回填:凡零版本行的模板,现 pages 列文本
// 原样固化为 v1(不重新 marshal,字节面诚实);created_at = 回填运行时刻;
// 谓词幂等,重跑 no-op 且 created_at 不变
func TestBackfillTemplateVersionsV1(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	idA, err := st.CreateTemplate(ctx, seedContent())
	if err != nil {
		t.Fatalf("创建模板 A: %v", err)
	}
	flowless := seedContent()
	flowless.Name = "无 flow 页模板"
	flowless.Pages = []pages.Page{
		seedPage("pg_a", "甲页", `{"canvas":{"width":794,"height":1123},"layers":[]}`, ""),
	}
	if _, err := st.CreateTemplate(ctx, flowless); err != nil {
		t.Fatalf("创建模板 B: %v", err)
	}

	n, err := st.BackfillTemplateVersionsV1(ctx)
	if err != nil {
		t.Fatalf("回填: %v", err)
	}
	if n != 2 {
		t.Fatalf("回填模板数 = %d, 期望 2", n)
	}

	// 回填是列文本原样固化:与 templates 列字节面一致(不重新 marshal)
	var rawPages string
	if err := st.db.QueryRow(`SELECT pages FROM templates WHERE id = ?`, idA).
		Scan(&rawPages); err != nil {
		t.Fatalf("读模板 A 列: %v", err)
	}
	var vPages, vCreatedAt string
	if err := st.db.QueryRow(
		`SELECT pages, created_at FROM template_versions WHERE template_id = ? AND version = 1`, idA,
	).Scan(&vPages, &vCreatedAt); err != nil {
		t.Fatalf("读版本行: %v", err)
	}
	if vPages != rawPages {
		t.Fatalf("回填应原样固化列文本:\n pages %q vs %q", vPages, rawPages)
	}
	if !strings.HasSuffix(vCreatedAt, "Z") || strings.Contains(vCreatedAt, "+") {
		t.Fatalf("created_at 应为 RFC3339 UTC: %q", vCreatedAt)
	}
	if _, err := time.Parse(time.RFC3339, vCreatedAt); err != nil {
		t.Fatalf("created_at 应可按 RFC3339 解析: %q (%v)", vCreatedAt, err)
	}

	// 谓词幂等:重跑 no-op,created_at 不变
	createdBefore := vCreatedAt
	n2, err := st.BackfillTemplateVersionsV1(ctx)
	if err != nil {
		t.Fatalf("重跑回填: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("重跑回填应 no-op,实得 %d", n2)
	}
	var createdAfter string
	if err := st.db.QueryRow(
		`SELECT created_at FROM template_versions WHERE template_id = ? AND version = 1`, idA,
	).Scan(&createdAfter); err != nil {
		t.Fatalf("回读版本行: %v", err)
	}
	if createdAfter != createdBefore {
		t.Fatalf("重跑回填不得改写 created_at: %q vs %q", createdAfter, createdBefore)
	}

	// 回填后草稿与 v1 字节相同 → duplicate 落 v1;改草稿后发布 → v2(v1 已占位,
	// MAX+1 从回填结果上续起)
	version, _, duplicate, err := st.PublishTemplateVersion(ctx, idA)
	if err != nil || !duplicate || version != 1 {
		t.Fatalf("回填后未改草稿的发布应 duplicate v1: (%d,%v,%v)", version, duplicate, err)
	}
	changed := seedContent()
	changed.Pages = []pages.Page{
		seedPage("pg_new", "回填后续发页", `{"canvas":{"width":794,"height":1123},"layers":[]}`, `{"mode":"fixed"}`),
	}
	if err := st.UpdateTemplateContent(ctx, idA, changed); err != nil {
		t.Fatalf("改草稿: %v", err)
	}
	version, _, duplicate, err = st.PublishTemplateVersion(ctx, idA)
	if err != nil {
		t.Fatalf("回填后发布: %v", err)
	}
	if duplicate || version != 2 {
		t.Fatalf("回填后发布应产生 v2: version=%d duplicate=%v", version, duplicate)
	}
}

// TestPublishTemplateVersion_MAXPlus1AndDuplicate 发布 = 事务内 MAX(version)+1;
// 草稿 pages 列文本与最新版本行字节相等 → duplicate 幂等跳过(createdAt 沿用
// 既有版本行);INSERT-only:版本行永无 UPDATE
func TestPublishTemplateVersion_MAXPlus1AndDuplicate(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	id, err := st.CreateTemplate(ctx, seedContent())
	if err != nil {
		t.Fatalf("创建模板: %v", err)
	}

	v1, c1, dup, err := st.PublishTemplateVersion(ctx, id)
	if err != nil || v1 != 1 || dup {
		t.Fatalf("首发 = (%d,%v,%v), 期望 (1,false,nil): %v", v1, dup, err, err)
	}
	if c1 == "" || !strings.HasSuffix(c1, "Z") {
		t.Fatalf("createdAt 应为 RFC3339 UTC: %q", c1)
	}

	// 草稿未动,重复发布:字节相等 → duplicate,版本号与 createdAt 不变
	v2, c2, dup2, err := st.PublishTemplateVersion(ctx, id)
	if err != nil || !dup2 || v2 != 1 || c2 != c1 {
		t.Fatalf("重复发布 = (%d,%q,%v), 期望 (1,%q,true): %v", v2, c2, dup2, c1, err)
	}

	// 改草稿(仅 pages 文本变化)再发布 → v2(快照 = pages 列整体,页 id 在内,
	// name 不在快照内;createdAt 形状断言,秒级解析度下不比新旧)
	changed := seedContent()
	changed.Pages = []pages.Page{
		seedPage("pg_main2", "主页改", `{"canvas":{"width":794,"height":1123},"layers":[]}`, `{"mode":"fixed"}`),
	}
	if err := st.UpdateTemplateContent(ctx, id, changed); err != nil {
		t.Fatalf("改草稿: %v", err)
	}
	v3, c3, dup3, err := st.PublishTemplateVersion(ctx, id)
	if err != nil || dup3 || v3 != 2 {
		t.Fatalf("改动后发布 = (%d,%v,%v), 期望 (2,false,nil): %v", v3, dup3, err, err)
	}
	if !strings.HasSuffix(c3, "Z") {
		t.Fatalf("新版本 createdAt 应为 RFC3339 UTC: %q", c3)
	}

	// 版本行数 = 2(INSERT-only,无 UPDATE 痕迹)
	if n := countRows(t, st, "template_versions"); n != 2 {
		t.Fatalf("版本行数 = %d, 期望 2", n)
	}
}

// TestPublishTemplateVersion_ByteComparison 幂等判定是字节比较而非 JSON 语义比较:
// 草稿列文本与最新版本行文本字节不同(哪怕语义等价)即造新版本
func TestPublishTemplateVersion_ByteComparison(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	id, err := st.CreateTemplate(ctx, seedContent())
	if err != nil {
		t.Fatalf("创建模板: %v", err)
	}
	if _, _, _, err := st.PublishTemplateVersion(ctx, id); err != nil {
		t.Fatalf("首发: %v", err)
	}
	// 绕过写端点直改列文本(键序重排,JSON 语义等价、字节面不同)
	if _, err := st.db.Exec(`UPDATE templates SET pages = ? WHERE id = ?`,
		`[{"flow":{"mode":"fixed"},"graph":{"canvas":{"height":1123,"width":794},"layers":[]},"id":"pg_main","name":"主页"},{"flow":{"mode":"paged","omitIfEmpty":true},"graph":{"canvas":{"height":1123,"width":794},"layers":[]},"id":"pg_cont","name":"续页"}]`,
		id); err != nil {
		t.Fatalf("直改草稿列: %v", err)
	}
	version, _, duplicate, err := st.PublishTemplateVersion(ctx, id)
	if err != nil {
		t.Fatalf("字节不同文本再发布: %v", err)
	}
	if duplicate || version != 2 {
		t.Fatalf("字节比较应判不等并造 v2: version=%d duplicate=%v", version, duplicate)
	}
}

// TestPublishTemplateVersion_PagesSnapshotScope 快照对象 = pages 列文本整体:
// 只改模板 name 会被 duplicate 幂等吸收(name 不在快照内);只改页 flow(其余
// 全同)即判不等造新版本
func TestPublishTemplateVersion_PagesSnapshotScope(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	id, err := st.CreateTemplate(ctx, seedContent())
	if err != nil {
		t.Fatalf("创建模板: %v", err)
	}
	if _, _, _, err := st.PublishTemplateVersion(ctx, id); err != nil {
		t.Fatalf("首发: %v", err)
	}

	// 只改 name:pages 文本不变 → duplicate
	renamed := seedContent()
	renamed.Name = "只改名"
	if err := st.UpdateTemplateContent(ctx, id, renamed); err != nil {
		t.Fatalf("只改名: %v", err)
	}
	if version, _, dup, err := st.PublishTemplateVersion(ctx, id); err != nil || !dup || version != 1 {
		t.Fatalf("name 不在快照内应 duplicate v1: (%d,%v,%v)", version, dup, err)
	}

	// 只改页 0 的 flow:pages 文本变 → v2
	reflowed := seedContent()
	reflowed.Pages[0].Flow = json.RawMessage(`{"mode":"fixed","quota":2}`)
	if err := st.UpdateTemplateContent(ctx, id, reflowed); err != nil {
		t.Fatalf("改页 flow: %v", err)
	}
	version, _, dup, err := st.PublishTemplateVersion(ctx, id)
	if err != nil || dup || version != 2 {
		t.Fatalf("页 flow 变化应造 v2: (%d,%v,%v)", version, dup, err)
	}
}

func TestPublishTemplateVersion_TemplateNotFound(t *testing.T) {
	st := openTestStore(t)
	if _, _, _, err := st.PublishTemplateVersion(context.Background(), 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("期望 ErrNotFound, 实得 %v", err)
	}
}

// TestListTemplateVersions_DescAndEmpty 版本列表 version 降序(最新在前);
// 无版本 = 空数组;模板不存在 → ErrNotFound(与 GetTemplate 惯例一致)
func TestListTemplateVersions_DescAndEmpty(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	id, err := st.CreateTemplate(ctx, seedContent())
	if err != nil {
		t.Fatalf("创建模板: %v", err)
	}

	items, err := st.ListTemplateVersions(ctx, id)
	if err != nil || len(items) != 0 || items == nil {
		t.Fatalf("未发布模板版本列表 = %v (%v), 期望空数组", items, err)
	}

	// 三个版本:每次发布前改 pages(快照 = pages 列文本,只改 name 会被
	// duplicate 幂等吸收)
	for i := range 3 {
		changed := seedContent()
		changed.Pages = []pages.Page{
			seedPage("pg_v"+string(rune('A'+i)), "主页改"+string(rune('A'+i)), `{"canvas":{"width":794,"height":1123},"layers":[]}`, `{"mode":"fixed"}`),
		}
		if err := st.UpdateTemplateContent(ctx, id, changed); err != nil {
			t.Fatalf("改草稿 %d: %v", i, err)
		}
		if _, _, _, err := st.PublishTemplateVersion(ctx, id); err != nil {
			t.Fatalf("发布 %d: %v", i, err)
		}
	}
	items, err = st.ListTemplateVersions(ctx, id)
	if err != nil {
		t.Fatalf("列表: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("列表长度 = %d, 期望 3", len(items))
	}
	for i, want := range []int64{3, 2, 1} {
		if items[i].Version != want {
			t.Fatalf("列表[%d].Version = %d, 期望 %d(降序)", i, items[i].Version, want)
		}
		if items[i].CreatedAt == "" {
			t.Fatalf("列表[%d].createdAt 应非空", i)
		}
	}

	// JSON 形状:轻量两字段(不吐 pages)
	b, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("序列化: %v", err)
	}
	if !strings.Contains(string(b), `"version"`) || strings.Contains(string(b), `"pages"`) {
		t.Fatalf("列表应只含 version/createdAt 轻量字段: %s", b)
	}

	if _, err := st.ListTemplateVersions(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("模板不存在期望 ErrNotFound, 实得 %v", err)
	}
}

// TestGetTemplateVersion_Roundtrip 单版本全量:pages 文本原样(页 id 在内);
// 版本不存在 → ErrTemplateVersionNotFound(含对无版本模板指定)
func TestGetTemplateVersion_Roundtrip(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	id, err := st.CreateTemplate(ctx, seedContent())
	if err != nil {
		t.Fatalf("创建模板: %v", err)
	}
	v1, _, _, err := st.PublishTemplateVersion(ctx, id)
	if err != nil || v1 != 1 {
		t.Fatalf("首发: %v", err)
	}

	rec, err := st.GetTemplateVersion(ctx, id, 1)
	if err != nil {
		t.Fatalf("取版本 1: %v", err)
	}
	if rec.Version != 1 || rec.CreatedAt == "" {
		t.Fatalf("版本行元数据不符: %+v", rec)
	}
	// 快照随 pages 整体:页 id 与 flow 在快照内
	if len(rec.Pages) != 2 {
		t.Fatalf("版本 pages 应两页: %+v", rec.Pages)
	}
	var id0 string
	if err := json.Unmarshal(rec.Pages[0].ID, &id0); err != nil || id0 != "pg_main" {
		t.Fatalf("快照应含页 id pg_main: %s (%v)", rec.Pages[0].ID, err)
	}
	if string(rec.Pages[1].Flow) != `{"mode":"paged","omitIfEmpty":true}` {
		t.Fatalf("快照应含页 flow 原文: %s", rec.Pages[1].Flow)
	}

	// 无 flow 页模板:快照 pages 与草稿一致(flow 键缺席)
	flowless := seedContent()
	flowless.Pages = []pages.Page{seedPage("pg_a", "甲页", `{"canvas":{"width":794,"height":1123},"layers":[]}`, "")}
	id2, err := st.CreateTemplate(ctx, flowless)
	if err != nil {
		t.Fatalf("创建无 flow 页模板: %v", err)
	}
	if _, _, _, err := st.PublishTemplateVersion(ctx, id2); err != nil {
		t.Fatalf("无 flow 页模板发布: %v", err)
	}
	rec2, err := st.GetTemplateVersion(ctx, id2, 1)
	if err != nil || len(rec2.Pages) != 1 || rec2.Pages[0].Flow != nil {
		t.Fatalf("无 flow 页快照 flow 应为键缺席: %+v (%v)", rec2.Pages, err)
	}

	// 版本不存在(含对无版本模板指定)→ ErrTemplateVersionNotFound
	if _, err := st.GetTemplateVersion(ctx, id, 99); !errors.Is(err, ErrTemplateVersionNotFound) {
		t.Fatalf("版本不存在期望 ErrTemplateVersionNotFound, 实得 %v", err)
	}
	if _, err := st.GetTemplateVersion(ctx, id2, 2); !errors.Is(err, ErrTemplateVersionNotFound) {
		t.Fatalf("无版本模板指定版本期望 ErrTemplateVersionNotFound, 实得 %v", err)
	}
}

// TestLatestTemplateVersion 最新版本 = ORDER BY version DESC LIMIT 1;无版本行
// ok=false 不报错(供渲染默认语义区分 409/404)
func TestLatestTemplateVersion(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	id, err := st.CreateTemplate(ctx, seedContent())
	if err != nil {
		t.Fatalf("创建模板: %v", err)
	}
	version, ok, err := st.LatestTemplateVersion(ctx, id)
	if err != nil || ok || version != 0 {
		t.Fatalf("无版本行 = (%d,%v,%v), 期望 (0,false,nil)", version, ok, err)
	}
	for i := range 2 {
		changed := seedContent()
		changed.Pages = []pages.Page{
			seedPage("pg_v"+string(rune('A'+i)), "主页改"+string(rune('A'+i)), `{"canvas":{"width":794,"height":1123},"layers":[]}`, `{"mode":"fixed"}`),
		}
		if err := st.UpdateTemplateContent(ctx, id, changed); err != nil {
			t.Fatalf("改草稿 %d: %v", i, err)
		}
		if _, _, _, err := st.PublishTemplateVersion(ctx, id); err != nil {
			t.Fatalf("发布 %d: %v", i, err)
		}
	}
	version, ok, err = st.LatestTemplateVersion(ctx, id)
	if err != nil || !ok || version != 2 {
		t.Fatalf("最新版本 = (%d,%v,%v), 期望 (2,true,nil)", version, ok, err)
	}
}

// TestRevertTemplateToVersion 回滚 = 单事务原子复制版本行 pages 文本回草稿列:
// pages 整体替换、updated_at 刷新(与「发布不动」对照),name/data_source_id/
// editor_state 都不碰;版本线线性,回滚不产生新版本
func TestRevertTemplateToVersion(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	dsID, err := st.SeedDataSourceIfMissing(ctx, seedDataSource())
	if err != nil {
		t.Fatalf("播种数据源: %v", err)
	}
	content := seedContent()
	content.DataSourceID = &dsID
	// 直插控制时间戳:老 updated_at,便于断言回滚刷新
	id, err := st.insertTemplate(ctx, content, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("插模板: %v", err)
	}
	snapshot := json.RawMessage(`{"schemaVersion":1,"frames":{},"prefs":{}}`)
	if err := st.UpdateTemplateEditorState(ctx, id, snapshot); err != nil {
		t.Fatalf("写工程数据: %v", err)
	}
	v1, _, _, err := st.PublishTemplateVersion(ctx, id)
	if err != nil || v1 != 1 {
		t.Fatalf("首发: %v", err)
	}

	// 草稿大改 + 再发布 v2(版本史留痕)
	rewritten := seedContent()
	rewritten.Pages = []pages.Page{seedPage("pg_rw", "重写页", `{"canvas":{"width":10,"height":10},"layers":[]}`, "")}
	if err := st.UpdateTemplateContent(ctx, id, rewritten); err != nil {
		t.Fatalf("重写草稿: %v", err)
	}
	if _, _, _, err := st.PublishTemplateVersion(ctx, id); err != nil {
		t.Fatalf("发 v2: %v", err)
	}

	if err := st.RevertTemplateToVersion(ctx, id, 1); err != nil {
		t.Fatalf("回滚 v1: %v", err)
	}
	rec, err := st.GetTemplate(ctx, id)
	if err != nil {
		t.Fatalf("回读: %v", err)
	}
	// pages 回到 v1 内容:两页、页名、flow 原样
	if len(rec.Pages) != 2 {
		t.Fatalf("pages 应回到 v1 内容: %+v", rec.Pages)
	}
	var name0 string
	if err := json.Unmarshal(rec.Pages[0].Name, &name0); err != nil || name0 != "主页" {
		t.Fatalf("页 0 应回到 v1(主页): %s", rec.Pages[0].Name)
	}
	if !strings.Contains(string(rec.Pages[1].Flow), `"mode":"paged"`) {
		t.Fatalf("页 1 flow 应回到 v1 内容: %s", rec.Pages[1].Flow)
	}
	if rec.UpdatedAt == "2026-01-01T00:00:00Z" {
		t.Fatalf("回滚应刷新 updated_at(与发布不动对照): %q", rec.UpdatedAt)
	}
	if !strings.HasSuffix(rec.UpdatedAt, "Z") {
		t.Fatalf("updated_at 应为 RFC3339 UTC: %q", rec.UpdatedAt)
	}
	if rec.Name != content.Name {
		t.Fatalf("回滚不碰 name: %q", rec.Name)
	}
	if rec.DataSourceID == nil || *rec.DataSourceID != dsID {
		t.Fatalf("回滚不碰 data_source_id: %v", rec.DataSourceID)
	}
	if string(rec.EditorState) != string(snapshot) {
		t.Fatalf("回滚不碰 editor_state: %s", rec.EditorState)
	}
	// 版本线线性:回滚不产生新版本
	items, err := st.ListTemplateVersions(ctx, id)
	if err != nil || len(items) != 2 || items[0].Version != 2 {
		t.Fatalf("回滚不得产生新版本: %v (%v)", items, err)
	}
}

// TestRevertTemplateToVersion_NotFound 模板不存在 → ErrNotFound;版本不存在
// → ErrTemplateVersionNotFound(回滚,草稿列不被部分改写)
func TestRevertTemplateToVersion_NotFound(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	id, err := st.CreateTemplate(ctx, seedContent())
	if err != nil {
		t.Fatalf("创建模板: %v", err)
	}
	if err := st.RevertTemplateToVersion(ctx, 999, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("模板不存在期望 ErrNotFound, 实得 %v", err)
	}
	if err := st.RevertTemplateToVersion(ctx, id, 7); !errors.Is(err, ErrTemplateVersionNotFound) {
		t.Fatalf("版本不存在期望 ErrTemplateVersionNotFound, 实得 %v", err)
	}
	// 失败回滚后草稿原样在
	rec, err := st.GetTemplate(ctx, id)
	if err != nil || len(rec.Pages) != 2 {
		t.Fatalf("失败回滚后草稿应原样: %+v (%v)", rec, err)
	}
}

// TestDeleteTemplate_CascadesVersionRows 删模板事务级联清版本行(renders 先例
// 同款,先删版本行再删模板行);其余模板版本史不受牵连
func TestDeleteTemplate_CascadesVersionRows(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	idA, err := st.CreateTemplate(ctx, seedContent())
	if err != nil {
		t.Fatalf("创建模板 A: %v", err)
	}
	idB, err := st.CreateTemplate(ctx, seedContent())
	if err != nil {
		t.Fatalf("创建模板 B: %v", err)
	}
	for _, id := range []int64{idA, idB} {
		if _, _, _, err := st.PublishTemplateVersion(ctx, id); err != nil {
			t.Fatalf("发布模板 %d: %v", id, err)
		}
	}
	if err := st.DeleteTemplate(ctx, idA); err != nil {
		t.Fatalf("删除模板 A: %v", err)
	}
	var nA int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM template_versions WHERE template_id = ?`, idA).Scan(&nA); err != nil || nA != 0 {
		t.Fatalf("模板 A 版本行 = %d (%v), 期望级联清 0", nA, err)
	}
	var nB int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM template_versions WHERE template_id = ?`, idB).Scan(&nB); err != nil || nB != 1 {
		t.Fatalf("模板 B 版本行 = %d (%v), 期望不受牵连为 1", nB, err)
	}
}

// --- renders.version 溯源列(工单 04,spec §4 渲染选版) ---

// TestOpen_EnsureRenderVersionColumn renders 表幂等补列:templates 已是 pages
// 形态而 renders 仍为旧形态(缺 version 列)的混合库启动 ALTER TABLE 补上,
// 旧行 NULL = 版本化前渲染(诚实语义,不造假数据);重复启动不重复迁移
func TestOpen_EnsureRenderVersionColumn(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate.db")
	st, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// 手工降级:去掉 version 列(重建 renders 表,塞一行旧行态数据)
	if _, err := st.db.Exec(`DROP TABLE renders;
		CREATE TABLE renders (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			template_id INTEGER NOT NULL,
			images      TEXT NOT NULL,
			created_at  TEXT NOT NULL
		);
		INSERT INTO renders (template_id, images, created_at) VALUES (1, '[]', '2026-01-01T00:00:00Z');`); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("补列迁移打开: %v", err)
	}
	defer st2.Close()
	var hasVersion int
	if err := st2.db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('renders') WHERE name = 'version'`,
	).Scan(&hasVersion); err != nil || hasVersion != 1 {
		t.Fatalf("renders 应补出 version 列: count=%d err=%v", hasVersion, err)
	}
	// 旧行原样保留,version NULL(版本化前渲染)
	var v any
	if err := st2.db.QueryRow(`SELECT version FROM renders WHERE id = 1`).Scan(&v); err != nil || v != nil {
		t.Fatalf("旧行应保留且 version NULL: %v (%v)", v, err)
	}
}

// TestCreateRenderRecord_VersionColumn 渲染记录版本溯源:新行必填渲染所用版本,
// draft 渲染(nil)记 NULL——未对应任何发布版本的诚实语义;JSON 序列化 null
// 透传(无 omitempty)
func TestCreateRenderRecord_VersionColumn(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := int64(2)

	rec, err := st.CreateRenderRecord(ctx, 1, &v)
	if err != nil {
		t.Fatalf("创建带版本渲染记录: %v", err)
	}
	if rec.Version == nil || *rec.Version != 2 {
		t.Fatalf("记录应携带版本 2: %v", rec.Version)
	}
	var stored any
	if err := st.db.QueryRow(`SELECT version FROM renders WHERE id = ?`, rec.ID).Scan(&stored); err != nil || stored != int64(2) {
		t.Fatalf("version 列应存 2: %v (%v)", stored, err)
	}

	draftRec, err := st.CreateRenderRecord(ctx, 1, nil)
	if err != nil {
		t.Fatalf("创建 draft 渲染记录: %v", err)
	}
	if draftRec.Version != nil {
		t.Fatalf("draft 渲染版本应为 nil: %v", draftRec.Version)
	}
	var storedDraft any
	if err := st.db.QueryRow(`SELECT version FROM renders WHERE id = ?`, draftRec.ID).Scan(&storedDraft); err != nil || storedDraft != nil {
		t.Fatalf("draft 渲染 version 列应为 NULL: %v (%v)", storedDraft, err)
	}

	// JSON 序列化:null 透传(响应侧 null = 未对应发布版本)
	b, err := json.Marshal(draftRec)
	if err != nil {
		t.Fatalf("序列化: %v", err)
	}
	if !strings.Contains(string(b), `"version":null`) {
		t.Fatalf("draft 渲染应序列化 \"version\":null(无 omitempty): %s", b)
	}
}
