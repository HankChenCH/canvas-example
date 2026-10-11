// 渲染端点(spec §2.4 #7 + 模板草稿-发布-版本化 spec §4):POST
// /api/templates/{id}/render → 201 RenderRecord。可选 JSON body 选版:
// {"version":N} 指定历史版本、"draft" 哨兵渲染草稿列现值(编辑器预览专用)、
// 缺 body/缺字段 = 最新已发布(明示 breaking change,由存量回填 v1 吸收)。
// 数据源 = 当前 data_source_id 绑定(绑定不快照,渲染 = 模板版本 × 数据源现值)。
// 产物落盘 renders/<renderId>/<seq>.png + 落库,keep-all 不清理——旧渲染文件即
// 快照(spec §2.4 #7)。
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"example/server/internal/codes"
	"example/server/internal/render"
	"example/server/internal/store"
)

// renderDeadline 渲染期 deadline(spec §2.6):handler 的 r.Context() 叠加
// 30s WithTimeout,到点 504 render_deadline_exceeded
const renderDeadline = 30 * time.Second

// 渲染选版 mode 三态常量(spec §4,票 03 Comments 契约修订 / 评审修复 F5 取态单点):
// latest = 缺省(最新已发布,原隐身于 switch default,显式化防取态漂移)/
// draft = 草稿哨兵(编辑器预览渲草稿列现值——本图定案前的原渲染语义显式化保留)/
// explicit = 显式指定历史版本号。三态同处声明,switch 显式分支,default 仅兜底
// 未知态(mode 只产自下述常量,理论不可达)
const (
	draftSentinel     = "draft"
	selectionLatest   = "latest"
	selectionExplicit = "explicit"
)

// writeInvalidJSON 400 invalid_json 信封单点(评审修复 F3 文件内收敛):选版解析
// 家族把坏 JSON/字段形态非法统一归此码(spec §4 哨兵复用裁决),message 按现场
// 承载具体拒因
func writeInvalidJSON(w http.ResponseWriter, message string) {
	writeError(w, http.StatusBadRequest, codes.InvalidJSON, message)
}

// renderRecordResponse RenderRecord 响应形状(spec §2.3):url 为前导斜杠形态
// (§2.2),DB 存相对 path;version null 透传(draft 渲染 / 版本化前旧行)
type renderRecordResponse struct {
	ID         int64                 `json:"id"`
	TemplateID int64                 `json:"templateId"`
	Version    *int64                `json:"version"`
	CreatedAt  string                `json:"createdAt"`
	Images     []renderImageResponse `json:"images"`
}

type renderImageResponse struct {
	Frame int    `json:"frame"`
	Name  string `json:"name"`
	URL   string `json:"url"`
}

// renderSelection 渲染选版解析产物(spec §4):mode 三态——latest(缺省,最新
// 已发布)/ draft(草稿列现值)/ explicit(指定历史版本号)
type renderSelection struct {
	mode    string
	version int64
}

// resolveRenderSelection 解析渲染端点可选 body:空 body(smoke 惯例
// --data-binary @/dev/null)或缺 version 字段 = 缺省最新已发布;正整数 = 指定
// 版本;"draft" = 草稿哨兵;null 视同缺省(null 归一先例);类型错/零/负数/
// 非整数/未知形态 → 400 invalid_json。失败已写响应,调用方拿到 ok=false 直接返回
func resolveRenderSelection(w http.ResponseWriter, r *http.Request) (renderSelection, bool) {
	def := renderSelection{mode: selectionLatest}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, codes.RequestTooLarge, "请求体超过 10MB 上限")
		} else {
			writeError(w, http.StatusInternalServerError, codes.InternalError, "读取请求体失败")
		}
		return def, false
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 { // 无 body = 缺省(smoke 全量无 body POST 同款)
		return def, true
	}
	var body struct {
		Version json.RawMessage `json:"version"`
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	if err := dec.Decode(&body); err != nil || dec.More() { // 严格单 JSON 值,坏 JSON 归 400
		writeError(w, http.StatusBadRequest, codes.InvalidJSON, "请求体不是合法 JSON")
		return def, false
	}
	field := bytes.TrimSpace(body.Version)
	if len(field) == 0 || bytes.Equal(field, []byte("null")) { // 缺字段 / null = 缺省
		return def, true
	}
	if bytes.Equal(field, []byte(`"`+draftSentinel+`"`)) {
		return renderSelection{mode: draftSentinel}, true
	}
	var num any
	numDec := json.NewDecoder(bytes.NewReader(field))
	numDec.UseNumber()
	if err := numDec.Decode(&num); err != nil || numDec.More() {
		writeInvalidJSON(w, "version 仅接受正整数或 \"draft\" 哨兵(spec §4)")
		return def, false
	}
	n, ok := num.(json.Number)
	if !ok {
		writeInvalidJSON(w, "version 仅接受正整数或 \"draft\" 哨兵(spec §4)")
		return def, false
	}
	version, err := n.Int64()
	if err != nil || version <= 0 {
		writeInvalidJSON(w, "version 仅接受正整数或 \"draft\" 哨兵(spec §4)")
		return def, false
	}
	return renderSelection{mode: selectionExplicit, version: version}, true
}

// versionedRunRecord 版本化路径的渲染输入(spec §4:版本行只快照文档整体,
// 数据源取当前绑定)——render.Run 仅消费 rec.Canvases/rec.FlowChain,零值壳
// 即可,不带 id/时间戳等无关字段
func versionedRunRecord(ver *store.TemplateVersionRecord) *store.TemplateRecord {
	return &store.TemplateRecord{Canvases: ver.Canvases, FlowChain: ver.FlowChain}
}

