package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

// httptestGet GET 请求返回原始 recorder(断言状态码/响应头用)
func httptestGet(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// postRender POST 渲染端点(无 body,spec §2.4 #7),返回 status + body
func postRender(t *testing.T, h http.Handler, id string) (int, map[string]any) {
	t.Helper()
	return doRequest(t, h, http.MethodPost, fmt.Sprintf("/api/templates/%s/render", id), "", nil)
}

// templateTableGraph 带模板态槽位表的画布(链上帧必须恰有 1 张,spec §3.3)
func templateTableGraph() map[string]any {
	return map[string]any{
		"canvas": map[string]any{"width": 794, "height": 1123},
		"layers": []any{map[string]any{
			"type": "TableLayer", "name": "槽位表", "priority": 50,
			"data": map[string]any{"rowsPath": "certificates"},
			"template": map[string]any{
				"type": "TableRowTemplate",
				"spec": map[string]any{"shape": map[string]any{"width": 714, "height": 400}},
				"cells": []any{map[string]any{
					"type": "TableCellLayer", "priority": 0,
					"spec": map[string]any{"shape": map[string]any{"width": 714, "height": 400}},
				}},
			},
		}},
	}
}

// newLocalRenderTemplate 造一个离线可渲染的模板(双页模板表 + 流策略,ADR 0025
// 页序列形状),创建即绑定数据源(单调用,spec §2.4 #3)并发布 v1(缺省渲染语义 =
// 最新已发布,明示 breaking change,spec §4)后返回模板 id
func newLocalRenderTemplate(t *testing.T, h http.Handler) int64 {
	t.Helper()
	ds := mustJSONFile(t, map[string]any{
		"name":   "渲染用数据源",
		"schema": map[string]any{"type": "object"},
		"data":   map[string]any{"certificates": []any{map[string]any{"n": "x"}}},
	})
	code, body := doRequest(t, h, http.MethodPost, "/api/datasources", "application/json", ds)
	if code != http.StatusCreated {
		t.Fatalf("造数据源失败: %d %v", code, body)
	}
	payload := mustJSONFile(t, map[string]any{
		"name": "渲染用本地模板",
		"pages": []any{
			map[string]any{"id": "pg_main", "name": "主页", "graph": templateTableGraph(), "flow": map[string]any{"mode": "fixed"}},
			map[string]any{"id": "pg_cont", "name": "续页", "graph": templateTableGraph(), "flow": map[string]any{"mode": "paged", "omitIfEmpty": true}},
		},
		"dataSourceId": body["id"],
	})
	code, body = doRequest(t, h, http.MethodPost, "/api/templates", "application/json", payload)
	if code != http.StatusCreated {
		t.Fatalf("造模板失败: %d %v", code, body)
	}
	id := int64(body["id"].(float64))
	code, body = doRequest(t, h, http.MethodPost, fmt.Sprintf("/api/templates/%d/publish", id), "application/json", nil)
	if code != http.StatusCreated {
		t.Fatalf("发布渲染用模板失败: %d %v", code, body)
	}
	return id
}

// TestPostRender_201SeedShape 渲染 → 201 RenderRecord:images 页序扁平(页归属
// 字段 frame 语义延续为页下标,ADR 0025)、url 前导斜杠形态;逐 url GET 200 且
// Content-Type image/png(spec §2.3/§2.4 #7)
func TestPostRender_201Shape(t *testing.T) {
	h, _ := newTestServer(t)
	t.Chdir(t.TempDir())
	id := newLocalRenderTemplate(t, h)

	code, body := postRender(t, h, fmt.Sprintf("%d", id))
	if code != http.StatusCreated {
		t.Fatalf("status = %d, 期望 201(%v)", code, body)
	}
	if body["templateId"].(float64) != float64(id) {
		t.Fatalf("templateId 不符: %v", body["templateId"])
	}
	if _, ok := body["createdAt"].(string); !ok || body["createdAt"] == "" {
		t.Fatalf("createdAt 应为 RFC3339 字符串: %v", body["createdAt"])
	}
	images, ok := body["images"].([]any)
	if !ok || len(images) != 1 {
		t.Fatalf("单行 dataset 下 paged 续页应 omitIfEmpty 跳帧,仅主页 1 页: %v", body["images"])
	}
	img := images[0].(map[string]any)
	if img["frame"] != float64(0) || img["name"] != "主页" {
		t.Fatalf("images[0] frame/name 不符: %v", img)
	}
	url, _ := img["url"].(string)
	if !regexp.MustCompile(`^/renders/\d+/1\.png$`).MatchString(url) {
		t.Fatalf("url 应匹配 ^/renders/\\d+/1\\.png$: %q", url)
	}

	// 产物直链:200 + image/png(spec §2.4 #9)
	rec := httptestGet(t, h, url)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, 期望 200", url, rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("Content-Type = %q, 期望 image/png", ct)
	}
}

