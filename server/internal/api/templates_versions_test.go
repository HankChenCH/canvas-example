package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"example/server/internal/pages"
	"example/server/internal/seed"
	"example/server/internal/store"
)

// --- 模板草稿-发布-版本化四端点(工单 03,spec §3/§5):publish / versions 家族 /
// revert。错误信封统一 writeError;发布守门 422 invalid_graph;回滚 200 回全量
// 模板记录(与 GET /api/templates/{id} 同形) ---

// newVersioningServer 与 newTestServer 同构,额外交出 store:发布守门用例需要
// 绕过保存预检直写坏草稿(草稿坏了还能进编辑器修,守门只在发布处)
func newVersioningServer(t *testing.T) (http.Handler, *store.Store, int64) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("开测试库: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	content, err := seed.Load(seedDir)
	if err != nil {
		t.Fatalf("装载 seed: %v", err)
	}
	dsID, err := st.SeedDataSourceIfMissing(t.Context(), content.DataSource)
	if err != nil {
		t.Fatalf("播种数据源: %v", err)
	}
	content.Template.DataSourceID = &dsID
	if err := st.SeedTemplateIfEmpty(t.Context(), content.Template); err != nil {
		t.Fatalf("播种: %v", err)
	}
	items, err := st.ListTemplates(t.Context())
	if err != nil || len(items) == 0 {
		t.Fatalf("取种子 id: %v", err)
	}
	return New(st, nil), st, items[0].ID
}

// publish POST …/publish(无 body)
func publish(t *testing.T, h http.Handler, id string) (int, map[string]any) {
	t.Helper()
	return doRequest(t, h, http.MethodPost, fmt.Sprintf("/api/templates/%s/publish", id), "application/json", nil)
}

// TestPublishTemplate_201ThenDuplicate200 首发 201 {version,createdAt}(不带
// duplicate 键);草稿未动重复发布 200 duplicate:true 幂等跳过且版本号/createdAt
// 不变;发布不动 updated_at(spec §3)
func TestPublishTemplate_201ThenDuplicate200(t *testing.T) {
	h, _, seedID := newVersioningServer(t)
	path := fmt.Sprintf("/api/templates/%d", seedID)

	// 未发布模板 GET versions → 空数组 = 未发布(推导态,spec §3)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path+"/versions", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("未发布 GET versions status = %d", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Fatalf("未发布模板版本列表应为空数组: %s", got)
	}

	code, body := publish(t, h, fmt.Sprintf("%d", seedID))
	if code != http.StatusCreated {
		t.Fatalf("首发 status = %d, 期望 201(%v)", code, body)
	}
	if body["version"].(float64) != 1 {
		t.Fatalf("首发应产生 v1: %v", body["version"])
	}
	if createdAt, _ := body["createdAt"].(string); createdAt == "" || !strings.HasSuffix(createdAt, "Z") {
		t.Fatalf("createdAt 应为 RFC3339 UTC: %v", body["createdAt"])
	}
	if _, has := body["duplicate"]; has {
		t.Fatalf("201 新版本响应不带 duplicate 键: %v", body)
	}

	// 重复发布:200 duplicate:true,版本号/createdAt 原样
	code, body = publish(t, h, fmt.Sprintf("%d", seedID))
	if code != http.StatusOK {
		t.Fatalf("重复发布 status = %d, 期望 200", code)
	}
	if body["version"].(float64) != 1 || body["duplicate"] != true {
		t.Fatalf("重复发布应 duplicate v1: %v", body)
	}

	// 发布不动 updated_at:与发布前 GET 记录对照
	_, before := doJSON(t, h, path)
	code, body = publish(t, h, fmt.Sprintf("%d", seedID))
	if code != http.StatusOK {
		t.Fatalf("再次重复发布 status = %d", code)
	}
	_, after := doJSON(t, h, path)
	if after["updatedAt"] != before["updatedAt"] {
		t.Fatalf("发布不得刷新 updated_at: %v vs %v", after["updatedAt"], before["updatedAt"])
	}
}

