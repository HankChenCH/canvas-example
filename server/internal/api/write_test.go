package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

// --- 通用小工具 ---

// doRequest 执行请求并解析响应 JSON(POST/PUT 等;返回 status + body 对象)
func doRequest(t *testing.T, h http.Handler, method, path, contentType string, body []byte) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func mustJSONFile(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// errCode 从错误信封取 code
func errCode(t *testing.T, body map[string]any) string {
	t.Helper()
	env, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("响应应为错误信封: %v", body)
	}
	code, _ := env["code"].(string)
	return code
}

// minimalGraph 最小合法画布 graph
func minimalGraph() map[string]any {
	return map[string]any{
		"canvas": map[string]any{"width": 794, "height": 1123},
		"layers": []any{},
	}
}

// --- POST /api/templates(spec §2.4 #3) ---

func TestPostTemplate_Minimal201(t *testing.T) {
	h, seedID := newTestServer(t)
	payload := mustJSONFile(t, map[string]any{
		"name":     "冒烟最小模板",
		"canvases": []any{map[string]any{"name": "单页", "graph": minimalGraph()}},
	})
	code, body := doRequest(t, h, http.MethodPost, "/api/templates", "application/json", payload)
	if code != http.StatusCreated {
		t.Fatalf("status = %d, 期望 201(body: %v)", code, body)
	}
	id, ok := body["id"].(float64)
	if !ok || id <= float64(seedID) {
		t.Fatalf("id 应为自增整数且大于种子 id %d: %v", seedID, body["id"])
	}
	// POST 载荷缺省未绑数据源(spec §2.4 #3):dataSourceId null、flowChain 缺省 null
	if body["dataSourceId"] != nil || body["flowChain"] != nil {
		t.Fatalf("新模板 dataSourceId/flowChain 应为 null: %v", body)
	}
	if body["createdAt"] != body["updatedAt"] {
		t.Fatalf("createdAt 应等于 updatedAt: %v / %v", body["createdAt"], body["updatedAt"])
	}
	// 全量记录可回读
	getCode, getBody := doRequest(t, h, http.MethodGet, fmt.Sprintf("/api/templates/%d", int64(id)), "", nil)
	if getCode != http.StatusOK || getBody["name"] != "冒烟最小模板" {
		t.Fatalf("回读失败: %d %v", getCode, getBody["name"])
	}
}

// listCount 取模板列表行数(列表端点响应为数组,与 doRequest 的对象解码分开)
func listCount(t *testing.T, h http.Handler) int {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/templates", nil))
	var list []any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("列表响应应为数组: %v\n%s", err, rec.Body.String())
	}
	return len(list)
}

// TestPostTemplate_PreflightRejects 保存即预检:400 打回不落库(spec §3.4);
// 错误码逐一对号在 preflight 包单测,此处压 HTTP 映射与不落库
func TestPostTemplate_PreflightRejects(t *testing.T) {
	h, _ := newTestServer(t)
	cases := map[string]struct {
		payload  map[string]any
		wantCode string
	}{
		"未知图层类型": {
			map[string]any{"name": "坏图层", "canvases": []any{map[string]any{
				"name": "单页",
				"graph": map[string]any{"canvas": map[string]any{"width": 100, "height": 100},
					"layers": []any{map[string]any{"type": "GhostLayer", "priority": 0}}},
			}}},
			"unknown_layer_type",
		},
		"双paged流链": {
			map[string]any{
				"name": "坏链",
				"canvases": []any{
					map[string]any{"name": "帧一", "graph": minimalGraph()},
					map[string]any{"name": "帧二", "graph": minimalGraph()},
				},
				"flowChain": []any{
					map[string]any{"frame": 0, "mode": "paged"},
					map[string]any{"frame": 1, "mode": "paged"},
				},
			},
			"flow_chain_invalid",
		},
		"流链节点非JSON对象": {
			map[string]any{
				"name":      "坏节点",
				"canvases":  []any{map[string]any{"name": "帧一", "graph": minimalGraph()}},
				"flowChain": []any{"fixed"},
			},
			"invalid_json",
		},
	}
	for name, tc := range cases {
		countBefore := listCount(t, h)

		code, body := doRequest(t, h, http.MethodPost, "/api/templates", "application/json", mustJSONFile(t, tc.payload))
		if code != http.StatusBadRequest {
			t.Fatalf("[%s] status = %d, 期望 400", name, code)
		}
		if got := errCode(t, body); got != tc.wantCode {
			t.Fatalf("[%s] code = %s, 期望 %s", name, got, tc.wantCode)
		}
		// 预检不过不落库(spec §3.4)
		if got := listCount(t, h); got != countBefore {
			t.Fatalf("[%s] 预检失败不得落库: 落库前 %d 行,落库后 %d 行", name, countBefore, got)
		}
	}
}

