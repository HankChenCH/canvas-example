// 模板写端点(spec §2.4 #3/#5):POST /api/templates 与 PUT /api/templates/{id}。
// 保存即预检(spec §3.4):页序列经编译边界全跑(页级形状 + 逐帧解码 + 流链
// 校验),不过 400 打回不落库。数据源为独立实体(spec §2.4 数据源段):POST 载荷
// 可选 dataSourceId 随建随绑(另存为单调用携带引用),PUT 不触碰绑定——绑定只经
// PUT …/datasource 通道。
package api

import (
	"errors"
	"net/http"

	"example/server/internal/codes"
	"example/server/internal/pages"
	"example/server/internal/store"
)

// templateWritePayload POST/PUT 模板载荷(spec §2.4 #3/#5,ADR 0025 页序列形状):
// pages = 单一页序列数组(页 id/name 由前端生成,服务端编译时逐项裁决);
// dataSourceId 仅 POST 消费(缺省 null = 未绑)——PUT 忽略此键,绑定只经数据源
// 绑定通道变更
type templateWritePayload struct {
	Name         string       `json:"name"`
	Pages        []pages.Page `json:"pages"`
	DataSourceID *int64       `json:"dataSourceId"`
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
		Pages:        payload.Pages,
		DataSourceID: payload.DataSourceID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, codes.InternalError, "写入模板失败")
		return
	}
	s.respondTemplate(w, r, http.StatusCreated, id)
}

// handleUpdateTemplate PUT /api/templates/{id} → 200 全量 TemplateRecord:
// name/pages 整存替换、不触碰 data_source_id、刷新 updatedAt(spec §2.4 #5)
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
		Name:  payload.Name,
		Pages: payload.Pages,
	}); err != nil {
		writeStoreError(w, err, "更新模板失败")
		return
	}
	s.respondTemplate(w, r, http.StatusOK, id)
}

// decodeTemplatePayload 解码载荷并跑保存预检(编译边界 pages.Compile,与渲染
// 管线同一段实现);失败已写响应,调用方拿到 ok=false 直接返回(400 稳定码信封,
// Compile 返回的 Code 原样上信封:page_invalid → 400 页级形状错,flow_chain_invalid
// 等引擎 sentinel → 400 流语义错,invalid_json/解码三码 → 400 wire/解码错)
func decodeTemplatePayload(w http.ResponseWriter, r *http.Request) (templateWritePayload, bool) {
	var payload templateWritePayload
	if err := decodeJSON(r, &payload); err != nil {
		writeDecodeError(w, err)
		return payload, false
	}
	if _, verr := pages.Compile(payload.Pages); verr != nil {
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
