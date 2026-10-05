// Package preflight:模板保存预检(spec §3.4)——POST/PUT 模板落库前逐帧解码 +
// 流链校验全跑,不过即 400 打回不落库。错误码为三端一致性锚点(spec §2.5,
// 单源在 internal/codes),经 errors.Is 对 go-canvas sentinel 判定;此为 09 票
// 导出面(ValidateFlowChain)的消费点,渲染期(13 票)对同一管线防御性复跑。
package preflight

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hankchen/go-canvas/canvas"
	"github.com/hankchen/go-canvas/layer"
	"github.com/hankchen/go-canvas/paginate"

	"example/server/internal/codes"
)

// Error 预检失败:Code 为稳定错误码(程序判定依据),Message 中文可读(不参与判定)
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Check 保存预检(spec §3.4):解码 + 链校验全跑,不过返回稳定码错误。
// 保存与渲染共用同一段(渲染期防御复跑),见 DecodeAndValidate
func Check(graphs []json.RawMessage, flowChain json.RawMessage) *Error {
	_, _, verr := DecodeAndValidate(graphs, flowChain)
	return verr
}

// DecodeAndValidate 预检管线共用段(spec §3.4:保存时预检与渲染期防御复跑
// 同一管线):逐帧 json.Unmarshal + canvas.FromGraph(解码三码),后流链解码与
// paginate.ValidateFlowChain(链三码)。flowChain 缺省/null/空数组 = 空链 =
// 纯文档管线,不经链校验(spec §3.1,与 DocumentCompiler.Compile 的空链分流
// 同判——保存端不得比渲染端严)。graph 与流链节点的 wire 形态错误(JSON 解码
// 错)归 invalid_json(spec §2.5:请求体 JSON 形态错误,含流链节点非 JSON 对象)。
// 通过返回解码出的帧序列与流链(空链形态为 nil),供渲染管线(13 票)直接续跑
// 编译,避免二次解码与双份解码逻辑。
func DecodeAndValidate(graphs []json.RawMessage, flowChain json.RawMessage) ([]*canvas.Canvas, []paginate.FlowChainNode, *Error) {
	frames := make([]*canvas.Canvas, 0, len(graphs))
	for i, raw := range graphs {
		var wire canvas.Graph
		if err := json.Unmarshal(raw, &wire); err != nil {
			return nil, nil, &Error{codes.InvalidJSON, fmt.Sprintf("第 %d 帧的 graph 不是合法 wire 形态: %v", i, err)}
		}
		c, err := canvas.FromGraph(wire)
		if err != nil {
			return nil, nil, toCodeError(err, fmt.Sprintf("第 %d 帧解码失败", i))
		}
		frames = append(frames, c)
	}

	if len(bytes.TrimSpace(flowChain)) == 0 {
		return frames, nil, nil // 键缺省 = 空链
	}
	var chain []paginate.FlowChainNode
	if err := json.Unmarshal(flowChain, &chain); err != nil {
		return nil, nil, &Error{codes.InvalidJSON, fmt.Sprintf("flowChain 须为节点对象数组(spec §3.1): %v", err)}
	}
	if len(chain) == 0 {
		return frames, nil, nil // 显式空链与 null 同义 = 纯文档管线,不经链校验(spec §3.1)
	}
	if _, err := paginate.ValidateFlowChain(frames, chain); err != nil {
		return nil, nil, toCodeError(err, "流链校验失败")
	}
	return frames, chain, nil
}

// toCodeError go-canvas sentinel → 稳定 code(spec §2.5)。无稳定码的解码错
// (模板行/单元格内容病态形态)同归 invalid_json——graph wire 形态错的兜底位。
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
