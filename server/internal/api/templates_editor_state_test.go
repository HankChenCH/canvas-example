// 工程数据通道端点测试(工程 spec §4):PUT /api/templates/{id}/editor-state
// 写分道 + GET 读合并 + 各写通道互不越界。
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// putEditorState PUT 快照并返回 (status, body)
func putEditorState(t *testing.T, h http.Handler, id int64, payload []byte) (int, map[string]any) {
	t.Helper()
	return doRequest(t, h, http.MethodPut, fmt.Sprintf("/api/templates/%d/editor-state", id), "application/json", payload)
}

// TestGetTemplate_EditorStateNullDefault 从未保存工程数据的模板 GET 响应带
// "editorState":null(全默认态,spec §4);seed 模板不补数据
func TestGetTemplate_EditorStateNullDefault(t *testing.T) {
	h, seedID := newTestServer(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/templates/%d", seedID), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, 期望 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"editorState":null`) {
		t.Fatalf("未保存状态应序列化 editorState: null: %s", rec.Body.String())
	}
}

// TestPutEditorState_Roundtrip 写分道往返:PUT 快照 → 200 全量记录含新状态
// (原样回显);GET 回读一致;载荷不落预检、不刷 updatedAt
func TestPutEditorState_Roundtrip(t *testing.T) {
	h, seedID := newTestServer(t)
	path := fmt.Sprintf("/api/templates/%d", seedID)
	_, before := doJSON(t, h, path)

	// 无多余空白的紧凑载荷,PUT 响应与 GET 回读均字节级含原样快照(编码器只
	// 压缩空白,不重排键序、不重塑值)
	snapshot := `{"schemaVersion":1,"frames":{"主页":{"guides":[{"id":"g1","orientation":"vertical","position":120}],"lockedPaths":["主页#标题"]}},"prefs":{"rulersVisible":true,"anchorExpanded":false}}`
	putRec := httptest.NewRecorder()
	h.ServeHTTP(putRec, httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/templates/%d/editor-state", seedID), strings.NewReader(snapshot)))
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT editor-state status = %d, 期望 200", putRec.Code)
	}
	if !strings.Contains(putRec.Body.String(), snapshot) {
		t.Fatalf("PUT 响应应原样回显快照字节: %s", putRec.Body.String())
	}

	// GET 回读一致
	getBody := getTemplateBody(t, h, seedID)
	if !strings.Contains(getBody, snapshot) {
		t.Fatalf("GET 回读应含原样快照字节: %s", getBody)
	}
	// 全量记录其余面不触碰
	_, after := doJSON(t, h, path)
	if after["name"] != before["name"] || after["updatedAt"] != before["updatedAt"] || after["createdAt"] != before["createdAt"] {
		t.Fatalf("状态写入不得触碰 name/时间戳: %v vs %v", after, before)
	}
	if len(after["pages"].([]any)) != len(before["pages"].([]any)) {
		t.Fatalf("状态写入不得触碰 pages: %v vs %v", after["pages"], before["pages"])
	}
}

// getTemplateBody GET 模板记录响应原文(字节级断言用,与 doJSON 的对象解码互补)
func getTemplateBody(t *testing.T, h http.Handler, id int64) string {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/templates/%d", id), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/templates/%d status = %d", id, rec.Code)
	}
	return rec.Body.String()
}

// TestPutEditorState_ZeroValidation 服务端零校验:非快照形状(标量/数组/未知
// 键深嵌/null 字面量)照收原样——宽容读权威在内核恢复端(spec §4)
func TestPutEditorState_ZeroValidation(t *testing.T) {
	h, seedID := newTestServer(t)
	cases := map[string]struct {
		payload string
		want    any
	}{
		"标量":      {`123`, float64(123)},
		"数组":      {`["x","y"]`, []any{"x", "y"}},
		"未知键深嵌":   {`{"unknown":{"deep":[1,2]}}`, map[string]any{"unknown": map[string]any{"deep": []any{float64(1), float64(2)}}}},
		"null字面量": {`null`, nil},
		"空对象":     {`{}`, map[string]any{}},
	}
	for name, tc := range cases {
		code, body := putEditorState(t, h, seedID, []byte(tc.payload))
		if code != http.StatusOK {
			t.Fatalf("[%s] status = %d, 期望 200(%v)", name, code, body)
		}
		if got := body["editorState"]; !jsonEqual(t, got, tc.want) {
			t.Fatalf("[%s] editorState = %v, 期望 %v", name, got, tc.want)
		}
	}
}

// jsonEqual JSON 值语义相等(经解码比对,规避 RawMessage 反序列化形态差)
func jsonEqual(t *testing.T, got, want any) bool {
	t.Helper()
	g, err := json.Marshal(got)
	if err != nil {
		return false
	}
	var gv, wv any
	if err := json.Unmarshal(g, &gv); err != nil {
		return false
	}
	wb, _ := json.Marshal(want)
	_ = json.Unmarshal(wb, &wv)
	gb, _ := json.Marshal(gv)
	wb2, _ := json.Marshal(wv)
	return string(gb) == string(wb2)
}