// TestPublishAndListVersions_ListShape 版本列表形状与降序(最新在前)
func TestPublishAndListVersions_ListShape(t *testing.T) {
	h, _, seedID := newVersioningServer(t)
	base := fmt.Sprintf("/api/templates/%d", seedID)

	// 首发 v1
	if code, body := publish(t, h, fmt.Sprintf("%d", seedID)); code != http.StatusCreated {
		t.Fatalf("首发 = %d (%v)", code, body)
	}
	// 改 pages 后再发 v2(快照 = pages 列文本,只改 name 会被 duplicate 吸收)
	changed := mustJSONFile(t, map[string]any{
		"name":  "结业证书 · 批量打印页",
		"pages": []any{minimalPage("pg_changed", "主页改")},
	})
	if code, body := doRequest(t, h, http.MethodPut, base, "application/json", changed); code != http.StatusOK {
		t.Fatalf("改草稿 = %d (%v)", code, body)
	}
	if code, body := publish(t, h, fmt.Sprintf("%d", seedID)); code != http.StatusCreated {
		t.Fatalf("再发 = %d (%v)", code, body)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, base+"/versions", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET versions = %d", rec.Code)
	}
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("版本列表应为数组: %v\n%s", err, rec.Body.String())
	}
	if len(list) != 2 {
		t.Fatalf("版本列表长度 = %d, 期望 2: %v", len(list), list)
	}
	if list[0]["version"].(float64) != 2 || list[1]["version"].(float64) != 1 {
		t.Fatalf("版本列表应降序(最新在前): %v", list)
	}
	for _, item := range list {
		keys := map[string]bool{}
		for k := range item {
			keys[k] = true
		}
		if len(keys) != 2 || !keys["version"] || !keys["createdAt"] {
			t.Fatalf("列表项应恰含 version/createdAt 轻量两字段: %v", item)
		}
	}
}

// TestPublishTemplate_Preflight422 发布守门:草稿过不了编译预检 → 422
// invalid_graph 且不产生版本行(spec §3:渲染默认踩最新已发布,必渲染失败的
// 版本不得入榜;守门码维持 422 invalid_graph 不透传 verr.Code——防御性路径,
// pages 形状错正常到不了守门,02 票 Comments 记决策)。草稿坏法经 store 直写
// 绕过保存预检(保存仍零额外校验面)
func TestPublishTemplate_Preflight422(t *testing.T) {
	h, st, seedID := newVersioningServer(t)

	broken := store.TemplateContent{
		Name: "结业证书 · 批量打印页",
		Pages: []pages.Page{
			{ID: json.RawMessage(`"pg_bad"`), Name: json.RawMessage(`"坏页"`), Graph: json.RawMessage(`{"canvas":{"width":794,"height":1123},"layers":[{"type":"GhostLayer","priority":0}]}`)},
		},
	}
	if err := st.UpdateTemplateContent(t.Context(), seedID, broken); err != nil {
		t.Fatalf("直写坏草稿: %v", err)
	}

	code, body := publish(t, h, fmt.Sprintf("%d", seedID))
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, 期望 422(%v)", code, body)
	}
	if got := errCode(t, body); got != "invalid_graph" {
		t.Fatalf("code = %s, 期望 invalid_graph", got)
	}
	if msg, _ := body["error"].(map[string]any)["message"].(string); msg == "" {
		t.Fatalf("422 应携带编译预检中文消息: %v", body)
	}

	// 不产生版本行:GET versions 仍为空数组
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/templates/%d/versions", seedID), nil))
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Fatalf("守门失败不得产生版本行: %s", got)
	}
}

// TestPublishTemplate_GateBeforeDuplicate 守门先于 duplicate 判定(评审修复 F6
// 钉死):回填 v1 零校验——坏图草稿经 BackfillTemplateVersionsV1 原样固化为 v1 后,
// 重发同内容字节面必命中 duplicate 判定(草稿列与最新版本行文本相等),但
// 编译守门在前 → 422 invalid_graph,不产版本行、不 duplicate。发布是主动
// 确认,坏内容不得借幂等通道蒙混(修复路径 = 编辑草稿后再发布)
func TestPublishTemplate_GateBeforeDuplicate(t *testing.T) {
	h, st, seedID := newVersioningServer(t)

	// 坏图草稿直写(绕过保存预检——草稿坏了还能进编辑器修,守门只在发布处),
	// 回填把同一坏内容零校验固化为 v1
	broken := store.TemplateContent{
		Name: "结业证书 · 批量打印页",
		Pages: []pages.Page{
			{ID: json.RawMessage(`"pg_bad"`), Name: json.RawMessage(`"坏页"`), Graph: json.RawMessage(`{"canvas":{"width":794,"height":1123},"layers":[{"type":"GhostLayer","priority":0}]}`)},
		},
	}
	if err := st.UpdateTemplateContent(t.Context(), seedID, broken); err != nil {
		t.Fatalf("直写坏草稿: %v", err)
	}
	if _, err := st.BackfillTemplateVersionsV1(t.Context()); err != nil {
		t.Fatalf("回填 v1: %v", err)
	}

	// 草稿与 v1 字节相等(duplicate 判定必命中)仍被守门打回:422 invalid_graph
	code, body := publish(t, h, fmt.Sprintf("%d", seedID))
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, 期望 422(守门先于 duplicate, %v)", code, body)
	}
	if got := errCode(t, body); got != "invalid_graph" {
		t.Fatalf("code = %s, 期望 invalid_graph", got)
	}

	// 不产版本行、不 duplicate:版本史仍只有回填的 v1
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/templates/%d/versions", seedID), nil))
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("版本列表应为数组: %v\n%s", err, rec.Body.String())
	}
	if len(list) != 1 || list[0]["version"].(float64) != 1 {
		t.Fatalf("守门失败不得产生新版本行(仍只有回填 v1): %v", list)
	}
}