// TestPostRender_NotFound404 寻址失败 → 404 template_not_found(spec §2.4 通用寻址)
func TestPostRender_NotFound404(t *testing.T) {
	h, _ := newTestServer(t)
	for _, id := range []string{"999", "abc"} {
		code, body := postRender(t, h, id)
		if code != http.StatusNotFound || errCode(t, body) != "template_not_found" {
			t.Fatalf("render %s = %d/%s, 期望 404/template_not_found", id, code, errCode(t, body))
		}
	}
}

// TestPostRender_KeepAll keep-all 不清理(spec §2.4 #7):改模板后再渲染,
// 旧 renderId 的 url 仍服务旧图
func TestPostRender_KeepAll(t *testing.T) {
	h, _ := newTestServer(t)
	t.Chdir(t.TempDir())
	id := newLocalRenderTemplate(t, h)

	_, first := postRender(t, h, fmt.Sprintf("%d", id))
	firstURL := first["images"].([]any)[0].(map[string]any)["url"].(string)
	oldRenderID := regexp.MustCompile(`^/renders/(\d+)/`).FindStringSubmatch(firstURL)[1]

	// 改模板(整存替换)后再渲染:新渲染记录新 id(缺省渲染 = 最新已发布 v1,
	// 与草稿改动无关——keep-all 旧图快照仍服务)
	put := mustJSONFile(t, map[string]any{
		"name":  "改名后的渲染模板",
		"pages": []any{map[string]any{"id": "pg_main", "name": "主页", "graph": templateTableGraph()}},
	})
	code, body := doRequest(t, h, http.MethodPut, fmt.Sprintf("/api/templates/%d", id), "application/json", put)
	if code != http.StatusOK {
		t.Fatalf("PUT 模板失败: %d %v", code, body)
	}
	_, second := postRender(t, h, fmt.Sprintf("%d", id))
	secondURL := second["images"].([]any)[0].(map[string]any)["url"].(string)
	newRenderID := regexp.MustCompile(`^/renders/(\d+)/`).FindStringSubmatch(secondURL)[1]
	if oldRenderID == newRenderID {
		t.Fatalf("两次渲染 id 不应相同: %s", oldRenderID)
	}

	// 旧渲染产物仍是快照:GET 200 + image/png
	rec := httptestGet(t, h, firstURL)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("旧渲染 %s 应仍可服务: %d %q", firstURL, rec.Code, rec.Header().Get("Content-Type"))
	}
}

// --- 渲染选版(工单 04,spec §4):可选 body {version: number|"draft"} ---

// postRenderBody 带任意 body 的 POST 渲染
func postRenderBody(t *testing.T, h http.Handler, id, body string) (int, map[string]any) {
	t.Helper()
	return doRequest(t, h, http.MethodPost, fmt.Sprintf("/api/templates/%s/render", id), "application/json", []byte(body))
}

// rewriteDraftCanvas 整存替换草稿为单页指定名(改 pages 文本,发布不 duplicate)
func rewriteDraftCanvas(t *testing.T, h http.Handler, id int64, pageName string) {
	t.Helper()
	put := mustJSONFile(t, map[string]any{
		"name":  "渲染用本地模板",
		"pages": []any{map[string]any{"id": "pg_draft", "name": pageName, "graph": templateTableGraph()}},
	})
	code, body := doRequest(t, h, http.MethodPut, fmt.Sprintf("/api/templates/%d", id), "application/json", put)
	if code != http.StatusOK {
		t.Fatalf("改草稿失败: %d %v", code, body)
	}
}

