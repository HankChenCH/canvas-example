// Package preflight:模板保存预检(spec §3.4)——POST/PUT 模板落库前逐帧解码 +
// 流链校验全跑,不过即 400 打回不落库。错误码为三端一致性锚点(spec §2.5),
// 经 errors.Is 对 go-canvas sentinel 判定;此为 09 票导出面(ValidateFlowChain)
// 的消费点,渲染期(13 票)对同一管线防御性复跑。
package preflight

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hankchen/go-canvas/canvas"
	"github.com/hankchen/go-canvas/layer"
	"github.com/hankchen/go-canvas/paginate"
)

// 稳定错误码(spec §2.5):解码三码 + 链三码 + wire 形态兜底
const (
	CodeInvalidJSON              = "invalid_json"
	CodeUnknownLayerType         = "unknown_layer_type"
	CodeTemplateRowsConflict     = "template_rows_conflict"
	CodeRowsPathMissing          = "rows_path_missing"
	CodeFlowChainInvalid         = "flow_chain_invalid"
	CodeFlowRowsPathInconsistent = "flow_rows_path_inconsistent"
	CodePaginateTargetInvalid    = "paginate_target_invalid"
)

// Error 预检失败:Code 为稳定错误码(程序判定依据),Message 中文可读(不参与判定)
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Check 保存预检(spec §3.4):逐帧 json.Unmarshal + canvas.FromGraph(解码三码),
// 后 paginate.ValidateFlowChain(链三码)。flowChain 缺省/null/空数组 = 空链 =
// 纯文档管线,不经链校验(spec §3.1,与 DocumentCompiler.Compile 的空链分流
// 同判——保存端不得比渲染端严)。graph 与流链节点的 wire 形态错误(JSON 解码
// 错)归 invalid_json(spec §2.5:请求体 JSON 形态错误,含流链节点非 JSON 对象)。
// 通过返回 nil。
func Check(graphs []json.RawMessage, flowChain json.RawMessage) *Error {
	frames := make([]*canvas.Canvas, 0, len(graphs))
	for i, raw := range graphs {
		var wire canvas.Graph
		if err := json.Unmarshal(raw, &wire); err != nil {
			return &Error{CodeInvalidJSON, fmt.Sprintf("第 %d 帧的 graph 不是合法 wire 形态: %v", i, err)}
		}
		c, err := canvas.FromGraph(wire)
		if err != nil {
			return toCodeError(err, fmt.Sprintf("第 %d 帧解码失败", i))
		}
		frames = append(frames, c)
	}

	if len(bytes.TrimSpace(flowChain)) == 0 {
		return nil // 键缺省 = 空链
	}
	var chain []paginate.FlowChainNode
	if err := json.Unmarshal(flowChain, &chain); err != nil {
		return &Error{CodeInvalidJSON, fmt.Sprintf("flowChain 须为节点对象数组(spec §3.1): %v", err)}
	}
	if len(chain) == 0 {
		return nil // 显式空链与 null 同义 = 纯文档管线,不经链校验(spec §3.1)
	}
	if _, err := paginate.ValidateFlowChain(frames, chain); err != nil {
		return toCodeError(err, "流链校验失败")
	}
	return nil
}

// toCodeError go-canvas sentinel → 稳定 code(spec §2.5)。无稳定码的解码错
// (模板行/单元格内容病态形态)同归 invalid_json——graph wire 形态错的兜底位。
func toCodeError(err error, context string) *Error {
	code := CodeInvalidJSON
	switch {
	case errors.Is(err, layer.ErrUnknownLayerType):
		code = CodeUnknownLayerType
	case errors.Is(err, layer.ErrTemplateRowsConflict):
		code = CodeTemplateRowsConflict
	case errors.Is(err, layer.ErrRowsPathMissing):
		code = CodeRowsPathMissing
	case errors.Is(err, paginate.ErrFlowChainInvalid):
		code = CodeFlowChainInvalid
	case errors.Is(err, paginate.ErrFlowRowsPathInconsistent):
		code = CodeFlowRowsPathInconsistent
	case errors.Is(err, paginate.ErrPaginateTargetInvalid):
		code = CodePaginateTargetInvalid
	}
	return &Error{Code: code, Message: fmt.Sprintf("%s: %v", context, err)}
}