// TestPublishTemplate_NotFound404 寻址失败 → 404 template_not_found
func TestPublishTemplate_NotFound404(t *testing.T) {
	h, _, _ := newVersioningServer(t)
	for _, id := range []string{"999", "abc"} {
		code, body := publish(t, h, id)
		if code != http.StatusNotFound || errCode(t, body) != "template_not_found" {
			t.Fatalf("publish %s = %d/%s, 期望 404/template_not_found", id, code, errCode(t, body))
		}
	}
}

// TestGetTemplateVersion_Endpoint 单版本全量 {version,pages,createdAt};
// 版本不存在/路径非整数/对无版本模板指定 → 404 version_not_found;列表寻址失败
// (模板不存在)→ 404 template_not_found
func TestGetTemplateVersion_Endpoint(t *testing.T) {
	h, _, seedID := newVersioningServer(t)
	base := fmt.Sprintf("/api/templates/%d", seedID)

	if code, body := publish(t, h, fmt.Sprintf("%d", seedID)); code != http.StatusCreated {
		t.Fatalf("首发 = %d (%v)", code, body)
	}

	code, body := doJSON(t, h, base+"/versions/1")
	if code != http.StatusOK {
		t.Fatalf("GET versions/1 = %d (%v)", code, body)
	}
	if body["version"].(float64) != 1 {
		t.Fatalf("version 不符: %v", body["version"])
	}
	pages, ok := body["pages"].([]any)
	if !ok || len(pages) != 2 {
		t.Fatalf("pages 应与 TemplateRecord.Pages 同形(两页): %v", body["pages"])
	}
	first := pages[0].(map[string]any)
	if first["name"] != "主页" || first["id"] == "" || first["graph"] == nil {
		t.Fatalf("pages[0] 应为 {id,name,graph,...}(页 id 随 pages 进快照): %v", first)
	}
	if _, ok := body["createdAt"].(string); !ok {
		t.Fatalf("createdAt 应为字符串: %v", body["createdAt"])
	}
	for key := range body {
		switch key {
		case "version", "pages", "createdAt":
		default:
			t.Fatalf("单版本全量不应含多余键 %q: %v", key, body)
		}
	}

	// 版本不存在 → 404 version_not_found
	code, body = doJSON(t, h, base+"/versions/99")
	if code != http.StatusNotFound || errCode(t, body) != "version_not_found" {
		t.Fatalf("GET versions/99 = %d/%s, 期望 404/version_not_found", code, errCode(t, body))
	}
	// 路径非整数 → 404 version_not_found
	code, body = doJSON(t, h, base+"/versions/abc")
	if code != http.StatusNotFound || errCode(t, body) != "version_not_found" {
		t.Fatalf("GET versions/abc = %d/%s, 期望 404/version_not_found", code, errCode(t, body))
	}
	// 对无版本模板指定 → 404 version_not_found(spec §4 同精神)
	payload := mustJSONFile(t, map[string]any{
		"name":  "未发布模板",
		"pages": []any{minimalPage("pg_unpub", "单页")},
	})
	code, body = doRequest(t, h, http.MethodPost, "/api/templates", "application/json", payload)
	if code != http.StatusCreated {
		t.Fatalf("建未发布模板 = %d (%v)", code, body)
	}
	freshID := int64(body["id"].(float64))
	code, body = doJSON(t, h, fmt.Sprintf("/api/templates/%d/versions/1", freshID))
	if code != http.StatusNotFound || errCode(t, body) != "version_not_found" {
		t.Fatalf("无版本模板指定 = %d/%s, 期望 404/version_not_found", code, errCode(t, body))
	}

	// 模板不存在的列表寻址 → 404 template_not_found(与 GetTemplate 惯例一致)
	code, body = doJSON(t, h, "/api/templates/999/versions")
	if code != http.StatusNotFound || errCode(t, body) != "template_not_found" {
		t.Fatalf("GET 999/versions = %d/%s, 期望 404/template_not_found", code, errCode(t, body))
	}
}

