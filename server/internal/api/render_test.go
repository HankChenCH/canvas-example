package api

import (
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

// newLocalRenderTemplate 造一个离线可渲染的模板(双帧模板表 + 默认链),
// 绑定 dataset 后返回模板 id
func newLocalRenderTemplate(t *testing.T, h http.Handler) int64 {
	t.Helper()
	payload := mustJSONFile(t, map[string]any{
		"name": "渲染用本地模板",
		"canvases": []any{
			map[string]any{"name": "主页", "graph": templateTableGraph()},
			map[string]any{"name": "续页", "graph": templateTableGraph()},
		},
		"flowChain": []any{
			map[string]any{"frame": 0, "mode": "fixed"},
			map[string]any{"frame": 1, "mode": "paged", "omitIfEmpty": true},
		},
	})
	code, body := doRequest(t, h, http.MethodPost, "/api/templates", "application/json", payload)
	if code != http.StatusCreated {
		t.Fatalf("造模板失败: %d %v", code, body)
	}
	ds := mustJSONFile(t, map[string]any{
		"schema": map[string]any{"type": "object"},
		"data":   map[string]any{"certificates": []any{map[string]any{"n": "x"}}},
	})
	code, body = doRequest(t, h, http.MethodPut, fmt.Sprintf("/api/templates/%v/dataset", body["id"]), "application/json", ds)
	if code != http.StatusOK {
		t.Fatalf("绑数据源失败: %d %v", code, body)
	}
	return int64(body["id"].(float64))
}

// TestPostRender_201SeedShape 渲染 → 201 RenderRecord:images 帧序×页序扁平、
// url 前导斜杠形态;逐 url GET 200 且 Content-Type image/png(spec §2.3/§2.4 #7)
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
		t.Fatalf("单帧空链渲染应 1 页: %v", body["images"])
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

	// 改模板(整存替换)后再渲染:新渲染记录新 id
	put := mustJSONFile(t, map[string]any{
		"name":     "改名后的渲染模板",
		"canvases": []any{map[string]any{"name": "主页", "graph": templateTableGraph()}},
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
