// Package pages:模板文档单元(页序列)的编译边界(ADR 0025)。模板内容 = 页序列,
// 页 = 画布(wire graph)+ 可选行分配策略(flow,键面镜像引擎链节点去 frame)+
// 编辑层 id + 可选 name;一页恰承载一帧。Compile 把页序列机械降维回引擎管线的
// 「帧序列 + 流链节点数组」(一次解码产出全部产物,渲染管线零二次解码):页级
// 形状错归服务端自有码 page_invalid,
// flow 语义错原样透传引擎流链校验 sentinel(本包不复刻/抢判任何流校验规则),
// graph 非法 JSON 与解码失败沿用既有 code 分流。保存预检与渲染管线两处复用
// 同一实现(编译 + 校验一体,返回带 code 的错误),wire 契约与三端语义面零改动。
package pages

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/paginate"

	"example/server/internal/codes"
)

// Page 页(ADR 0025 页形状契约)。四字段全用 json.RawMessage 承接原文:
// graph/flow 本就以原文透传(产物不做反解重组,防丢未知键);id/name 用原文
// 是为了把「缺失/null/空/类型错」留给编译期逐项裁决 page_invalid——若声明为
// string,类型错会在载荷解码层提前变 invalid_json,形状错与语义错的分流就断了。
type Page struct {
	ID    json.RawMessage `json:"id"`
	Name  json.RawMessage `json:"name,omitempty"`
	Graph json.RawMessage `json:"graph"`
	Flow  json.RawMessage `json:"flow,omitempty"`
}

// Error 编译失败:Code 为稳定错误码(程序判定依据,单源 internal/codes),
// Message 中文可读(不参与判定)。调用点(HTTP 写端点/发布守门/渲染管线)
// 按 Code 上信封分流,Message 仅承载定位上下文。
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Products 编译产物:帧序列 + 流链节点(引擎管线输入)。一次解码产出全部产物,
// 渲染管线不再对同批 graph 二次解码(原 DecodeProducts 机械解码段并入 Compile,
// 撤销其独立消费面);页 id 与 name 是编辑层字段,编译即弃、不进产物。
type Products struct {
	// Frames 帧序列:帧序 = 页序
	Frames []*canvas.Canvas
	// Chain 流链节点数组:nil = 空链缺省形态 = 纯文档级管线(与引擎空链分流同义)
	Chain []paginate.FlowChainNode
}

