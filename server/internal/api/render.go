// 渲染端点(spec §2.4 #7):POST /api/templates/{id}/render → 201 RenderRecord。
// 无 body 不读;渲染始终以服务端存储态为准(模板 + dataset,dataset null 按
// 未绑直通编译)。产物落盘 renders/<renderId>/<seq>.png + 落库,keep-all 不
// 清理——旧渲染文件即快照(spec §2.4 #7)。
package api

import (
	"context"
	"encoding/json"
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

// renderRecordResponse RenderRecord 响应形状(spec §2.3):url 为前导斜杠形态
// (§2.2),DB 存相对 path
type renderRecordResponse struct {
	ID         int64                 `json:"id"`
	TemplateID int64                 `json:"templateId"`
	CreatedAt  string                `json:"createdAt"`
	Images     []renderImageResponse `json:"images"`
}

type renderImageResponse struct {
	Frame int    `json:"frame"`
	Name  string `json:"name"`
	URL   string `json:"url"`
}

// handleRenderTemplate 渲染一跳:取存储态 → 解析数据源引用 → 管线(解码/编译/
// 逐页渲染)→ 落盘 → 落库 → 201。未绑数据源按未绑直通编译(spec §2.4 #7 直通
// 语义);错误映射全走 render 包对 §2.5/§2.6 的实现;断连取消不写响应仅日志
// (spec §2.5 渲染段)。
func (s *Server) handleRenderTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, codes.TemplateNotFound, "模板不存在")
		return
	}
	rec, err := s.store.GetTemplate(r.Context(), id)
	if err != nil {
		// 模板/数据源存取不设超时(spec §2.6),用 r.Context()
		writeStoreError(w, err, "读取模板失败")
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

	pages, perr := s.render.Run(ctx, rec, datasetJSON)
	if perr != nil {
		if perr.Silent {
			log.Printf("渲染模板 %d 取消(客户端断连): %s", id, perr.Message)
			return // 不写响应,仅日志(spec §2.5:render_cancelled)
		}
		writeError(w, perr.Status, perr.Code, perr.Message)
		return
	}

	record, err := s.store.CreateRenderRecord(r.Context(), id)
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
