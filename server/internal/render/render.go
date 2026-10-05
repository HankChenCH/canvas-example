// Package render:文档级渲染管线(13 票,spec §2.4 #7/§2.7)——存储态模板与
// dataset 逐帧解码 → 文档编译(流链配当)→ 逐页位图渲染 → PNG 字节,产物
// 落盘 renders/<renderId>/<seq>.png。错误按 §2.5 总表映射 HTTP,超时/取消/
// 下载流超时的分流语义见 §2.6(先判 ctx 归因再判资源码)。
package render

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"net/http"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/hankchen/go-canvas/canvas"
	"github.com/hankchen/go-canvas/hydrate"
	"github.com/hankchen/go-canvas/image-renderer"
	"github.com/hankchen/go-canvas/layer"
	"github.com/hankchen/go-canvas/paginate"
	"github.com/hankchen/go-canvas/renderer"
	"github.com/hankchen/go-canvas/resolver"

	"example/server/internal/codes"
	"example/server/internal/preflight"
	"example/server/internal/store"
)

// Service 渲染管线服务。可共享面(spec §2.7):物化器(磁盘缓存、http.Client);
// 渲染器实例有状态,每请求经 Run 现建、请求内逐页顺序复用,跨请求不复用。
type Service struct {
	rs *resolver.ResourceResolver
}

// NewService 组装生产管线(spec §2.7 组装基线)
func NewService() *Service {
	return &Service{rs: newDefaultResolver()}
}

// newDefaultResolver 物化器组装基线(02 票/spec §2.7):缓存根显式 .cache/
// (相对 CWD;容器内 UserCacheDir 不可用/不可写风险),下载器 10s 超时;经
// NewDefaultResolver 保留 QR 缝默认接线(后置 opts 覆写默认项不丢 QR)
func newDefaultResolver(opts ...resolver.Option) *resolver.ResourceResolver {
	base := []resolver.Option{
		resolver.WithCacheRoot(".cache/"),
		resolver.WithDownloader(&resolver.HTTPDownloader{Client: &http.Client{Timeout: 10 * time.Second}}),
	}
	return imagerenderer.NewDefaultResolver(append(base, opts...)...)
}

// Page 单页渲染产物:源帧下标 + 帧名 + PNG 字节(序列 = CompileResult 页序
// 扁平序,spec §2.3)
type Page struct {
	Frame int
	Name  string
	PNG   []byte
}

// PipelineErr 管线失败:已按 spec §2.5/§2.6 映射的 HTTP 状态与稳定 code;
// Silent = 断连取消,不写响应仅日志(spec §2.5 渲染段)
type PipelineErr struct {
	Status  int
	Code    string
	Message string
	Silent  bool
}

func (e *PipelineErr) Error() string { return e.Code + ": " + e.Message }

// Run 渲染一跳(spec §2.4 #7):解码(防御性复跑保存预检段,spec §3.4)→
// 文档编译 → 逐页渲染为 PNG。dataset 为 null 按未绑直通编译,零行空壳页合法。
// ctx 携带渲染期 30s deadline(handler 叠加,spec §2.6),穿引到绘制检查点与
// 物化下载;编译为纯结构计算不设检查点。
func (s *Service) Run(ctx context.Context, rec *store.TemplateRecord) ([]Page, *PipelineErr) {
	graphs := make([]json.RawMessage, 0, len(rec.Canvases))
	for _, c := range rec.Canvases {
		graphs = append(graphs, c.Graph)
	}
	// 防御性复跑解码 + 链校验,帧序列与流链直接复用预检产物(spec §3.4 同一管线)
	frames, chain, verr := preflight.DecodeAndValidate(graphs, rec.FlowChain)
	if verr != nil {
		return nil, &PipelineErr{Status: http.StatusBadRequest, Code: verr.Code, Message: verr.Message}
	}

	dataset, jerr := decodeDataset(rec.Dataset)
	if jerr != nil {
		return nil, &PipelineErr{Status: http.StatusBadRequest, Code: codes.InvalidJSON, Message: jerr.Error()}
	}

	// 文档编译(spec §2.7:nil evaluator = 默认受限插值求值器)
	compiler := paginate.NewDocumentCompiler(nil)
	result, err := compiler.Compile(frames, dataset, chain)
	if err != nil {
		return nil, mapPipelineErr(ctx, err)
	}

	frameByPage, perr := attributePages(ctx, compiler, frames, dataset, chain, len(result.Canvases))
	if perr != nil {
		return nil, perr
	}

	names := make([]string, len(rec.Canvases))
	for i, c := range rec.Canvases {
		names[i] = c.Name
	}
	return s.renderPages(ctx, names, result.Canvases, frameByPage)
}

