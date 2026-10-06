package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- DELETE /api/templates/{id}(卡片操作修订,spec §2.4 #10) ---

// TestDeleteTemplate_204CascadeKeepFiles 删除 → 204 无响应体;模板行消失后
// GET/render 寻址 404;已落盘渲染产物照 keep-all 保留,直链仍 200 image/png;
// 再删同 id 与非整数 id → 404 template_not_found(通用寻址)
func TestDeleteTemplate_204CascadeKeepFiles(t *testing.T) {
	h, _ := newTestServer(t)
	t.Chdir(t.TempDir())
	id := newLocalRenderTemplate(t, h)

	// 先渲染一次:造渲染记录行 + 落盘产物(级联与保留断言面)
	_, rendered := postRender(t, h, fmt.Sprintf("%d", id))
	url := rendered["images"].([]any)[0].(map[string]any)["url"].(string)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/templates/%d", id), nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, 期望 204(body: %s)", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("204 响应体应为空: %q", rec.Body.String())
	}

	code, body := doRequest(t, h, http.MethodGet, fmt.Sprintf("/api/templates/%d", id), "", nil)
	if code != http.StatusNotFound || errCode(t, body) != "template_not_found" {
		t.Fatalf("GET 已删模板 = %d/%s, 期望 404/template_not_found", code, errCode(t, body))
	}
	code, body = postRender(t, h, fmt.Sprintf("%d", id))
	if code != http.StatusNotFound || errCode(t, body) != "template_not_found" {
		t.Fatalf("已删模板 render = %d/%s, 期望 404/template_not_found", code, errCode(t, body))
	}
	// 产物文件保留(keep-all 快照语义,spec §2.4 #10):直链仍服务
	rec = httptestGet(t, h, url)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("已删模板产物 %s 应仍服务: %d %q", url, rec.Code, rec.Header().Get("Content-Type"))
	}

	// 再删同 id / 非整数 id:通用寻址 404
	for _, path := range []string{fmt.Sprintf("/api/templates/%d", id), "/api/templates/abc"} {
		code, body := doRequest(t, h, http.MethodDelete, path, "", nil)
		if code != http.StatusNotFound || errCode(t, body) != "template_not_found" {
			t.Fatalf("DELETE %s = %d/%s, 期望 404/template_not_found", path, code, errCode(t, body))
		}
	}
}