// --- PUT /api/templates/{id}(spec §2.4 #5) ---

// TestPutTemplate_PreservesBinding 绑定通道先行,再 PUT 模板:引用列不触碰
// (绑定只经数据源通道变更,spec §2.4 #5)
func TestPutTemplate_PreservesBinding(t *testing.T) {
	h, seedID := newTestServer(t)
	path := fmt.Sprintf("/api/templates/%d", seedID)

	// 先建数据源并绑定到种子模板
	dsPayload := mustJSONFile(t, map[string]any{
		"name":   "绑定用数据源",
		"schema": map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "string"}}, "required": []string{"a"}},
		"data":   map[string]any{"a": "值"},
	})
	code, body := doRequest(t, h, http.MethodPost, "/api/datasources", "application/json", dsPayload)
	if code != http.StatusCreated {
		t.Fatalf("POST datasources status = %d, 期望 201(%v)", code, body)
	}
	dsID := int64(body["id"].(float64))
	bindPayload := mustJSONFile(t, map[string]any{"dataSourceId": dsID})
	code, body = doRequest(t, h, http.MethodPut, path+"/datasource", "application/json", bindPayload)
	if code != http.StatusOK {
		t.Fatalf("PUT datasource 绑定 status = %d, 期望 200(%v)", code, body)
	}

	// 再 PUT 模板:name/canvases/flowChain 整存替换,flowChain 键缺省视同 null
	putPayload := mustJSONFile(t, map[string]any{
		"name":     "改名后的模板",
		"canvases": []any{map[string]any{"name": "新帧", "graph": minimalGraph()}},
	})
	code, body = doRequest(t, h, http.MethodPut, path, "application/json", putPayload)
	if code != http.StatusOK {
		t.Fatalf("PUT template status = %d, 期望 200(%v)", code, body)
	}
	if body["name"] != "改名后的模板" {
		t.Fatalf("name 应整存替换: %v", body["name"])
	}
	if got, ok := body["dataSourceId"].(float64); !ok || int64(got) != dsID {
		t.Fatalf("dataSourceId 不得被触碰(spec §2.4 #5): %v", body["dataSourceId"])
	}
	if body["flowChain"] != nil {
		t.Fatalf("flowChain 键缺省应视同 null: %v", body["flowChain"])
	}
	if len(body["canvases"].([]any)) != 1 {
		t.Fatalf("canvases 应整存替换: %v", body["canvases"])
	}
}

func TestPutTemplate_NotFound404(t *testing.T) {
	h, _ := newTestServer(t)
	payload := mustJSONFile(t, map[string]any{"name": "不存在", "canvases": []any{}})
	code, body := doRequest(t, h, http.MethodPut, "/api/templates/999", "application/json", payload)
	if code != http.StatusNotFound || errCode(t, body) != "template_not_found" {
		t.Fatalf("status/code = %d/%s, 期望 404/template_not_found", code, errCode(t, body))
	}
}

// --- 数据源独立实体端点(spec §2.4 数据源段) ---

