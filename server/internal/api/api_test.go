package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example/server/internal/seed"
	"example/server/internal/store"
)

// seedDir seed 目录绝对路径:部分用例(assets 上传)会在测试中途 Chdir,
// 相对路径随之失效——包初始化时(尚在包目录)解析一次
var seedDir = func() string {
	abs, err := filepath.Abs(filepath.Join("..", "..", "seed"))
	if err != nil {
		panic(err)
	}
	return abs
}()

// newTestServer 真实 seed 装载(05 票 fixture)→ 播种 → api handler;
// 返回 handler 与种子模板 id(列表首项)
func newTestServer(t *testing.T) (http.Handler, int64) {
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
	if err := st.SeedTemplateIfEmpty(t.Context(), content); err != nil {
		t.Fatalf("播种: %v", err)
	}
	items, err := st.ListTemplates(t.Context())
	if err != nil || len(items) == 0 {
		t.Fatalf("取种子 id: %v", err)
	}
	return New(st), items[0].ID
}

// doJSON GET 请求并解析响应 JSON
func doJSON(t *testing.T, h http.Handler, path string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET %s 响应非 JSON: %v\n%s", path, err, rec.Body.String())
	}
	return rec.Code, body
}

func TestHealth(t *testing.T) {
	h, _ := newTestServer(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, 期望 200", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"status":"ok"}` {
		t.Fatalf("body = %s, 期望 {\"status\":\"ok\"}", got)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
}

func TestListTemplates_Seeded(t *testing.T) {
	h, seedID := newTestServer(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/templates", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, 期望 200", rec.Code)
	}
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("响应应为摘要数组: %v\n%s", err, rec.Body.String())
	}
	if len(list) < 1 {
		t.Fatal("播种后列表应非空(seed 保证,spec §5.2)")
	}
	for _, item := range list {
		keys := map[string]bool{}
		for k := range item {
			keys[k] = true
		}
		if len(keys) != 3 || !keys["id"] || !keys["name"] || !keys["updatedAt"] {
			t.Fatalf("摘要仅应含 id/name/updatedAt 三字段: %v", item)
		}
	}
	for i := 1; i < len(list); i++ {
		if list[i-1]["updatedAt"].(string) < list[i]["updatedAt"].(string) {
			t.Fatalf("updatedAt 应降序: %v", list)
		}
	}
	if list[0]["id"].(float64) != float64(seedID) {
		t.Fatalf("首项应为种子模板 id=%d, 实得 %v", seedID, list[0]["id"])
	}
}

func TestGetTemplate_Seeded(t *testing.T) {
	h, seedID := newTestServer(t)
	code, body := doJSON(t, h, fmt.Sprintf("/api/templates/%d", seedID))
	if code != http.StatusOK {
		t.Fatalf("status = %d, 期望 200", code)
	}
	canvases, ok := body["canvases"].([]any)
	if !ok || len(canvases) != 2 {
		t.Fatalf("canvases 应长 2(spec §5.1 两帧文档): %v", body["canvases"])
	}
	names := []string{}
	for _, c := range canvases {
		names = append(names, c.(map[string]any)["name"].(string))
	}
	if names[0] != "主页" || names[1] != "续页" {
		t.Fatalf("帧名应为主页/续页: %v", names)
	}
	chain, ok := body["flowChain"].([]any)
	if !ok || len(chain) != 2 {
		t.Fatalf("flowChain 应为两节点: %v", body["flowChain"])
	}
	if body["datasetSchema"] == nil || body["dataset"] == nil {
		t.Fatal("全量记录应含 datasetSchema 与 dataset(spec §2.4 #4)")
	}
	if body["id"].(float64) != float64(seedID) || body["name"] != seed.DefaultTemplateName {
		t.Fatalf("id/name 不符: %v / %v", body["id"], body["name"])
	}
	if _, ok := body["createdAt"].(string); !ok {
		t.Fatalf("createdAt 应为字符串: %v", body["createdAt"])
	}
}

func TestGetTemplate_AddressingFails(t *testing.T) {
	h, _ := newTestServer(t)
	for _, path := range []string{"/api/templates/999", "/api/templates/abc", "/api/templates/0"} {
		code, body := doJSON(t, h, path)
		if code != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, 期望 404", path, code)
		}
		errObj, ok := body["error"].(map[string]any)
		if !ok || errObj["code"] != "template_not_found" {
			t.Fatalf("GET %s 错误信封应为 template_not_found: %v", path, body)
		}
		if msg, ok := errObj["message"].(string); !ok || msg == "" {
			t.Fatalf("错误信封应含可读 message: %v", body)
		}
	}
}

// TestStaticFileServer /assets、/renders 由 FileServer 直服(spec §2.1);
// t.Chdir 到临时目录模拟 CWD=example/server 的运行约定
func TestStaticFileServer(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "static-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := New(st)

	root := t.TempDir()
	t.Chdir(root)
	if err := os.MkdirAll(filepath.Join(root, "assets", "u"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "u", "x.png"), []byte("PNGDATA"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/u/x.png", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "PNGDATA" {
		t.Fatalf("GET /assets/u/x.png = %d %q", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/renders/1/1.png", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("缺文件应 404, 实得 %d", rec.Code)
	}
}

// TestEmptyListReturnsArray 空表也回 [] 而非 null(spec §2.4 #2 响应为数组)
func TestEmptyListReturnsArray(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	rec := httptest.NewRecorder()
	New(st).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/templates", nil))
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Fatalf("空表响应 = %s, 期望 []", body)
	}
}