// publishDraft 发布当前草稿,返回新版本号
func publishDraft(t *testing.T, h http.Handler, id int64) float64 {
	t.Helper()
	code, body := doRequest(t, h, http.MethodPost, fmt.Sprintf("/api/templates/%d/publish", id), "application/json", nil)
	if code != http.StatusCreated {
		t.Fatalf("发布失败: %d %v", code, body)
	}
	return body["version"].(float64)
}

// firstPageName 渲染记录首页的页名(选版内容断言位)
func firstPageName(t *testing.T, body map[string]any) string {
	t.Helper()
	images, ok := body["images"].([]any)
	if !ok || len(images) == 0 {
		t.Fatalf("渲染记录应至少一页: %v", body)
	}
	return images[0].(map[string]any)["name"].(string)
}

// TestPostRender_SelectionMatrix 选版矩阵:缺省 = 最新已发布(明示 breaking
// change,草稿领先也不踩);显式数字 = 指定历史版本;draft 哨兵 = 草稿列现值;
// 记录 version 字段逐态正确(draft 渲染 null 透传)
func TestPostRender_SelectionMatrix(t *testing.T) {
	h, _ := newTestServer(t)
	t.Chdir(t.TempDir())
	id := newLocalRenderTemplate(t, h) // v1 = 主页/续页 + 流策略
	rewriteDraftCanvas(t, h, id, "主页改")
	if got := publishDraft(t, h, id); got != 2 {
		t.Fatalf("改版后发布应得 v2, 实得 %v", got)
	}
	rewriteDraftCanvas(t, h, id, "草稿页") // 草稿领先于 v2,不发布

	// 缺省(无 body)= 最新已发布 v2,而非草稿
	code, body := postRender(t, h, fmt.Sprintf("%d", id))
	if code != http.StatusCreated {
		t.Fatalf("无 body 渲染 = %d (%v)", code, body)
	}
	if v, _ := body["version"].(float64); v != 2 {
		t.Fatalf("缺省渲染应记 version=2: %v", body["version"])
	}
	if got := firstPageName(t, body); got != "主页改" {
		t.Fatalf("缺省渲染应踩 v2 内容(主页改,明示 breaking change), 实得 %q", got)
	}

	// 显式数字 v1 = 历史版本内容
	code, body = postRenderBody(t, h, fmt.Sprintf("%d", id), `{"version":1}`)
	if code != http.StatusCreated {
		t.Fatalf("{version:1} 渲染 = %d (%v)", code, body)
	}
	if v, _ := body["version"].(float64); v != 1 {
		t.Fatalf("显式渲染应记 version=1: %v", body["version"])
	}
	if got := firstPageName(t, body); got != "主页" {
		t.Fatalf("显式 v1 渲染应踩 v1 内容(主页), 实得 %q", got)
	}

	// draft 哨兵 = 草稿列现值(编辑器预览专用,票 03 Comments 契约修订)
	code, body = postRenderBody(t, h, fmt.Sprintf("%d", id), `{"version":"draft"}`)
	if code != http.StatusCreated {
		t.Fatalf("{version:draft} 渲染 = %d (%v)", code, body)
	}
	if body["version"] != nil {
		t.Fatalf("draft 渲染记录 version 应 null 透传: %v", body["version"])
	}
	if got := firstPageName(t, body); got != "草稿页" {
		t.Fatalf("draft 渲染应踩草稿列现值, 实得 %q", got)
	}

	// {"version":null} 视同缺省(null 归一先例,与缺字段同语义)
	code, body = postRenderBody(t, h, fmt.Sprintf("%d", id), `{"version":null}`)
	if code != http.StatusCreated {
		t.Fatalf("{version:null} 渲染 = %d (%v)", code, body)
	}
	if v, _ := body["version"].(float64); v != 2 {
		t.Fatalf("version null 应视同缺省(最新已发布): %v", body["version"])
	}
}

