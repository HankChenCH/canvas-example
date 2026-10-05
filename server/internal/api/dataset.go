// 数据源写端点(spec §2.4 #6):PUT /api/templates/{id}/dataset。
// draft-07 完整校验权威在服务端(04 票锚点①):schema 本身不合法 → schema_invalid、
// data 不过 schema → dataset_schema_mismatch,通过则整存替换两列并刷新 updatedAt。
package api

import (
	"encoding/json"
	"net/http"

	"example/server/internal/codes"
	"example/server/internal/dataset"
)

// datasetPayload PUT …/dataset 载荷(spec §2.4 #6):{schema, data}
type datasetPayload struct {
	Schema json.RawMessage `json:"schema"`
	Data   json.RawMessage `json:"data"`
}

// handleUpdateDataset PUT /api/templates/{id}/dataset → 200 更新后全量 TemplateRecord
func (s *Server) handleUpdateDataset(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, codes.TemplateNotFound, "模板不存在")
		return
	}
	var payload datasetPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeDecodeError(w, err)
		return
	}
	if verr := dataset.Validate(payload.Schema, payload.Data); verr != nil {
		writeError(w, http.StatusBadRequest, verr.Code, verr.Message)
		return
	}
	if err := s.store.UpdateDataset(r.Context(), id, payload.Schema, payload.Data); err != nil {
		writeStoreError(w, err, "更新数据源失败")
		return
	}
	s.respondTemplate(w, r, http.StatusOK, id)
}