// TestRevertTemplate_Endpoint 回滚:200 回全量模板记录(与 GET 同形),pages
// 回到目标版本;updated_at 刷新;404 映射与坏 body 400
func TestRevertTemplate_Endpoint(t *testing.T) {
	h, _, seedID := newVersioningServer(t)
	base := fmt.Sprintf("/api/templates/%d", seedID)

	if code, body := publish(t, h, fmt.Sprintf("%d", seedID)); code != http.StatusCreated {
		t.Fatalf("首发 v1 = %d (%v)", code, body)
	}
	// v1 全量(回滚基线)
	_, v1 := doJSON(t, h, base+"/versions/1")

	// 草稿重写 + 发 v2(版本史留痕)
	rewritten := mustJSONFile(t, map[string]any{
		"name":  "结业证书 · 批量打印页",
		"pages": []any{minimalPage("pg_rw", "重写页")},
	})
	if code, body := doRequest(t, h, http.MethodPut, base, "application/json", rewritten); code != http.StatusOK {
		t.Fatalf("重写草稿 = %d (%v)", code, body)
	}
	if code, body := publish(t, h, fmt.Sprintf("%d", seedID)); code != http.StatusCreated {
		t.Fatalf("发 v2 = %d (%v)", code, body)
	}

	// 回滚 v1 → 200 全量记录
	code, body := doRequest(t, h, http.MethodPost, base+"/revert", "application/json", mustJSONFile(t, map[string]any{"version": 1}))
	if code != http.StatusOK {
		t.Fatalf("revert = %d (%v)", code, body)
	}
	if body["id"].(float64) != float64(seedID) || body["name"] == nil || body["createdAt"] == nil {
		t.Fatalf("revert 应回全量模板记录(与 GET 同形): %v", body)
	}
	reverted, _ := json.Marshal(body["pages"])
	v1Pages, _ := json.Marshal(v1["pages"])
	if string(reverted) != string(v1Pages) {
		t.Fatalf("revert 后 pages 应回到 v1:\n%s\nvs\n%s", reverted, v1Pages)
	}
	// (updated_at 刷新的确定性断言在 store 层:insertTemplate 固定旧时间戳,
	// RFC3339 秒级解析度下 API 层无法免撞车)

	// 回滚后 GET 全量记录与 revert 响应一致,且版本史仍为 v1/v2(线性,不造版)
	code, rec2 := doJSON(t, h, base)
	if code != http.StatusOK {
		t.Fatalf("GET = %d", code)
	}
	rec2Pages, _ := json.Marshal(rec2["pages"])
	if string(rec2Pages) != string(v1Pages) {
		t.Fatalf("revert 后 GET 的 pages 应与目标版本一致: %s", rec2Pages)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, base+"/versions", nil))
	var list []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 2 || list[0]["version"].(float64) != 2 {
		t.Fatalf("回滚不得产生新版本: %v", list)
	}

	// 404 映射:版本不存在 → version_not_found;模板不存在 → template_not_found
	code, body = doRequest(t, h, http.MethodPost, base+"/revert", "application/json", mustJSONFile(t, map[string]any{"version": 99}))
	if code != http.StatusNotFound || errCode(t, body) != "version_not_found" {
		t.Fatalf("revert 99 = %d/%s, 期望 404/version_not_found", code, errCode(t, body))
	}
	code, body = doRequest(t, h, http.MethodPost, "/api/templates/999/revert", "application/json", mustJSONFile(t, map[string]any{"version": 1}))
	if code != http.StatusNotFound || errCode(t, body) != "template_not_found" {
		t.Fatalf("revert 模板不存在 = %d/%s, 期望 404/template_not_found", code, errCode(t, body))
	}

	// 坏 body → 400 invalid_json:坏 JSON / 缺 version / 类型错 / 非正整数
	badBodies := map[string][]byte{
		"坏JSON":    []byte("not json"),
		"缺version": []byte(`{}`),
		"类型错":      []byte(`{"version":"1"}`),
		"零":        []byte(`{"version":0}`),
		"负数":       []byte(`{"version":-1}`),
		"小数":       []byte(`{"version":1.5}`),
		"null载荷":   []byte(`null`),
	}
	for name, raw := range badBodies {
		code, body := doRequest(t, h, http.MethodPost, base+"/revert", "application/json", raw)
		if code != http.StatusBadRequest || errCode(t, body) != "invalid_json" {
			t.Fatalf("[revert %s] = %d/%s, 期望 400/invalid_json", name, code, errCode(t, body))
		}
	}
}
