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
	// POST 载荷不含 dataset(spec §2.4 #3):两数据列 null、flowChain 缺省 null
	if body["dataset"] != nil || body["datasetSchema"] != nil || body["flowChain"] != nil {
		t.Fatalf("新模板 dataset/datasetSchema/flowChain 应为 null: %v", body)
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

// TestPutTemplate_PreservesDataset 票面第三条:先 PUT dataset 再 PUT 模板,验证仍在
func TestPutTemplate_PreservesDataset(t *testing.T) {
	h, seedID := newTestServer(t)
	path := fmt.Sprintf("/api/templates/%d", seedID)

	// 先 PUT dataset:合法 schema+data
	dsPayload := mustJSONFile(t, map[string]any{
		"schema": map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "string"}}, "required": []string{"a"}},
		"data":   map[string]any{"a": "值"},
	})
	code, body := doRequest(t, h, http.MethodPut, path+"/dataset", "application/json", dsPayload)
	if code != http.StatusOK {
		t.Fatalf("PUT dataset status = %d, 期望 200(%v)", code, body)
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
	if got, ok := body["dataset"].(map[string]any); !ok || got["a"] != "值" {
		t.Fatalf("dataset 不得被触碰(spec §2.4 #5): %v", body["dataset"])
	}
	if body["datasetSchema"] == nil {
		t.Fatal("datasetSchema 不得被触碰")
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

// --- PUT /api/templates/{id}/dataset(spec §2.4 #6) ---

func TestPutDataset_Codes(t *testing.T) {
	h, seedID := newTestServer(t)
	path := fmt.Sprintf("/api/templates/%d/dataset", seedID)

	// 合法 → 200 且两列整存替换
	dsPayload := mustJSONFile(t, map[string]any{
		"schema": map[string]any{"type": "object", "properties": map[string]any{"student": map[string]any{"type": "string"}}, "required": []string{"student"}},
		"data":   map[string]any{"student": "林晚晴"},
	})
	code, body := doRequest(t, h, http.MethodPut, path, "application/json", dsPayload)
	if code != http.StatusOK {
		t.Fatalf("合法 dataset status = %d, 期望 200(%v)", code, body)
	}
	data, ok := body["dataset"].(map[string]any)
	if !ok || data["student"] != "林晚晴" {
		t.Fatalf("dataset 应整存替换: %v", body["dataset"])
	}

	// 坏 schema → 400 schema_invalid;不过 schema → 400 dataset_schema_mismatch
	badCases := map[string]struct {
		payload  map[string]any
		wantCode string
	}{
		"schema 本身不合法": {
			map[string]any{"schema": 42, "data": map[string]any{}},
			"schema_invalid",
		},
		"schema 键缺失": {
			map[string]any{"data": map[string]any{}},
			"schema_invalid",
		},
		"data 不过 schema": {
			map[string]any{"schema": map[string]any{"type": "object"}, "data": []any{1, 2}},
			"dataset_schema_mismatch",
		},
		"data 键缺失": {
			map[string]any{"schema": map[string]any{"type": "object"}},
			"dataset_schema_mismatch",
		},
	}
	for name, tc := range badCases {
		code, body := doRequest(t, h, http.MethodPut, path, "application/json", mustJSONFile(t, tc.payload))
		if code != http.StatusBadRequest {
			t.Fatalf("[%s] status = %d, 期望 400", name, code)
		}
		if got := errCode(t, body); got != tc.wantCode {
			t.Fatalf("[%s] code = %s, 期望 %s", name, got, tc.wantCode)
		}
	}
}

func TestPutDataset_NotFound404(t *testing.T) {
	h, _ := newTestServer(t)
	payload := mustJSONFile(t, map[string]any{"schema": map[string]any{"type": "object"}, "data": map[string]any{}})
	code, body := doRequest(t, h, http.MethodPut, "/api/templates/999/dataset", "application/json", payload)
	if code != http.StatusNotFound || errCode(t, body) != "template_not_found" {
		t.Fatalf("status/code = %d/%s, 期望 404/template_not_found", code, errCode(t, body))
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