func TestDataSourceCRUD_AndCodes(t *testing.T) {
	h, _ := newTestServer(t)

	// 合法 → 201 全量记录
	dsPayload := mustJSONFile(t, map[string]any{
		"name":   "冒烟数据源",
		"schema": map[string]any{"type": "object", "properties": map[string]any{"student": map[string]any{"type": "string"}}, "required": []string{"student"}},
		"data":   map[string]any{"student": "林晚晴"},
	})
	code, body := doRequest(t, h, http.MethodPost, "/api/datasources", "application/json", dsPayload)
	if code != http.StatusCreated {
		t.Fatalf("POST datasources status = %d, 期望 201(%v)", code, body)
	}
	if body["name"] != "冒烟数据源" || body["schema"] == nil || body["data"] == nil {
		t.Fatalf("201 应为全量记录: %v", body)
	}
	dsID := int64(body["id"].(float64))

	// PUT 更新 → 200 整存替换
	updPayload := mustJSONFile(t, map[string]any{
		"name":   "改名数据源",
		"schema": map[string]any{"type": "object"},
		"data":   map[string]any{"v": 1},
	})
	code, body = doRequest(t, h, http.MethodPut, fmt.Sprintf("/api/datasources/%d", dsID), "application/json", updPayload)
	if code != http.StatusOK {
		t.Fatalf("PUT datasources status = %d, 期望 200(%v)", code, body)
	}
	if body["name"] != "改名数据源" {
		t.Fatalf("name 应整存替换: %v", body["name"])
	}

	// 列表摘要四字段(含引用计数);seed 数据源同表在列,按 id 取自建项断言
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/datasources", nil))
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("列表响应应为数组: %v\n%s", err, rec.Body.String())
	}
	if len(list) < 1 {
		t.Fatalf("列表长度 = %d, 期望 ≥ 1", len(list))
	}
	var mine map[string]any
	for _, item := range list {
		if int64(item["id"].(float64)) == dsID {
			mine = item
		}
	}
	if mine == nil {
		t.Fatalf("列表应含自建数据源 %d: %v", dsID, list)
	}
	for _, key := range []string{"id", "name", "templateCount", "updatedAt"} {
		if _, ok := mine[key]; !ok {
			t.Fatalf("摘要应含 %s: %v", key, mine)
		}
	}
	if mine["templateCount"].(float64) != 0 {
		t.Fatalf("零引用 templateCount = %v, 期望 0", mine["templateCount"])
	}

	// 坏 schema → 400 schema_invalid;不过 schema → 400 dataset_schema_mismatch
	badCases := map[string]struct {
		payload  map[string]any
		wantCode string
	}{
		"schema 本身不合法": {
			map[string]any{"name": "x", "schema": 42, "data": map[string]any{}},
			"schema_invalid",
		},
		"schema 键缺失": {
			map[string]any{"name": "x", "data": map[string]any{}},
			"schema_invalid",
		},
		"data 不过 schema": {
			map[string]any{"name": "x", "schema": map[string]any{"type": "object"}, "data": []any{1, 2}},
			"dataset_schema_mismatch",
		},
		"data 键缺失": {
			map[string]any{"name": "x", "schema": map[string]any{"type": "object"}},
			"dataset_schema_mismatch",
		},
	}
	for name, tc := range badCases {
		code, body := doRequest(t, h, http.MethodPost, "/api/datasources", "application/json", mustJSONFile(t, tc.payload))
		if code != http.StatusBadRequest || errCode(t, body) != tc.wantCode {
			t.Fatalf("[%s] = %d/%s, 期望 400/%s", name, code, errCode(t, body), tc.wantCode)
		}
		code, body = doRequest(t, h, http.MethodPut, fmt.Sprintf("/api/datasources/%d", dsID), "application/json", mustJSONFile(t, tc.payload))
		if code != http.StatusBadRequest || errCode(t, body) != tc.wantCode {
			t.Fatalf("[PUT %s] = %d/%s, 期望 400/%s", name, code, errCode(t, body), tc.wantCode)
		}
	}

	// 寻址失败 → 404 data_source_not_found(含非整数 id)
	for _, path := range []string{"/api/datasources/999", "/api/datasources/abc"} {
		code, body := doRequest(t, h, http.MethodGet, path, "", nil)
		if code != http.StatusNotFound || errCode(t, body) != "data_source_not_found" {
			t.Fatalf("GET %s = %d/%s, 期望 404/data_source_not_found", path, code, errCode(t, body))
		}
	}
}

