// 工程数据通道端点(工程 spec §4):PUT /api/templates/{id}/editor-state 写分道,
// 读合并复用 GET /api/templates/{id} 全量记录。通道语义完整复刻 datasource 绑定
// 先例(写分道读合并):载荷 = 快照原样、服务端零校验——pages 走编译预检是因渲染
// 管线消费它,editor_state 唯一消费方是内核恢复端,宽容读权威在那边;不触碰
// name/pages/data_source_id/updated_at(拖参考线不污染「最近编辑」排序、
// 不过页序列「保存即预检」)。
package api

import (
	"bytes"
	"encoding/json"
	"net/http"

	"example/server/internal/codes"
)

// handleUpdateTemplateEditorState PUT /api/templates/{id}/editor-state → 200 全量
// TemplateRecord(含新状态):载荷原样整存替换、幂等,并发即 last-write-wins
// (单用户应用,明示接受);null 字面量归 SQL NULL(响应序列化两者同为 JSON null)。
// 寻址失败 404 template_not_found
func (s *Server) handleUpdateTemplateEditorState(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, codes.TemplateNotFound, "模板不存在")
		return
	}
	var payload json.RawMessage
	if err := decodeJSON(r, &payload); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := s.store.UpdateTemplateEditorState(r.Context(), id, normalizeNullJSON(payload)); err != nil {
		writeStoreError(w, err, "更新工程数据失败")
		return
	}
	s.respondTemplate(w, r, http.StatusOK, id)
}

// normalizeNullJSON null 字面量与空载荷归 nil——存储 SQL NULL 而非字面 "null"
// 文本(响应序列化两者同为 JSON null,spec §2.3)
func normalizeNullJSON(raw json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	return raw
}