// Compile 页序列 → 编译产物(帧序列 + 流链节点)。帧序 = 页序;流链 = 有 flow
// 页的 flow 原文并入链节点(只注入 frame = 页下标;页 flow 键面按规格不含
// frame,越规携带时以页序覆盖,不让载荷值改写帧位),全页无 flow = 空链缺省
// 形态(nil)。页 id 与 name 是编辑层字段,编译即弃、不进产物。
//
// 校验一体(spec:保存预检与渲染管线复用同一实现):逐页裁决页级形状(id
// 缺失/null/空/非字符串/页内重复、flow 非 JSON 对象、name 非字符串 →
// page_invalid,消息带页序号(1 起,对齐前端页签)与 id),graph 逐帧解码
// (非法 JSON 与解码失败沿用既有 code 分流),后交 paginate.ValidateFlowChain
// 做流链裁决(语义错透传引擎 sentinel,经 errors.Is 对号)。失败返回零值产物
// 与带 code 的错误。
func Compile(pages []Page) (Products, *Error) {
	frames := make([]*canvas.Canvas, 0, len(pages))
	nodes := make([]json.RawMessage, 0, len(pages))
	seenIDs := make(map[string]struct{}, len(pages))

	for i := range pages {
		p := pages[i]

		// 页 id:非空字符串且页内唯一(编辑层引用键,前端生成、服务端校验)
		idTrim := bytes.TrimSpace(p.ID)
		if len(idTrim) == 0 || string(idTrim) == "null" {
			return Products{}, &Error{codes.PageInvalid, fmt.Sprintf("第 %d 页: 页 id 缺失(页须携带非空字符串 id)", i+1)}
		}
		var id string
		if err := json.Unmarshal(idTrim, &id); err != nil {
			return Products{}, &Error{codes.PageInvalid, fmt.Sprintf("第 %d 页: 页 id 须为字符串,得到 %s", i+1, idTrim)}
		}
		if id == "" {
			return Products{}, &Error{codes.PageInvalid, fmt.Sprintf("第 %d 页: 页 id 不得为空字符串", i+1)}
		}
		if _, dup := seenIDs[id]; dup {
			return Products{}, &Error{codes.PageInvalid, fmt.Sprintf("第 %d 页(id=%s): 页 id 页内重复", i+1, id)}
		}
		seenIDs[id] = struct{}{}
		ctx := fmt.Sprintf("第 %d 页(id=%s)", i+1, id)

		// 页 name:可选显示用,缺席/null 宽容,在场须为字符串(服务端不查重)
		if nameTrim := bytes.TrimSpace(p.Name); len(nameTrim) > 0 && string(nameTrim) != "null" {
			var name string
			if err := json.Unmarshal(nameTrim, &name); err != nil {
				return Products{}, &Error{codes.PageInvalid, fmt.Sprintf("%s: 页 name 须为字符串", ctx)}
			}
		}

		// 页 flow 形状:键缺席与 null 同义 = 链外页;在场须为 JSON 对象。
		// mode 缺失/未知键/双 paged/quota 形态等语义错不在形状层抢判,
		// 全数交引擎流链校验透传(spec:example 侧不复刻流校验规则)
		flowTrim := bytes.TrimSpace(p.Flow)
		hasFlow := len(flowTrim) > 0 && string(flowTrim) != "null"
		var flowNode map[string]json.RawMessage
		if hasFlow {
			if err := json.Unmarshal(flowTrim, &flowNode); err != nil {
				return Products{}, &Error{codes.PageInvalid, fmt.Sprintf("%s: 页 flow 须为 JSON 对象(键面镜像引擎链节点去 frame;语义错由引擎流链校验透传)", ctx)}
			}
		}

		// graph 解码:沿用既有 code 分流(invalid_json / 解码三码),与预检同管线
		var wire canvas.Graph
		if err := json.Unmarshal(p.Graph, &wire); err != nil {
			return Products{}, &Error{codes.InvalidJSON, fmt.Sprintf("%s 的 graph 不是合法 wire 形态: %v", ctx, err)}
		}
		frame, err := canvas.FromGraph(wire)
		if err != nil {
			return Products{}, toCodeError(err, ctx+" 解码失败")
		}

		frames = append(frames, frame)
		if hasFlow {
			// flow 原文并入链节点:map[string]json.RawMessage 承接全部键
			// (未知键原样留存供引擎键封闭裁决),只注入 frame = 页下标
			flowNode["frame"], _ = json.Marshal(i)
			node, err := json.Marshal(flowNode)
			if err != nil {
				return Products{}, &Error{codes.InvalidJSON, fmt.Sprintf("%s 的 flow 序列化失败: %v", ctx, err)}
			}
			nodes = append(nodes, node)
		}
	}

	// 全页无 flow = 空链 = 纯文档级管线(不经链校验,与引擎空链分流同判)
	if len(nodes) == 0 {
		return Products{Frames: frames}, nil
	}
	// 节点原文 → 类型化链节点须经 JSON 往返:引擎 FlowChainNode 以未导出 raw
	// 键面承接封闭键裁决,解码是其唯一填充机制(引擎内部数据,不得旁路)
	chainRaw, err := json.Marshal(nodes)
	if err != nil {
		return Products{}, &Error{codes.InvalidJSON, fmt.Sprintf("流链序列化失败: %v", err)}
	}
	var chain []paginate.FlowChainNode
	if err := json.Unmarshal(chainRaw, &chain); err != nil {
		return Products{}, &Error{codes.InvalidJSON, fmt.Sprintf("流链解码失败: %v", err)}
	}
	if _, err := paginate.ValidateFlowChain(frames, chain); err != nil {
		return Products{}, toCodeError(err, "流链校验失败")
	}
	return Products{Frames: frames, Chain: chain}, nil
}

// PageName 页显示名解码:缺席/null/非字符串归空串——编译期已把非字符串 name
// 打回(page_invalid),此处只兜底防御性路径;缺省名「第 N 页」由前端渲染,
// 服务端不补。渲染产物 images[].name 消费。
func PageName(p Page) string {
	trim := bytes.TrimSpace(p.Name)
	if len(trim) == 0 || string(trim) == "null" {
		return ""
	}
	var name string
	if err := json.Unmarshal(trim, &name); err != nil {
		return ""
	}
	return name
}

// toCodeError go-canvas sentinel → 稳定 code(沿用既有预检管线的 sentinel 对号表,
// 保证 graph 解码与流链校验的 code 分流与既有管线逐一相同;无稳定码的解码错
// 兜底 invalid_json——graph wire 形态错的兜底位)。
func toCodeError(err error, context string) *Error {
	code := codes.InvalidJSON
	switch {
	case errors.Is(err, layer.ErrUnknownLayerType):
		code = codes.UnknownLayerType
	case errors.Is(err, layer.ErrTemplateRowsConflict):
		code = codes.TemplateRowsConflict
	case errors.Is(err, layer.ErrRowsPathMissing):
		code = codes.RowsPathMissing
	case errors.Is(err, paginate.ErrFlowChainInvalid):
		code = codes.FlowChainInvalid
	case errors.Is(err, paginate.ErrFlowRowsPathInconsistent):
		code = codes.FlowRowsPathInconsistent
	case errors.Is(err, paginate.ErrPaginateTargetInvalid):
		code = codes.PaginateTargetInvalid
	}
	return &Error{Code: code, Message: fmt.Sprintf("%s: %v", context, err)}
}
