package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- DELETE /api/datasources/{id}(28 票,spec §2.4 #6e) ---

// TestDeleteDataSource_204AndInUseGuard 删除 → 204 无响应体;被模板引用 →
// 409 data_source_in_use 且实体不动(引用不悬空,先解绑再删);解绑后可删,
// 行消失后 GET 404;再删同 id 与非整数 id → 404 data_source_not_found
func TestDeleteDataSource_204AndInUseGuard(t *testing.T) {
	h, seedID := newTestServer(t)

	// 建数据源并绑定 seed 模板 → 删除被 409 拒
	_, body := doRequest(t, h, http.MethodPost, "/api/datasources", "application/json", mustJSONFile(t, map[string]any{
		"name":   "删除用数据源",
		"schema": map[string]any{"type": "object"},
		"data":   map[string]any{"k": "v"},
	}))
	if body["id"] == nil {
		t.Fatalf("POST datasources 期望 201 全量记录: %v", body)
	}
	dsID := int64(body["id"].(float64))

	bind := mustJSONFile(t, map[string]any{"dataSourceId": dsID})
	code, _ := doRequest(t, h, http.MethodPut, fmt.Sprintf("/api/templates/%d/datasource", seedID), "application/json", bind)
	if code != http.StatusOK {
		t.Fatalf("绑定数据源 status = %d, 期望 200", code)
	}

	code, body = doRequest(t, h, http.MethodDelete, fmt.Sprintf("/api/datasources/%d", dsID), "", nil)
	if code != http.StatusConflict || errCode(t, body) != "data_source_in_use" {
		t.Fatalf("被引用 DELETE = %d/%s, 期望 409/data_source_in_use", code, errCode(t, body))
	}
	code, body = doRequest(t, h, http.MethodGet, fmt.Sprintf("/api/datasources/%d", dsID), "", nil)
	if code != http.StatusOK || body["name"] != "删除用数据源" {
		t.Fatalf("被拒删除后实体应原样在: %d/%v", code, body)
	}

	// 解绑 → 删除 204 无响应体 → GET 404
	unbind := mustJSONFile(t, map[string]any{"dataSourceId": nil})
	code, _ = doRequest(t, h, http.MethodPut, fmt.Sprintf("/api/templates/%d/datasource", seedID), "application/json", unbind)
	if code != http.StatusOK {
		t.Fatalf("解绑 status = %d, 期望 200", code)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/datasources/%d", dsID), nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, 期望 204(body: %s)", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("204 响应体应为空: %q", rec.Body.String())
	}
	code, body = doRequest(t, h, http.MethodGet, fmt.Sprintf("/api/datasources/%d", dsID), "", nil)
	if code != http.StatusNotFound || errCode(t, body) != "data_source_not_found" {
		t.Fatalf("GET 已删数据源 = %d/%s, 期望 404/data_source_not_found", code, errCode(t, body))
	}

	// 再删同 id / 非整数 id:通用寻址 404
	for _, path := range []string{fmt.Sprintf("/api/datasources/%d", dsID), "/api/datasources/abc"} {
		code, body := doRequest(t, h, http.MethodDelete, path, "", nil)
		if code != http.StatusNotFound || errCode(t, body) != "data_source_not_found" {
			t.Fatalf("DELETE %s = %d/%s, 期望 404/data_source_not_found", path, code, errCode(t, body))
		}
	}
}
