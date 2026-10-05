// Package api:HTTP 契约面(spec §2)。总则:JSON 前缀 /api、错误信封统一
// {"error":{code,message}}、请求体上限 10MB、静态前缀 FileServer 直服。
// 读路径(11 票)、模板/数据源/上传写路径(12 票)与渲染管线(13 票)同构,
// 错误码统一对 §2.5 总表。
package api

import (
	"encoding/json"
	"net/http"

	"example/server/internal/render"
	"example/server/internal/store"
)

// maxBodyBytes 请求体上限 10MB,全部 API 一体适用含 multipart(spec §2.1)
const maxBodyBytes = 10 << 20

// errDetail 错误信封内层:code 是程序判定依据,message 仅中文可读(spec §2.1)
type errDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Error errDetail `json:"error"`
}

// Server 组装路由;store 为唯一持久化依赖,render 承载渲染管线
type Server struct {
	store  *store.Store
	render *render.Service
}

// New 组装完整 HTTP handler:/api 路由 + /assets /renders FileServer 直服,
// 外层统一套请求体上限。运行约定 CWD = example/server,静态根按 CWD 解析。
func New(st *store.Store) http.Handler {
	return newServer(st, render.NewService())
}

// newServer 注入渲染服务(测试可替换管线依赖)
func newServer(st *store.Store, rs *render.Service) http.Handler {
	s := &Server{store: st, render: rs}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/templates", s.handleListTemplates)
	mux.HandleFunc("GET /api/templates/{id}", s.handleGetTemplate)
	mux.HandleFunc("POST /api/templates", s.handleCreateTemplate)
	mux.HandleFunc("PUT /api/templates/{id}", s.handleUpdateTemplate)
	mux.HandleFunc("PUT /api/templates/{id}/dataset", s.handleUpdateDataset)
	mux.HandleFunc("POST /api/assets", s.handleUploadAsset)
	mux.HandleFunc("POST /api/templates/{id}/render", s.handleRenderTemplate)
	// 静态前缀(spec §2.1):URL /assets/u/x.png ↔ 磁盘 assets/u/x.png(剥前缀即三合一的相对形态)
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))
	mux.Handle("/renders/", http.StripPrefix("/renders/", http.FileServer(http.Dir("renders"))))
	return withBodyLimit(mux)
}

// writeJSON 统一 JSON 响应出口(spec §2.1:application/json; charset=utf-8)
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError 统一错误信封出口;按 code 判定语义,message 不参与程序判定
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorEnvelope{Error: errDetail{Code: code, Message: message}})
}