// TestBindTemplateDataSource 绑定通道:合法绑定/解绑/引用不存在/模板不存在
func TestBindTemplateDataSource(t *testing.T) {
	h, seedID := newTestServer(t)

	dsPayload := mustJSONFile(t, map[string]any{
		"name":   "绑定用数据源",
		"schema": map[string]any{"type": "object"},
		"data":   map[string]any{"k": "v"},
	})
	_, body := doRequest(t, h, http.MethodPost, "/api/datasources", "application/json", dsPayload)
	dsID := int64(body["id"].(float64))

	// 绑定 → 200 且引用列更新;模板内容不触碰
	bind := mustJSONFile(t, map[string]any{"dataSourceId": dsID})
	code, body := doRequest(t, h, http.MethodPut, fmt.Sprintf("/api/templates/%d/datasource", seedID), "application/json", bind)
	if code != http.StatusOK {
		t.Fatalf("绑定 status = %d, 期望 200(%v)", code, body)
	}
	if got, ok := body["dataSourceId"].(float64); !ok || int64(got) != dsID {
		t.Fatalf("绑定后 dataSourceId = %v, 期望 %d", body["dataSourceId"], dsID)
	}

	// 解绑:null
	unbind := mustJSONFile(t, map[string]any{"dataSourceId": nil})
	code, body = doRequest(t, h, http.MethodPut, fmt.Sprintf("/api/templates/%d/datasource", seedID), "application/json", unbind)
	if code != http.StatusOK || body["dataSourceId"] != nil {
		t.Fatalf("解绑 = %d/%v, 期望 200/null", code, body["dataSourceId"])
	}

	// 引用不存在 → 404 data_source_not_found(绑定态不被改写)
	badBind := mustJSONFile(t, map[string]any{"dataSourceId": 999})
	code, body = doRequest(t, h, http.MethodPut, fmt.Sprintf("/api/templates/%d/datasource", seedID), "application/json", badBind)
	if code != http.StatusNotFound || errCode(t, body) != "data_source_not_found" {
		t.Fatalf("绑定不存在 = %d/%s, 期望 404/data_source_not_found", code, errCode(t, body))
	}

	// 模板不存在 → 404 template_not_found
	code, body = doRequest(t, h, http.MethodPut, "/api/templates/999/datasource", "application/json", bind)
	if code != http.StatusNotFound || errCode(t, body) != "template_not_found" {
		t.Fatalf("模板寻址失败 = %d/%s, 期望 404/template_not_found", code, errCode(t, body))
	}
}

// TestPostTemplate_WithDataSourceId 创建即绑定(另存为单调用路径):引用随建落库;
// 引用不存在 → 404 data_source_not_found
func TestPostTemplate_WithDataSourceId(t *testing.T) {
	h, _ := newTestServer(t)
	dsPayload := mustJSONFile(t, map[string]any{
		"name":   "创建绑定用",
		"schema": map[string]any{"type": "object"},
		"data":   map[string]any{},
	})
	_, body := doRequest(t, h, http.MethodPost, "/api/datasources", "application/json", dsPayload)
	dsID := int64(body["id"].(float64))

	payload := mustJSONFile(t, map[string]any{
		"name":         "带绑定的模板",
		"canvases":     []any{map[string]any{"name": "单页", "graph": minimalGraph()}},
		"dataSourceId": dsID,
	})
	code, body := doRequest(t, h, http.MethodPost, "/api/templates", "application/json", payload)
	if code != http.StatusCreated {
		t.Fatalf("POST status = %d, 期望 201(%v)", code, body)
	}
	if got, ok := body["dataSourceId"].(float64); !ok || int64(got) != dsID {
		t.Fatalf("创建应随建绑定: %v, 期望 %d", body["dataSourceId"], dsID)
	}

	payload = mustJSONFile(t, map[string]any{
		"name":         "坏引用模板",
		"canvases":     []any{map[string]any{"name": "单页", "graph": minimalGraph()}},
		"dataSourceId": 999,
	})
	code, body = doRequest(t, h, http.MethodPost, "/api/templates", "application/json", payload)
	if code != http.StatusNotFound || errCode(t, body) != "data_source_not_found" {
		t.Fatalf("坏引用 = %d/%s, 期望 404/data_source_not_found", code, errCode(t, body))
	}
}

// --- POST /api/assets(spec §2.4 #8) ---

