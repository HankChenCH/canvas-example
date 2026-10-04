package preflight

import (
	"encoding/json"
	"fmt"
	"testing"
)

// graphWire 画布 wire 最小形态:layers 为 JSON 数组字面
func graphWire(layers string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"canvas":{"width":794,"height":1123},"layers":[%s]}`, layers))
}

// templateTableWire 模板态顶层表(rowsPath 取行路径):链校验的槽位表形态(spec §3.1)
func templateTableWire(rowsPath string) string {
	return fmt.Sprintf(`{"type":"TableLayer","priority":50,"name":"卡片区","data":{"rowsPath":%q},"template":{"type":"TableRowTemplate","priority":0,"cells":[]}}`, rowsPath)
}

// chainWire 流链 wire 形态
func chainWire(nodes string) json.RawMessage {
	return json.RawMessage(nodes)
}

// mustCode 断言预检失败且 code 对号
func mustCode(t *testing.T, got *Error, wantCode string) {
	t.Helper()
	if got == nil {
		t.Fatalf("预检应失败(code=%s),实得通过", wantCode)
	}
	if got.Code != wantCode {
		t.Fatalf("code = %s, 期望 %s(消息:%s)", got.Code, wantCode, got.Message)
	}
}

func TestCheck_MinimalFramePasses(t *testing.T) {
	// 纯文档管线:flowChain 缺省/null = 空链,免链校验(spec §3.1)
	for _, chain := range map[string]json.RawMessage{
		"缺省":   nil,
		"null": json.RawMessage(`null`),
		"空数组":  json.RawMessage(`[]`),
	} {
		if got := Check([]json.RawMessage{graphWire(``)}, chain); got != nil {
			t.Fatalf("最小帧应通过(%v): %v", chain, got)
		}
	}
}

func TestCheck_TemplateFramesWithChainPasses(t *testing.T) {
	// 形状对齐 seed:两帧各一张模板态顶层表,同 rowsPath,合法链
	graphs := []json.RawMessage{
		graphWire(templateTableWire("certificates")),
		graphWire(templateTableWire("certificates")),
	}
	chain := chainWire(`[{"frame":0,"mode":"fixed"},{"frame":1,"mode":"paged","omitIfEmpty":true}]`)
	if got := Check(graphs, chain); got != nil {
		t.Fatalf("seed 同形载荷应通过: %v", got)
	}
}

// TestCheck_ExplicitEmptyChainSkipsValidation 显式空链与 null 同义 = 纯文档管线,
// 不经链校验(spec §3.1)——链校验面(如 ≥2 顶层表)只约束真实非空链,
// 保存端不得比渲染端(DocumentCompiler 空链分流)严
func TestCheck_ExplicitEmptyChainSkipsValidation(t *testing.T) {
	graphs := []json.RawMessage{
		graphWire(templateTableWire("a") + "," + templateTableWire("a")),
	}
	if got := Check(graphs, chainWire(`[]`)); got != nil {
		t.Fatalf("空链 + 双顶层表应按纯文档管线通过: %v", got)
	}
}

// TestCheck_DecodeCodes 逐帧解码三码对号(spec §2.5 解码段)
func TestCheck_DecodeCodes(t *testing.T) {
	cases := map[string]struct {
		layers   string
		wantCode string
	}{
		"未知图层类型": {`{"type":"GhostLayer","priority":0}`, CodeUnknownLayerType},
		"template与rows双键": {
			`{"type":"TableLayer","priority":0,"data":{"rowsPath":"rows"},"template":{"type":"TableRowTemplate","cells":[]},"rows":[]}`,
			CodeTemplateRowsConflict,
		},
		"模板态缺rowsPath": {
			`{"type":"TableLayer","priority":0,"template":{"type":"TableRowTemplate","cells":[]}}`,
			CodeRowsPathMissing,
		},
	}
	for _, tc := range cases {
		got := Check([]json.RawMessage{graphWire(tc.layers)}, nil)
		mustCode(t, got, tc.wantCode)
	}
}

// TestCheck_ChainCodes 链三码对号(spec §2.5 编译段——链校验三条)
func TestCheck_ChainCodes(t *testing.T) {
	twoFrames := []json.RawMessage{graphWire(``), graphWire(``)}
	cases := map[string]struct {
		graphs   []json.RawMessage
		chain    json.RawMessage
		wantCode string
	}{
		"双paged不在链尾": {
			twoFrames,
			chainWire(`[{"frame":0,"mode":"paged"},{"frame":1,"mode":"paged"}]`),
			CodeFlowChainInvalid,
		},
		"未知键": {
			twoFrames,
			chainWire(`[{"frame":0,"mode":"fixed","bogus":1}]`),
			CodeFlowChainInvalid,
		},
		"frame越界": {
			twoFrames,
			chainWire(`[{"frame":5,"mode":"fixed"}]`),
			CodeFlowChainInvalid,
		},
		"链内rowsPath不一致": {
			[]json.RawMessage{
				graphWire(templateTableWire("certificates")),
				graphWire(templateTableWire("other")),
			},
			chainWire(`[{"frame":0,"mode":"fixed"},{"frame":1,"mode":"paged"}]`),
			CodeFlowRowsPathInconsistent,
		},
		// 链外帧同 rowsPath 碰撞(不限定模板态,spec §3.3 链外一致性)
		"链外帧同rowsPath碰撞": {
			[]json.RawMessage{
				graphWire(templateTableWire("certificates")),
				graphWire(templateTableWire("certificates")),
			},
			chainWire(`[{"frame":0,"mode":"paged"}]`),
			CodeFlowRowsPathInconsistent,
		},
		"链上帧双顶层表": {
			[]json.RawMessage{
				graphWire(templateTableWire("a") + "," + templateTableWire("a")),
			},
			chainWire(`[{"frame":0,"mode":"fixed"}]`),
			CodePaginateTargetInvalid,
		},
		"链外帧双顶层表": {
			[]json.RawMessage{
				graphWire(templateTableWire("a")),
				graphWire(templateTableWire("b") + "," + templateTableWire("b")),
			},
			chainWire(`[{"frame":0,"mode":"fixed"}]`),
			CodePaginateTargetInvalid,
		},
	}
	for _, tc := range cases {
		got := Check(tc.graphs, tc.chain)
		mustCode(t, got, tc.wantCode)
	}
}

// TestCheck_WireFormErrors wire 形态错误归 invalid_json(spec §2.5:请求体 JSON
// 形态错误,含流链节点非 JSON 对象形态——12 票票面第四条)
func TestCheck_WireFormErrors(t *testing.T) {
	cases := map[string]struct {
		graphs []json.RawMessage
		chain  json.RawMessage
	}{
		"graph非JSON对象": {[]json.RawMessage{json.RawMessage(`42`)}, nil},
		"graph字段类型不符":  {[]json.RawMessage{json.RawMessage(`{"canvas":{"width":"abc"}}`)}, nil},
		"流链节点非对象":      {[]json.RawMessage{graphWire(``)}, json.RawMessage(`["fixed"]`)},
		"流链非数组":        {[]json.RawMessage{graphWire(``)}, json.RawMessage(`{"frame":0}`)},
	}
	for _, tc := range cases {
		got := Check(tc.graphs, tc.chain)
		mustCode(t, got, CodeInvalidJSON)
	}
}