// TestPostRender_Unpublished409 从未发布 → 409 template_unpublished(状态冲突型);
// 对无版本模板指定数字 → 404 version_not_found(寻址失败)
func TestPostRender_Unpublished409(t *testing.T) {
	h, _ := newTestServer(t)
	t.Chdir(t.TempDir())
	id := newUnpublishedTemplate(t, h)

	code, body := postRender(t, h, fmt.Sprintf("%d", id))
	if code != http.StatusConflict || errCode(t, body) != "template_unpublished" {
		t.Fatalf("未发布渲染 = %d/%s, 期望 409/template_unpublished", code, errCode(t, body))
	}
	if msg, _ := body["error"].(map[string]any)["message"].(string); msg == "" {
		t.Fatalf("409 应携带可读 message: %v", body)
	}

	// 对无版本模板指定数字 → 404 version_not_found(spec §4 含对无版本模板指定)
	code, body = postRenderBody(t, h, fmt.Sprintf("%d", id), `{"version":1}`)
	if code != http.StatusNotFound || errCode(t, body) != "version_not_found" {
		t.Fatalf("无版本模板指定 = %d/%s, 期望 404/version_not_found", code, errCode(t, body))
	}
}

// TestPostRender_VersionNotFound404 指定不存在的版本 → 404 version_not_found
func TestPostRender_VersionNotFound404(t *testing.T) {
	h, _ := newTestServer(t)
	t.Chdir(t.TempDir())
	id := newLocalRenderTemplate(t, h) // 已发布 v1

	code, body := postRenderBody(t, h, fmt.Sprintf("%d", id), `{"version":99}`)
	if code != http.StatusNotFound || errCode(t, body) != "version_not_found" {
		t.Fatalf("{version:99} = %d/%s, 期望 404/version_not_found", code, errCode(t, body))
	}
}

// TestPostRender_InvalidSelection400 version 形态错误 → 400 invalid_json:
// 类型错/零/负数/非整数/未知字符串哨兵/坏 JSON
func TestPostRender_InvalidSelection400(t *testing.T) {
	h, _ := newTestServer(t)
	t.Chdir(t.TempDir())
	id := newLocalRenderTemplate(t, h)

	for name, raw := range map[string]string{
		"未知字符串": `{"version":"nope"}`,
		"零":     `{"version":0}`,
		"负数":    `{"version":-1}`,
		"非整数":   `{"version":1.5}`,
		"布尔":    `{"version":true}`,
		"数组":    `{"version":[1]}`,
		"对象":    `{"version":{"n":1}}`,
		"坏JSON": `not json`,
		"尾随内容":  `{"version":1} tail`,
	} {
		code, body := postRenderBody(t, h, fmt.Sprintf("%d", id), raw)
		if code != http.StatusBadRequest || errCode(t, body) != "invalid_json" {
			t.Fatalf("[%s] = %d/%s, 期望 400/invalid_json", name, code, errCode(t, body))
		}
	}
}

// TestPostRender_TemplateMissingWithBody404 先寻址模板:合法 body + 模板不存在
// → 404 template_not_found(spec §4)
func TestPostRender_TemplateMissingWithBody404(t *testing.T) {
	h, _ := newTestServer(t)
	code, body := postRenderBody(t, h, "999", `{"version":1}`)
	if code != http.StatusNotFound || errCode(t, body) != "template_not_found" {
		t.Fatalf("999 + body = %d/%s, 期望 404/template_not_found", code, errCode(t, body))
	}
}

// newUnpublishedTemplate 造一个离线可渲染但从未发布的模板(无 flow 单页直通)
func newUnpublishedTemplate(t *testing.T, h http.Handler) int64 {
	t.Helper()
	payload := mustJSONFile(t, map[string]any{
		"name":  "未发布渲染模板",
		"pages": []any{map[string]any{"id": "pg_unpub", "name": "单页", "graph": minimalGraph()}},
	})
	code, body := doRequest(t, h, http.MethodPost, "/api/templates", "application/json", payload)
	if code != http.StatusCreated {
		t.Fatalf("造未发布模板失败: %d %v", code, body)
	}
	return int64(body["id"].(float64))
}

