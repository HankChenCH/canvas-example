package api

import (
	"fmt"
	"net/http"
	"strconv"
)

// handleHealth GET /api/health → 200 {"status":"ok"}(spec §2.4 #1)
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleListTemplates GET /api/templates → 摘要数组 [{id,name,updatedAt}],
// updatedAt 降序(spec §2.4 #2);播种保证非空(spec §5.2),空表也回 []
func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListTemplates(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternalError, "列出模板失败")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// handleGetTemplate GET /api/templates/{id} → 全量记录(含 datasetSchema/
// dataset,spec §2.4 #4);id 非整数或不存在 → 404 template_not_found
// (spec §2.4 通用寻址)
func (s *Server) handleGetTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, codeTemplateNotFound, "模板不存在")
		return
	}
	rec, err := s.store.GetTemplate(r.Context(), id)
	if err != nil {
		writeStoreError(w, err, "读取模板失败")
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// parseID 通用寻址:正整数才成立;非整数归寻址失败(spec §2.1 id 为正整数、
// §2.4 非整数或不存在 → 404)
func parseID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("非法 id %q", raw)
	}
	return id, nil
}