// decodeDataset 存储态 dataset 解码:null/缺省 = 未绑(Hydrate 恒等直通,
// spec §2.4 #7 直通语义),否则经 JSON 语义读入(Hydrate 内部还会归一)
func decodeDataset(raw json.RawMessage) (any, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	var dataset any
	if err := json.Unmarshal(trimmed, &dataset); err != nil {
		return nil, fmt.Errorf("存储态 dataset 不是合法 JSON: %w", err)
	}
	return dataset, nil
}

// renderPages 逐页渲染(spec §2.7):每请求一个 Renderer 请求内顺序复用
// (Begin 每页新分配 surface,faces 为实例内缓存;实例非并发安全,跨请求不复用)
func (s *Service) renderPages(ctx context.Context, names []string, pages []*canvas.Canvas, frameByPage []int) ([]Page, *PipelineErr) {
	backend := imagerenderer.NewRenderer(s.rs)
	out := make([]Page, 0, len(pages))
	for i, page := range pages {
		img, err := renderer.RenderAs[*image.NRGBA](ctx, backend, page)
		if err != nil {
			return nil, mapPipelineErr(ctx, err)
		}
		png, err := imagerenderer.PNGBytes(img)
		if err != nil {
			return nil, mapPipelineErr(ctx, err)
		}
		out = append(out, Page{Frame: frameByPage[i], Name: names[frameByPage[i]], PNG: png})
	}
	return out, nil
}

// attributePages 页序列的源帧归属(spec §2.3 images[].frame = 源帧下标):
// CompileResult 页序列按帧序×页序扁平但不带源帧标识。go-canvas 零改动红线内
// 以「链前缀重放」测得每帧页数,全程只调真实 DocumentCompiler、不复制管线数学:
//
//   - 前缀链 C[:j+1] 在截断帧集 F[:c_j+1] 上对帧 0..c_j 的页贡献与全链运行逐一
//     相同——流分配按序推进、与后继节点无关;帧下标严格递增(链校验保证)使
//     前缀链引用的帧都在截断帧集内;
//   - 故 pages(c_j) = |run_j| − |run_{j−1}| − (链间链外帧数),其中链外帧经分页
//     关 Paginate 恒返单元素,每帧恰 1 页;
//   - paged 链尾吸收余量(多页切分全权交分页器,页数不预设)。
//
// 主链运行已通过后前缀重放理论不可失败;Σ 页数与产物长度不符属防御性兜底
// (internal_error)。
func attributePages(ctx context.Context, compiler *paginate.DocumentCompiler, frames []*canvas.Canvas, dataset any, chain []paginate.FlowChainNode, total int) ([]int, *PipelineErr) {
	counts := make([]int, len(frames))
	if len(chain) == 0 {
		// 空链 = 纯文档管线:逐帧分页关直通,各帧恰 1 页(spec §10.3)
		for i := range counts {
			counts[i] = 1
		}
	} else {
		onChain := make([]bool, len(frames))
		for _, n := range chain {
			onChain[n.Frame] = true
		}
		for f := range counts {
			if !onChain[f] {
				counts[f] = 1
			}
		}
		prev, prevCut := 0, -1
		for j := range chain {
			cut := chain[j].Frame
			run, err := compiler.Compile(frames[:cut+1], dataset, chain[:j+1])
			if err != nil {
				return nil, mapPipelineErr(ctx, err)
			}
			counts[cut] = len(run.Canvases) - prev - (cut - prevCut - 1)
			prev, prevCut = len(run.Canvases), cut
		}
	}

	sum := 0
	for _, n := range counts {
		sum += n
	}
	if sum != total {
		return nil, &PipelineErr{Status: http.StatusInternalServerError, Code: codes.InternalError,
			Message: fmt.Sprintf("帧归属页数合计 %d 与产物页数 %d 不符", sum, total)}
	}

	frameByPage := make([]int, 0, total)
	for f, n := range counts {
		for k := 0; k < n; k++ {
			frameByPage = append(frameByPage, f)
		}
	}
	return frameByPage, nil
}

