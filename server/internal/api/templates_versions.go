// 模板版本化端点(模板草稿-发布-版本化,工单 03,spec §3/§5):发布动作 +
// 版本史读家族 + 回滚。发布是动作不是资源创建,对齐 /render 动作式先例;
// 错误信封统一 writeError,错误码单源 codes 包(spec §2.5)。
//
// 契约要点:发布读的是已保存草稿(UI 编排层保证先促存再发,票 05);发布守门
// preflight 422 invalid_graph 不产生版本行;发布不动 updated_at(单义 = 草稿
// 内容最后编辑时间);回滚单事务原子复制回草稿并刷新 updated_at(与发布不动
// 形成语义对照);版本线线性,回滚永不产生新版本。
package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"example/server/internal/codes"
	"example/server/internal/preflight"
	"example/server/internal/store"
)

// publishResponse 发布响应(spec §3):201 新版本不带 duplicate 键(omitempty),
// 200 幂等跳过携带 duplicate:true——状态码本身已是主判定位,键只在 duplicate
// 时在场
type publishResponse struct {
	Version   int64  `json:"version"`
	CreatedAt string `json:"createdAt"`
	Duplicate bool   `json:"duplicate,omitempty"`
}

// handlePublishTemplate POST /api/templates/{id}/publish:读草稿列固化为新版本。
// 守门先行:草稿过 preflight.Check(解码 + 链校验,纯结构不吃数据集)——失败
// 422 invalid_graph(message 用 preflight 中文消息),不产生版本行;新版本 201
// {version,createdAt};草稿列文本与最新版本行字节相等 → 200 duplicate:true
// (幂等跳过,同编码路径字节比较确定)。updated_at 不动。寻址失败 404
// template_not_found
func (s *Server) handlePublishTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, codes.TemplateNotFound, "模板不存在")
		return
	}
	// 守门读全量草稿(仅取 canvases/flowChain 形状),字节面比较交给 store 层
	rec, err := s.store.GetTemplate(r.Context(), id)
	if err != nil {
		writeStoreError(w, err, "读取模板失败")
		return
	}
	graphs := make([]json.RawMessage, 0, len(rec.Canvases))
	for _, c := range rec.Canvases {
		graphs = append(graphs, c.Graph)
	}
	if verr := preflight.Check(graphs, rec.FlowChain); verr != nil {
		writeError(w, http.StatusUnprocessableEntity, codes.InvalidGraph, verr.Message)
		return
	}

	version, createdAt, duplicate, err := s.store.PublishTemplateVersion(r.Context(), id)
	if err != nil {
		writeStoreError(w, err, "发布模板失败")
		return
	}
	status := http.StatusCreated
	if duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, publishResponse{Version: version, CreatedAt: createdAt, Duplicate: duplicate})
}

// handleListTemplateVersions GET /api/templates/{id}/versions → [{version,createdAt}]
// 轻量列表(不吐 canvases 载荷),version 降序;空数组 = 未发布(永远是推导态,
// spec §3)。模板不存在 → 404 template_not_found(与 GetTemplate 惯例一致)
func (s *Server) handleListTemplateVersions(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, codes.TemplateNotFound, "模板不存在")
		return
	}
	items, err := s.store.ListTemplateVersions(r.Context(), id)
	if err != nil {
		writeStoreError(w, err, "读取版本列表失败")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// handleGetTemplateVersion GET /api/templates/{id}/versions/{version} → 单版本
// 全量 {version,canvases,flowChain,createdAt}(回滚与未来预览消费)。版本寻址
// 失败同码:路径 version 非整数、版本不存在、对无版本模板指定 → 404
// version_not_found(spec §4 同精神)
func (s *Server) handleGetTemplateVersion(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		// 版本寻址以版本为准:模板 id 非整数同归 version_not_found
		writeError(w, http.StatusNotFound, codes.VersionNotFound, "版本不存在")
		return
	}
	version, err := parseID(r.PathValue("version"))
	if err != nil {
		writeError(w, http.StatusNotFound, codes.VersionNotFound, "版本不存在")
		return
	}
	rec, err := s.store.GetTemplateVersion(r.Context(), id, version)
	if err != nil {
		if errors.Is(err, store.ErrTemplateVersionNotFound) {
			writeError(w, http.StatusNotFound, codes.VersionNotFound, "版本不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, codes.InternalError, "读取版本失败")
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// revertPayload 回滚载荷(spec §5):version 必需正整数——缺字段解码为 0、类型
// 错解码失败,连同坏 JSON 一并归 400 invalid_json
type revertPayload struct {
	Version int64 `json:"version"`
}

// handleRevertTemplate POST /api/templates/{id}/revert,body {version}:单事务
// 原子复制版本行内容回草稿列,200 回全量模板记录(与 GET /api/templates/{id}
// 同形——编辑器拿到即 reopenDocument,无需二次 GET)。updated_at 刷新。寻址
// 失败:模板不存在 404 template_not_found、版本不存在 404 version_not_found;
// 版本内容发布时已过 preflight,回滚不重验
func (s *Server) handleRevertTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, codes.TemplateNotFound, "模板不存在")
		return
	}
	var payload revertPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeDecodeError(w, err)
		return
	}
	if payload.Version <= 0 {
		writeError(w, http.StatusBadRequest, codes.InvalidJSON, "version 须为正整数(spec §5)")
		return
	}
	if err := s.store.RevertTemplateToVersion(r.Context(), id, payload.Version); err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, codes.TemplateNotFound, "模板不存在")
		case errors.Is(err, store.ErrTemplateVersionNotFound):
			writeError(w, http.StatusNotFound, codes.VersionNotFound, "版本不存在")
		default:
			writeError(w, http.StatusInternalServerError, codes.InternalError, "回滚模板失败")
		}
		return
	}
	s.respondTemplate(w, r, http.StatusOK, id)
}