// handleRenderTemplate 渲染一跳:寻址模板 → 解析可选 body 选版 → 解析数据源
// 当前绑定 → 管线(解码/编译/逐页渲染)→ 落盘 → 落库 → 201。选版语义(spec §4):
// 缺省 = 最新已发布(从未发布 409 template_unpublished);显式版本不存在 → 404
// version_not_found;draft 哨兵 = 草稿列现值(渲染记 version null)。未绑数据源
// 按未绑直通编译(spec §2.4 #7 直通语义);错误映射全走 render 包对 §2.5/§2.6 的
// 实现;断连取消不写响应仅日志(spec §2.5 渲染段)。
func (s *Server) handleRenderTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, codes.TemplateNotFound, "模板不存在")
		return
	}
	selection, ok := resolveRenderSelection(w, r)
	if !ok {
		return
	}
	rec, err := s.store.GetTemplate(r.Context(), id)
	if err != nil {
		// 模板/数据源存取不设超时(spec §2.6),用 r.Context();先寻址模板——
		// 模板不存在 404 template_not_found 优先于选版语义
		writeStoreError(w, err, "读取模板失败")
		return
	}

	// 选版落位:版本化路径用版本行 canvases/flow_chain + 当前数据源绑定;
	// draft 哨兵用草稿列现值(rec 原样),渲染记 version NULL(未对应任何发布
	// 版本,诚实语义)。三态显式分支(latest 原隐身 default,评审修复 F5 显式化),
	// default 仅兜底未知态——mode 只产自上述三常量,理论不可达
	runRec := rec
	var version *int64
	switch selection.mode {
	case selectionLatest: // 缺省 = 最新已发布(明示 breaking change,spec §4)
		latest, ok, err := s.store.LatestTemplateVersion(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, codes.InternalError, "读取最新版本失败")
			return
		}
		if !ok {
			writeError(w, http.StatusConflict, codes.TemplateUnpublished, "模板尚未发布任何版本,无法渲染(可先 POST …/publish 或显式 {\"version\":\"draft\"})")
			return
		}
		ver, err := s.store.GetTemplateVersion(r.Context(), id, latest)
		if err != nil {
			writeError(w, http.StatusInternalServerError, codes.InternalError, "读取版本失败")
			return
		}
		runRec = versionedRunRecord(ver)
		version = &latest
	case draftSentinel: // 草稿列现值 = 原语义显式化
	case selectionExplicit:
		ver, err := s.store.GetTemplateVersion(r.Context(), id, selection.version)
		if err != nil {
			if errors.Is(err, store.ErrTemplateVersionNotFound) {
				writeError(w, http.StatusNotFound, codes.VersionNotFound, "版本不存在")
				return
			}
			writeError(w, http.StatusInternalServerError, codes.InternalError, "读取版本失败")
			return
		}
		runRec = versionedRunRecord(ver)
		v := selection.version
		version = &v
	default: // 兜底未知态(理论不可达):mode 三常量闭合,到此即内部不变量破坏
		writeError(w, http.StatusInternalServerError, codes.InternalError, "未知渲染选版形态")
		return
	}

	var datasetJSON json.RawMessage
	if rec.DataSourceID != nil {
		ds, err := s.store.GetDataSource(r.Context(), *rec.DataSourceID)
		if err != nil {
			// 绑定与渲染间隙的防御面:绑定端点先验存在、数据源删除有引用守卫
			// (被引用 409,spec §2.4 #6e),正常不可达
			writeDataSourceStoreError(w, err, "读取数据源失败")
			return
		}
		datasetJSON = ds.Data
	}

	ctx, cancel := context.WithTimeout(r.Context(), renderDeadline)
	defer cancel()

	pages, perr := s.render.Run(ctx, runRec, datasetJSON)
	if perr != nil {
		if perr.Silent {
			log.Printf("渲染模板 %d 取消(客户端断连): %s", id, perr.Message)
			return // 不写响应,仅日志(spec §2.5:render_cancelled)
		}
		writeError(w, perr.Status, perr.Code, perr.Message)
		return
	}

	record, err := s.store.CreateRenderRecord(r.Context(), id, version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codes.InternalError, "写入渲染记录失败")
		return
	}

	images, werr := render.WritePages(pages, record.ID)
	if werr != nil {
		// 回滚 stub 记录(产物半成品文件保留——无记录引用,无害孤儿;
		// AUTOINCREMENT 保证 id 不复用,不会与后续产物相撞)。
		// 用 Background:断连场景下 r.Context() 已取消,清理须照常完成
		if delErr := s.store.DeleteRenderRecord(context.Background(), record.ID); delErr != nil {
			log.Printf("回滚渲染 stub 记录 %d 失败: %v", record.ID, delErr)
		}
		writeError(w, werr.Status, werr.Code, werr.Message)
		return
	}

	if err := s.store.UpdateRenderImages(r.Context(), record.ID, images); err != nil {
		// 产物已落盘但记录不完整——keep-all 保留文件,响应以失败论
		log.Printf("渲染记录 %d 回填 images 失败(产物文件保留): %v", record.ID, err)
		writeError(w, http.StatusInternalServerError, codes.InternalError, "写入渲染记录失败")
		return
	}

	writeJSON(w, http.StatusCreated, renderRecordResponse{
		ID:         record.ID,
		TemplateID: record.TemplateID,
		Version:    record.Version,
		CreatedAt:  record.CreatedAt,
		Images:     toImageResponses(images),
	})
}

// toImageResponses DB 相对 path → 前导斜杠 url(spec §2.2 响应形态)
func toImageResponses(images []store.RenderImage) []renderImageResponse {
	out := make([]renderImageResponse, 0, len(images))
	for _, img := range images {
		out = append(out, renderImageResponse{Frame: img.Frame, Name: img.Name, URL: "/" + img.Path})
	}
	return out
}
