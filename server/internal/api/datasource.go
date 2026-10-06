// 数据源独立实体端点(spec §2.4 数据源段):GET/POST /api/datasources、
// GET/PUT /api/datasources/{id} 与绑定端点 PUT /api/templates/{id}/datasource。
// draft-07 完整校验权威在服务端(04 票锚点①):schema 本身不合法 → schema_invalid、
// data 不过 schema → dataset_schema_mismatch;模板对数据源只持引用,绑定/解绑
// 走模板侧绑定通道,数据源内容编辑影响所有引用它的模板。
package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"example/server/internal/codes"
	"example/server/internal/dataset"
	"example/server/internal/store"
)

// dataSourcePayload POST/PUT 数据源载荷:{name, schema, data}——整存替换语义
type dataSourcePayload struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Data   json.RawMessage `json:"data"`
}

// dataSourceBindPayload PUT /api/templates/{id}/datasource 载荷:
// {dataSourceId: number|null}——null = 解绑
type dataSourceBindPayload struct {
	DataSourceID *int64 `json:"dataSourceId"`
}

// validateDataSourcePayload draft-07 完整校验(内容三件套共用);失败已写响应
func (s *Server) validateDataSourcePayload(w http.ResponseWriter, payload dataSourcePayload) bool {
	if verr := dataset.Validate(payload.Schema, payload.Data); verr != nil {
		writeError(w, http.StatusBadRequest, verr.Code, verr.Message)
		return false
	}
	return true
}

// handleListDataSources GET /api/datasources → 200 摘要数组
// [{id,name,templateCount,updatedAt}],updatedAt 降序;空表也回 []
func (s *Server) handleListDataSources(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListDataSources(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, codes.InternalError, "列出数据源失败")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// handleCreateDataSource POST /api/datasources → 201 全量 DataSourceRecord
func (s *Server) handleCreateDataSource(w http.ResponseWriter, r *http.Request) {
	var payload dataSourcePayload
	if err := decodeJSON(r, &payload); err != nil {
		writeDecodeError(w, err)
		return
	}
	if !s.validateDataSourcePayload(w, payload) {
		return
	}
	id, err := s.store.CreateDataSource(r.Context(), store.DataSourceContent{
		Name:   payload.Name,
		Schema: payload.Schema,
		Data:   payload.Data,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, codes.InternalError, "写入数据源失败")
		return
	}
	s.respondDataSource(w, r, http.StatusCreated, id)
}

// handleGetDataSource GET /api/datasources/{id} → 200 全量 DataSourceRecord;
// id 非整数或不存在 → 404 data_source_not_found
func (s *Server) handleGetDataSource(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, codes.DataSourceNotFound, "数据源不存在")
		return
	}
	rec, err := s.store.GetDataSource(r.Context(), id)
	if err != nil {
		writeDataSourceStoreError(w, err, "读取数据源失败")
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// handleUpdateDataSource PUT /api/datasources/{id} → 200 全量 DataSourceRecord:
// name/schema/data 整存替换;通过 draft-07 校验后落库
func (s *Server) handleUpdateDataSource(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, codes.DataSourceNotFound, "数据源不存在")
		return
	}
	var payload dataSourcePayload
	if err := decodeJSON(r, &payload); err != nil {
		writeDecodeError(w, err)
		return
	}
	if !s.validateDataSourcePayload(w, payload) {
		return
	}
	if err := s.store.UpdateDataSource(r.Context(), id, store.DataSourceContent{
		Name:   payload.Name,
		Schema: payload.Schema,
		Data:   payload.Data,
	}); err != nil {
		writeDataSourceStoreError(w, err, "更新数据源失败")
		return
	}
	s.respondDataSource(w, r, http.StatusOK, id)
}

// handleBindTemplateDataSource PUT /api/templates/{id}/datasource → 200 更新后
// 全量 TemplateRecord:{dataSourceId} 引用列整存替换,null = 解绑;引用存在性
// 先验(数据源无 DELETE 端点,绑定后引用不悬空)
func (s *Server) handleBindTemplateDataSource(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, codes.TemplateNotFound, "模板不存在")
		return
	}
	var payload dataSourceBindPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeDecodeError(w, err)
		return
	}
	if payload.DataSourceID != nil {
		if _, err := s.store.GetDataSource(r.Context(), *payload.DataSourceID); err != nil {
			writeDataSourceStoreError(w, err, "绑定的数据源不存在")
			return
		}
	}
	if err := s.store.UpdateTemplateDataSource(r.Context(), id, payload.DataSourceID); err != nil {
		writeStoreError(w, err, "更新数据源绑定失败")
		return
	}
	s.respondTemplate(w, r, http.StatusOK, id)
}

// respondDataSource 数据源写端点统一出口:回读全量记录(POST 201 / PUT 200 同形)
func (s *Server) respondDataSource(w http.ResponseWriter, r *http.Request, status int, id int64) {
	rec, err := s.store.GetDataSource(r.Context(), id)
	if err != nil {
		writeDataSourceStoreError(w, err, "回读数据源失败")
		return
	}
	writeJSON(w, status, rec)
}

// writeDataSourceStoreError 数据源寻址失败 → 404 data_source_not_found,
// 其余归 internal_error
func writeDataSourceStoreError(w http.ResponseWriter, err error, fallback string) {
	if errors.Is(err, store.ErrDataSourceNotFound) {
		writeError(w, http.StatusNotFound, codes.DataSourceNotFound, "数据源不存在")
		return
	}
	writeError(w, http.StatusInternalServerError, codes.InternalError, fallback)
}