// multipartBody 构造 multipart/form-data 请求体与 Content-Type
func multipartBody(t *testing.T, fieldName, filename string, content []byte) (string, []byte) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if fieldName != "" {
		fw, err := mw.CreateFormFile(fieldName, filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return mw.FormDataContentType(), buf.Bytes()
}

// uploadAsset 上传并返回 (status, body);Chdir 到临时 CWD(运行约定 CWD=example/server:
// 落盘与 FileServer 都按 CWD 解析)。先装载 server(相对读 seed)再切 CWD
func uploadAsset(t *testing.T, fieldName, filename string, content []byte) (http.Handler, int, map[string]any) {
	t.Helper()
	h, _ := newTestServer(t)
	t.Chdir(t.TempDir())
	ctype, body := multipartBody(t, fieldName, filename, content)
	code, out := doRequest(t, h, http.MethodPost, "/api/assets", ctype, body)
	return h, code, out
}

func TestPostAssets_UploadRoundtrip(t *testing.T) {
	png := []byte("FAKEPNGBYTES-for-test")
	h, code, body := uploadAsset(t, "file", "photo.png", png)
	if code != http.StatusCreated {
		t.Fatalf("status = %d, 期望 201(%v)", code, body)
	}
	url, _ := body["url"].(string)
	if !regexp.MustCompile(`^/assets/u/[0-9a-f]{16}\.png$`).MatchString(url) {
		t.Fatalf("url 应匹配 ^/assets/u/[0-9a-f]{16}\\.png$: %q", url)
	}
	// 前导斜杠形态(spec §2.2)可直连 FileServer 取回原字节
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), png) {
		t.Fatalf("GET %s = %d %q, 期望 200 与原字节", url, rec.Code, rec.Body.String())
	}
}

func TestPostAssets_ExtensionWhitelist(t *testing.T) {
	cases := map[string]struct {
		filename string
		wantExt  string
	}{
		"无后缀落bin":   {"noext", ".bin"},
		"超长后缀落bin":  {"a.abcdefghijkl", ".bin"},
		"多段取最后一段":   {"archive.tar.gz", ".gz"},
		"大写保留":      {"PHOTO.PNG", ".PNG"},
		"非常规字符落bin": {"a.b#c", ".bin"},
		"恰好8位保留":    {"a.12345678", ".12345678"},
	}
	for name, tc := range cases {
		_, code, body := uploadAsset(t, "file", tc.filename, []byte("x"))
		if code != http.StatusCreated {
			t.Fatalf("[%s] status = %d, 期望 201(%v)", name, code, body)
		}
		url, _ := body["url"].(string)
		re := regexp.MustCompile(`^/assets/u/[0-9a-f]{16}` + regexp.QuoteMeta(tc.wantExt) + `$`)
		if !re.MatchString(url) {
			t.Fatalf("[%s] url = %q, 期望后缀 %s", name, url, tc.wantExt)
		}
	}
}

func TestPostAssets_MissingFieldAndNonMultipart(t *testing.T) {
	// multipart 但缺 file 字段
	h, code, body := uploadAsset(t, "other", "x.png", []byte("x"))
	if code != http.StatusBadRequest || errCode(t, body) != "invalid_json" {
		t.Fatalf("缺 file 字段 status/code = %d/%s, 期望 400/invalid_json", code, errCode(t, body))
	}

	// 非 multipart 请求体
	t.Chdir(t.TempDir())
	code, body = doRequest(t, h, http.MethodPost, "/api/assets", "text/plain", []byte("not multipart"))
	if code != http.StatusBadRequest || errCode(t, body) != "invalid_json" {
		t.Fatalf("非 multipart status/code = %d/%s, 期望 400/invalid_json", code, errCode(t, body))
	}
}

// --- 请求体上限与坏 body(spec §2.1,端到端承接 11 票中间件单测) ---

func TestPostTemplate_OverLimit413(t *testing.T) {
	h, _ := newTestServer(t)
	// 未闭合字符串使解码器读至体尽,超限由 MaxBytesError 触发(而非语法错先到)
	body := append([]byte(`{"name":"`), bytes.Repeat([]byte("a"), 10<<20+1)...)
	code, env := doRequest(t, h, http.MethodPost, "/api/templates", "application/json", body)
	if code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, 期望 413", code)
	}
	if got := errCode(t, env); got != "request_too_large" {
		t.Fatalf("code = %s, 期望 request_too_large", got)
	}
}

func TestPostTemplate_InvalidBody400(t *testing.T) {
	h, _ := newTestServer(t)
	code, env := doRequest(t, h, http.MethodPost, "/api/templates", "application/json", []byte("not json"))
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, 期望 400", code)
	}
	if got := errCode(t, env); got != "invalid_json" {
		t.Fatalf("code = %s, 期望 invalid_json", got)
	}
}