// TestPutEditorState_LWW 幂等整存替换:后写覆盖先写(单用户 LWW,明示接受)
func TestPutEditorState_LWW(t *testing.T) {
	h, seedID := newTestServer(t)
	first := `{"schemaVersion":1,"frames":{"主页":{"guides":[{"id":"g1","orientation":"horizontal","position":10}],"lockedPaths":[]}},"prefs":{}}`
	second := `{"schemaVersion":1,"frames":{},"prefs":{"rulersVisible":false}}`
	if code, body := putEditorState(t, h, seedID, []byte(first)); code != http.StatusOK {
		t.Fatalf("首写 status = %d(%v)", code, body)
	}
	code, body := putEditorState(t, h, seedID, []byte(second))
	if code != http.StatusOK {
		t.Fatalf("覆盖写 status = %d(%v)", code, body)
	}
	if got, _ := body["editorState"].(map[string]any); got == nil || len(got["frames"].(map[string]any)) != 0 {
		t.Fatalf("后写应整列覆盖先写: %v", body["editorState"])
	}
	if strings.Contains(getTemplateBody(t, h, seedID), `"position":10`) {
		t.Fatal("先写的 guides 应已被覆盖")
	}
}

// TestPutEditorState_AddressingFails 寻址失败 404 template_not_found(含非整数 id)
func TestPutEditorState_AddressingFails(t *testing.T) {
	h, _ := newTestServer(t)
	for _, path := range []string{"/api/templates/999/editor-state", "/api/templates/abc/editor-state", "/api/templates/0/editor-state"} {
		code, body := doRequest(t, h, http.MethodPut, path, "application/json", []byte(`{}`))
		if code != http.StatusNotFound || errCode(t, body) != "template_not_found" {
			t.Fatalf("PUT %s = %d/%s, 期望 404/template_not_found", path, code, errCode(t, body))
		}
	}
}

// TestPutEditorState_BadBody 坏 JSON → 400 invalid_json(通道只宽容快照形状,
// 不宽容传输层坏体)
func TestPutEditorState_BadBody(t *testing.T) {
	h, seedID := newTestServer(t)
	code, body := putEditorState(t, h, seedID, []byte("not json"))
	if code != http.StatusBadRequest || errCode(t, body) != "invalid_json" {
		t.Fatalf("status/code = %d/%s, 期望 400/invalid_json", code, errCode(t, body))
	}
}

// TestPutTemplate_PreservesEditorState 内容保存(PUT /api/templates/{id})不冲掉
// editor_state(写分道互不越界,spec §4)
func TestPutTemplate_PreservesEditorState(t *testing.T) {
	h, seedID := newTestServer(t)
	snapshot := `{"schemaVersion":1,"frames":{},"prefs":{"rulersVisible":true}}`
	if code, body := putEditorState(t, h, seedID, []byte(snapshot)); code != http.StatusOK {
		t.Fatalf("写状态 status = %d(%v)", code, body)
	}
	putPayload := mustJSONFile(t, map[string]any{
		"name":  "改名后的模板",
		"pages": []any{minimalPage("pg_new", "新页")},
	})
	code, body := doRequest(t, h, http.MethodPut, fmt.Sprintf("/api/templates/%d", seedID), "application/json", putPayload)
	if code != http.StatusOK {
		t.Fatalf("PUT template status = %d, 期望 200(%v)", code, body)
	}
	if body["name"] != "改名后的模板" {
		t.Fatalf("内容应整存替换: %v", body["name"])
	}
	if !strings.Contains(getTemplateBody(t, h, seedID), snapshot) {
		t.Fatalf("内容保存不得冲掉 editor_state: %v", body["editorState"])
	}
}

// TestPostTemplate_IgnoresEditorState 另存为不收 editorState:载荷携带也忽略,
// 新模板空态起步(editorState null,spec §4)
func TestPostTemplate_IgnoresEditorState(t *testing.T) {
	h, _ := newTestServer(t)
	payload := mustJSONFile(t, map[string]any{
		"name":  "另存为模板",
		"pages": []any{minimalPage("pg_saveas", "单页")},
		"editorState": map[string]any{
			"schemaVersion": 1,
			"frames": map[string]any{
				"主页": map[string]any{
					"guides":      []any{map[string]any{"id": "g1", "orientation": "vertical", "position": 5}},
					"lockedPaths": []any{},
				},
			},
			"prefs": map[string]any{"rulersVisible": true},
		},
	})
	code, body := doRequest(t, h, http.MethodPost, "/api/templates", "application/json", payload)
	if code != http.StatusCreated {
		t.Fatalf("POST status = %d, 期望 201(%v)", code, body)
	}
	if body["editorState"] != nil {
		t.Fatalf("另存为新模板应空态起步: %v", body["editorState"])
	}
}

// TestEditorStateWrites_LeaveListSummary 编辑器状态写分道不进列表摘要:拖参考线
// 不污染「最近编辑」排序,摘要保持 {id, name, updatedAt} 三字段(spec §4)
func TestEditorStateWrites_LeaveListSummary(t *testing.T) {
	h, seedID := newTestServer(t)
	_, before := doJSON(t, h, fmt.Sprintf("/api/templates/%d", seedID))
	snapshot := `{"schemaVersion":1,"frames":{},"prefs":{}}`
	if code, body := putEditorState(t, h, seedID, []byte(snapshot)); code != http.StatusOK {
		t.Fatalf("写状态 status = %d(%v)", code, body)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/templates", nil))
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("列表响应应为数组: %v\n%s", err, rec.Body.String())
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
	// 首项仍为种子模板(状态写入未刷 updatedAt,排序不动)
	if list[0]["id"].(float64) != float64(seedID) {
		t.Fatalf("排序不应被状态写入扰动: 首项 %v", list[0]["id"])
	}
	_, after := doJSON(t, h, fmt.Sprintf("/api/templates/%d", seedID))
	if after["updatedAt"] != before["updatedAt"] {
		t.Fatalf("状态写入不得刷新 updatedAt: %v vs %v", after["updatedAt"], before["updatedAt"])
	}
}