// renderOutputDir 产物目录(spec §2.2 三类资源落位):CWD 相对
const renderOutputDir = "renders"

// WritePages 产物落盘 renders/<renderID>/<seq>.png(seq 1 起,spec §2.4 #7)。
// Path 为磁盘相对形态(DB 存储,§2.2);任一页写失败即中止——失败记录行由
// 调用方回滚,已写文件不清理(keep-all 不清理语义;无记录引用,无害孤儿)
func WritePages(pages []Page, renderID int64) ([]store.RenderImage, *PipelineErr) {
	dir := path.Join(renderOutputDir, strconv.FormatInt(renderID, 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, outputWriteFailed(fmt.Sprintf("创建产物目录 %s: %v", dir, err))
	}
	images := make([]store.RenderImage, 0, len(pages))
	for i, page := range pages {
		p := path.Join(dir, fmt.Sprintf("%d.png", i+1))
		if err := os.WriteFile(p, page.PNG, 0o644); err != nil {
			return nil, outputWriteFailed(fmt.Sprintf("写入产物 %s: %v", p, err))
		}
		images = append(images, store.RenderImage{Frame: page.Frame, Name: page.Name, Path: p})
	}
	return images, nil
}

// outputWriteFailed 产物落盘失败(spec §2.5 服务端自有码,500)
func outputWriteFailed(message string) *PipelineErr {
	return &PipelineErr{Status: http.StatusInternalServerError, Code: codes.RenderOutputWriteFailed, Message: message}
}

// mapPipelineErr 渲染期错误 → HTTP 映射(spec §2.5 总表 + §2.6 分流)。
// 先判 ctx 归因再判资源码(spec §2.6):渲染 ctx 自身到点/断连是一切归因的
// 第一优先;实测 http.Client{Timeout} 自身超时的错误链同样含
// context.DeadlineExceeded,且 resolver 已把它预归因为渲染 deadline code
// (resolver.cachedRemoteFile 的 ctxAttribution)——渲染 ctx 仍活时链上的
// 渲染 deadline code 只能来自下载流自身 10s 超时,按 §2.6 归资源码 502。
func mapPipelineErr(ctx context.Context, err error) *PipelineErr {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return &PipelineErr{Status: http.StatusGatewayTimeout, Code: codes.RenderDeadlineExceeded,
			Message: fmt.Sprintf("渲染超过 30s 截止时间: %v", err)}
	case errors.Is(ctx.Err(), context.Canceled):
		return &PipelineErr{Silent: true, Code: codes.RenderCancelled,
			Message: fmt.Sprintf("客户端断连,渲染取消: %v", err)}
	}
	switch {
	case errors.Is(err, resolver.ErrRenderCancelled):
		return &PipelineErr{Silent: true, Code: codes.RenderCancelled, Message: fmt.Sprintf("客户端断连,渲染取消: %v", err)}
	case errors.Is(err, resolver.ErrRenderDeadlineExceeded):
		// 渲染 ctx 仍活:deadline 来自下载流自身 10s 超时(spec §2.6 → 502)
		return &PipelineErr{Status: http.StatusBadGateway, Code: codes.ResourceDownloadFailed,
			Message: fmt.Sprintf("物化下载失败(下载流自身超时): %v", err)}
	case errors.Is(err, resolver.ErrResourceDownloadFailed):
		return &PipelineErr{Status: http.StatusBadGateway, Code: codes.ResourceDownloadFailed,
			Message: fmt.Sprintf("物化下载失败: %v", err)}
	case errors.Is(err, resolver.ErrResourceSaveFailed):
		return &PipelineErr{Status: http.StatusInternalServerError, Code: codes.ResourceSaveFailed,
			Message: fmt.Sprintf("物化产物落盘失败: %v", err)}
	case errors.Is(err, resolver.ErrQRGenerateFailed):
		return &PipelineErr{Status: http.StatusInternalServerError, Code: codes.QRGenerateFailed,
			Message: fmt.Sprintf("二维码生成失败: %v", err)}
	// 解码(FromGraph,spec §2.5 三码;正常由 preflight.DecodeAndValidate 先行
	// 对号,此处覆盖 Compile 内部 FromGraph 重建路径的防御性复现)
	case errors.Is(err, layer.ErrUnknownLayerType):
		return pipeline400(err, codes.UnknownLayerType)
	case errors.Is(err, layer.ErrTemplateRowsConflict):
		return pipeline400(err, codes.TemplateRowsConflict)
	case errors.Is(err, layer.ErrRowsPathMissing):
		return pipeline400(err, codes.RowsPathMissing)
	// 填充(hydrate,spec §2.5 六码)
	case errors.Is(err, hydrate.ErrRowsPathInvalid):
		return pipeline400(err, codes.RowsPathInvalid)
	case errors.Is(err, hydrate.ErrExpressionRowOutsideLoop):
		return pipeline400(err, codes.ExpressionRowOutsideLoop)
	case errors.Is(err, hydrate.ErrReservedRootKey):
		return pipeline400(err, codes.ReservedRootKey)
	case errors.Is(err, hydrate.ErrExpressionEmptyResource):
		return pipeline400(err, codes.ExpressionEmptyResource)
	case errors.Is(err, hydrate.ErrExpressionTypeMismatch):
		return pipeline400(err, codes.ExpressionTypeMismatch)
	case errors.Is(err, hydrate.ErrExpressionSyntaxError):
		return pipeline400(err, codes.ExpressionSyntaxError)
	// 文档编译(链校验 + 流分配 + 分页;分配期超容复用 paginate 段码)
	case errors.Is(err, paginate.ErrFlowChainInvalid):
		return pipeline400(err, codes.FlowChainInvalid)
	case errors.Is(err, paginate.ErrFlowRowsPathInconsistent):
		return pipeline400(err, codes.FlowRowsPathInconsistent)
	case errors.Is(err, paginate.ErrPaginateTargetInvalid):
		return pipeline400(err, codes.PaginateTargetInvalid)
	case errors.Is(err, paginate.ErrContentOverflow):
		return pipeline400(err, codes.ContentOverflow)
	}
	// 兜底(spec §2.5):frames_empty 等调用方编程错误、绘制原语内部错误
	// (颜色解析/图片解码/字体加载/渲染面尺寸非法)、ErrQRMaterializerRequired
	// 组装错误、wire 病态形态等 → 500 internal_error
	return &PipelineErr{Status: http.StatusInternalServerError, Code: codes.InternalError,
		Message: fmt.Sprintf("渲染内部错误: %v", err)}
}

// pipeline400 编译期稳定码错误(spec §2.5 解码/填充/编译段一律 400)
func pipeline400(err error, code string) *PipelineErr {
	return &PipelineErr{Status: http.StatusBadRequest, Code: code, Message: err.Error()}
}
