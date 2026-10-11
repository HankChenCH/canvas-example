package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
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

// TestBackfillTemplateVersionsV1 存量回填:凡零版本行的模板,现 canvases+flow_chain
// 列文本原样固化为 v1(不重新 marshal,字节面诚实);created_at = 回填运行时刻;
// 谓词幂等,重跑 no-op 且 created_at 不变
func TestBackfillTemplateVersionsV1(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	idA, err := st.CreateTemplate(ctx, seedContent())
	if err != nil {
		t.Fatalf("创建模板 A: %v", err)
	}
	chainless := seedContent()
	chainless.Name = "无链模板"
	chainless.FlowChain = nil
	idB, err := st.CreateTemplate(ctx, chainless)
	if err != nil {
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
	var rawCanvases, rawChain string
	if err := st.db.QueryRow(`SELECT canvases, flow_chain FROM templates WHERE id = ?`, idA).
		Scan(&rawCanvases, &rawChain); err != nil {
		t.Fatalf("读模板 A 列: %v", err)
	}
	var vCanvases, vChain string
	var vCreatedAt string
	if err := st.db.QueryRow(
		`SELECT canvases, flow_chain, created_at FROM template_versions WHERE template_id = ? AND version = 1`, idA,
	).Scan(&vCanvases, &vChain, &vCreatedAt); err != nil {
		t.Fatalf("读版本行: %v", err)
	}
	if vCanvases != rawCanvases || vChain != rawChain {
		t.Fatalf("回填应原样固化列文本:\n canvases %q vs %q\n flow_chain %q vs %q",
			vCanvases, rawCanvases, vChain, rawChain)
	}
	if !strings.HasSuffix(vCreatedAt, "Z") || strings.Contains(vCreatedAt, "+") {
		t.Fatalf("created_at 应为 RFC3339 UTC: %q", vCreatedAt)
	}
	if _, err := time.Parse(time.RFC3339, vCreatedAt); err != nil {
		t.Fatalf("created_at 应可按 RFC3339 解析: %q (%v)", vCreatedAt, err)
	}

	// 无链模板:flow_chain 镜像 templates 列的 NULL
	var bChain any
	if err := st.db.QueryRow(
		`SELECT flow_chain FROM template_versions WHERE template_id = ? AND version = 1`, idB,
	).Scan(&bChain); err != nil {
		t.Fatalf("读模板 B 版本行: %v", err)
	}
	if bChain != nil {
		t.Fatalf("无链模板版本行 flow_chain 应为 NULL: %v", bChain)
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
	changed.Canvases = []CanvasEntry{
		{Name: "回填后续发帧", Graph: json.RawMessage(`{"canvas":{"width":794,"height":1123},"layers":[]}`)},
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
// 草稿列文本与最新版本行字节相等 → duplicate 幂等跳过(createdAt 沿用既有版本行);
// INSERT-only:版本行永无 UPDATE
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

	// 改草稿(仅 canvases 文本变化)再发布 → v2(快照 = canvases+flow_chain,
	// name 不在快照内;createdAt 形状断言,秒级解析度下不比新旧)
	changed := seedContent()
	changed.Canvases = []CanvasEntry{
		{Name: "主页改", Graph: json.RawMessage(`{"canvas":{"width":794,"height":1123},"layers":[]}`)},
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
	if _, err := st.db.Exec(`UPDATE templates SET canvases = ? WHERE id = ?`,
		`[{"graph":{"canvas":{"height":1123,"width":794},"layers":[]},"name":"主页"},{"name":"续页","graph":{"canvas":{"height":1123,"width":794},"layers":[]}}]`,
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

// TestPublishTemplateVersion_FlowChainNullAware 幂等判定的 flow_chain 两侧 NULL
// 对齐(NULL vs NULL 相等;NULL vs 文本不等)
func TestPublishTemplateVersion_FlowChainNullAware(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	chainless := seedContent()
	chainless.FlowChain = nil
	id, err := st.CreateTemplate(ctx, chainless)
	if err != nil {
		t.Fatalf("创建模板: %v", err)
	}
	if _, _, _, err := st.PublishTemplateVersion(ctx, id); err != nil {
		t.Fatalf("首发: %v", err)
	}
	// 草稿 flow_chain 仍 NULL:字节相等 → duplicate
	if _, _, dup, err := st.PublishTemplateVersion(ctx, id); err != nil || !dup {
		t.Fatalf("NULL vs NULL 应判等: duplicate=%v err=%v", dup, err)
	}
	// 草稿补上流链 → 不等 → v2
	withChain := seedContent()
	withChain.FlowChain = json.RawMessage(`[{"frame":0,"mode":"fixed"}]`)
	if err := st.UpdateTemplateContent(ctx, id, withChain); err != nil {
		t.Fatalf("补流链: %v", err)
	}
	version, _, dup, err := st.PublishTemplateVersion(ctx, id)
	if err != nil || dup || version != 2 {
		t.Fatalf("NULL vs 文本应判不等: (%d,%v,%v)", version, dup, err)
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

	// 三个版本:每次发布前改 canvases(快照 = canvases+flow_chain,只改 name
	// 会被 duplicate 幂等吸收)
	for i := range 3 {
		changed := seedContent()
		changed.Canvases = []CanvasEntry{
			{Name: "主页改" + string(rune('A'+i)), Graph: json.RawMessage(`{"canvas":{"width":794,"height":1123},"layers":[]}`)},
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

	// JSON 形状:轻量两字段(不吐 canvases)
	b, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("序列化: %v", err)
	}
	if !strings.Contains(string(b), `"version"`) || strings.Contains(string(b), `"canvases"`) {
		t.Fatalf("列表应只含 version/createdAt 轻量字段: %s", b)
	}

	if _, err := st.ListTemplateVersions(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("模板不存在期望 ErrNotFound, 实得 %v", err)
	}
}

// TestGetTemplateVersion_Roundtrip 单版本全量:canvases 文本原样、flow_chain 可空;
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
	if len(rec.Canvases) != 2 || rec.Canvases[0].Name != "主页" || rec.Canvases[1].Name != "续页" {
		t.Fatalf("canvases 形状不符: %+v", rec.Canvases)
	}
	var chain []map[string]any
	if err := json.Unmarshal(rec.FlowChain, &chain); err != nil || len(chain) != 2 {
		t.Fatalf("flowChain 应为两节点数组: %s (%v)", rec.FlowChain, err)
	}

	// 无链模板:flowChain 序列化为 JSON null(与 TemplateRecord 同款)
	chainless := seedContent()
	chainless.FlowChain = nil
	id2, err := st.CreateTemplate(ctx, chainless)
	if err != nil {
		t.Fatalf("创建无链模板: %v", err)
	}
	if _, _, _, err := st.PublishTemplateVersion(ctx, id2); err != nil {
		t.Fatalf("无链模板发布: %v", err)
	}
	rec2, err := st.GetTemplateVersion(ctx, id2, 1)
	if err != nil || rec2.FlowChain != nil {
		t.Fatalf("无链版本 flowChain 应为 nil: %+v (%v)", rec2.FlowChain, err)
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
		changed.Canvases = []CanvasEntry{
			{Name: "主页改" + string(rune('A'+i)), Graph: json.RawMessage(`{"canvas":{"width":794,"height":1123},"layers":[]}`)},
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

// TestRevertTemplateToVersion 回滚 = 单事务原子复制版本行内容回草稿列:
// canvases/flow_chain 整体替换、updated_at 刷新(与「发布不动」对照),
// name/data_source_id/editor_state 都不碰;版本线线性,回滚不产生新版本
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
	rewritten.Canvases = []CanvasEntry{{Name: "重写帧", Graph: json.RawMessage(`{"canvas":{"width":10,"height":10},"layers":[]}`)}}
	rewritten.FlowChain = nil
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
	if len(rec.Canvases) != 2 || rec.Canvases[0].Name != "主页" || rec.Canvases[1].Name != "续页" {
		t.Fatalf("canvases 应回到 v1 内容: %+v", rec.Canvases)
	}
	if !strings.Contains(string(rec.FlowChain), `"mode":"fixed"`) {
		t.Fatalf("flow_chain 应回到 v1 内容: %s", rec.FlowChain)
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
	if err != nil || len(rec.Canvases) != 2 {
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