// TestGetRenders_HistoryList 渲染历史列表(spec:GET /api/templates/{id}/renders):
// POST 两条后 GET → 200 数组 id 降序(最新在前)、响应形状与 POST render 一致
// (version/createdAt/images[].url 前导斜杠)、draft 渲染行 version null;模板
// 不存在 404;无记录模板 200 空数组
func TestGetRenders_HistoryList(t *testing.T) {
	h, seedID := newTestServer(t)
	t.Chdir(t.TempDir())
	id := newLocalRenderTemplate(t, h)

	code, rOld := postRender(t, h, fmt.Sprintf("%d", id))
	if code != http.StatusCreated {
		t.Fatalf("首渲 status = %d(%v)", code, rOld)
	}
	code, rNew := postRender(t, h, fmt.Sprintf("%d", id))
	if code != http.StatusCreated {
		t.Fatalf("再渲 status = %d(%v)", code, rNew)
	}

	list := getRenders(t, h, id)
	if len(list) != 2 {
		t.Fatalf("应 2 条记录, 实得 %d: %v", len(list), list)
	}
	if list[0]["id"].(float64) != rNew["id"].(float64) || list[1]["id"].(float64) != rOld["id"].(float64) {
		t.Fatalf("id 应降序(最新在前): %v / %v", list[0]["id"], list[1]["id"])
	}
	for i, item := range list {
		// 缺省渲染 = 最新已发布(v1,见 newLocalRenderTemplate 发布)
		if item["version"].(float64) != 1 {
			t.Fatalf("list[%d].version 应为 1: %v", i, item["version"])
		}
		if _, ok := item["createdAt"].(string); !ok || item["createdAt"] == "" {
			t.Fatalf("list[%d].createdAt 应为非空字符串: %v", i, item)
		}
		images, ok := item["images"].([]any)
		if !ok || len(images) != 1 {
			t.Fatalf("list[%d].images 应 1 页: %v", i, item["images"])
		}
		img := images[0].(map[string]any)
		if !regexp.MustCompile(`^/renders/\d+/1\.png$`).MatchString(img["url"].(string)) {
			t.Fatalf("list[%d] url 应前导斜杠形态: %v", i, img["url"])
		}
	}

	// draft 渲染行入列且 version null
	code, draft := doRequest(t, h, http.MethodPost, fmt.Sprintf("/api/templates/%d/render", id), "application/json", []byte(`{"version":"draft"}`))
	if code != http.StatusCreated {
		t.Fatalf("draft 渲染 status = %d(%v)", code, draft)
	}
	list = getRenders(t, h, id)
	if len(list) != 3 {
		t.Fatalf("draft 后应 3 条, 实得 %d", len(list))
	}
	if list[0]["id"].(float64) != draft["id"].(float64) {
		t.Fatalf("最新一条应为 draft 记录: %v vs %v", list[0]["id"], draft["id"])
	}
	if list[0]["version"] != nil {
		t.Fatalf("draft 记录 version 应 null: %v", list[0]["version"])
	}

	// 模板不存在/非整数 id → 404 template_not_found
	for _, path := range []string{"/api/templates/999/renders", "/api/templates/abc/renders"} {
		code, body := doRequest(t, h, http.MethodGet, path, "", nil)
		if code != http.StatusNotFound || errCode(t, body) != "template_not_found" {
			t.Fatalf("GET %s = %d/%s, 期望 404/template_not_found", path, code, errCode(t, body))
		}
	}

	// 无记录模板(seed)→ 200 空数组
	if list := getRenders(t, h, seedID); len(list) != 0 {
		t.Fatalf("无记录模板应空数组: %v", list)
	}
}

// getRenders GET 渲染历史列表并解数组响应(数组端点与 doRequest 对象解码分开)
func getRenders(t *testing.T, h http.Handler, id int64) []map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/templates/%d/renders", id), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET renders = %d, 期望 200: %s", rec.Code, rec.Body.String())
	}
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("渲染历史响应应为数组: %v\n%s", err, rec.Body.String())
	}
	return list
}
