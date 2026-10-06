// 模板写端点(spec §2.4 #3/#5):POST /api/templates 与 PUT /api/templates/{id}。
// 保存即预检(spec §3.4):逐帧解码 + 流链校验全跑,不过 400 打回不落库。
// 数据源为独立实体(spec §2.4 数据源段):POST 载荷可选 dataSourceId 随建随绑
// (另存为单调用携带引用),PUT 不触碰绑定——绑定只经 PUT …/datasource 通道。
package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"

	"example/server/internal/codes"
	"example/server/internal/preflight"
	"example/server/internal/store"
)

// templateWritePayload POST/PUT 模板载荷(spec §2.4 #3/#5):flowChain 缺键 =
// null = 空链(spec §3.1);dataSourceId 仅 POST 消费(缺省 null = 未绑)——
// PUT 忽略此键,绑定只经数据源绑定通道变更
type templateWritePayload struct {
	Name         string              `json:"name"`
	Canvases     []store.CanvasEntry `json:"canvases"`
	FlowChain    json.RawMessage     `json:"flowChain"`
	DataSourceID *int64              `json:"dataSourceId"`
}

// handleCreateTemplate POST /api/templates → 201 全量 TemplateRecord:id 自增
// 分配,createdAt = updatedAt(spec §2.4 #3);dataSourceId 引用存在性先验
func (s *Server) handleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	payload, ok := decodeTemplatePayload(w, r)
	if !ok {
		return
	}
	if payload.DataSourceID != nil {
		if _, err := s.store.GetDataSource(r.Context(), *payload.DataSourceID); err != nil {
			writeDataSourceStoreError(w, err, "绑定的数据源不存在")
			return
		}
	}
	id, err := s.store.CreateTemplate(r.Context(), store.TemplateContent{
		Name:         payload.Name,
		Canvases:     payload.Canvases,
		FlowChain:    normalizeNullJSON(payload.FlowChain),
		DataSourceID: payload.DataSourceID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, codes.InternalError, "写入模板失败")
		return
	}
	s.respondTemplate(w, r, http.StatusCreated, id)
}

// handleUpdateTemplate PUT /api/templates/{id} → 200 全量 TemplateRecord:
// name/canvases/flowChain 整存替换、不触碰 data_source_id、刷新 updatedAt
// (spec §2.4 #5)
func (s *Server) handleUpdateTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, codes.TemplateNotFound, "模板不存在")
		return
	}
	payload, ok := decodeTemplatePayload(w, r)
	if !ok {
		return
	}
	if err := s.store.UpdateTemplateContent(r.Context(), id, store.TemplateContent{
		Name:      payload.Name,
		Canvases:  payload.Canvases,
		FlowChain: normalizeNullJSON(payload.FlowChain),
	}); err != nil {
		writeStoreError(w, err, "更新模板失败")
		return
	}
	s.respondTemplate(w, r, http.StatusOK, id)
}

// decodeTemplatePayload 解码载荷并跑保存预检;失败已写响应,调用方拿到 ok=false
// 直接返回(预检失败 = 400 稳定码信封,spec §3.4)
func decodeTemplatePayload(w http.ResponseWriter, r *http.Request) (templateWritePayload, bool) {
	var payload templateWritePayload
	if err := decodeJSON(r, &payload); err != nil {
		writeDecodeError(w, err)
		return payload, false
	}
	graphs := make([]json.RawMessage, 0, len(payload.Canvases))
	for _, c := range payload.Canvases {
		graphs = append(graphs, c.Graph)
	}
	if verr := preflight.Check(graphs, payload.FlowChain); verr != nil {
		writeError(w, http.StatusBadRequest, verr.Code, verr.Message)
		return payload, false
	}
	return payload, true
}

// respondTemplate 写端点统一出口:回读全量记录(POST 201 / PUT 200 同形,spec §2.4)
func (s *Server) respondTemplate(w http.ResponseWriter, r *http.Request, status int, id int64) {
	rec, err := s.store.GetTemplate(r.Context(), id)
	if err != nil {
		writeStoreError(w, err, "回读模板失败")
		return
	}
	writeJSON(w, status, rec)
}

// writeStoreError 存储寻址失败 → 404,其余归 internal_error(spec §2.4 通用寻址)
func writeStoreError(w http.ResponseWriter, err error, fallback string) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, codes.TemplateNotFound, "模板不存在")
		return
	}
	writeError(w, http.StatusInternalServerError, codes.InternalError, fallback)
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
