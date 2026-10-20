package pages

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"example/server/internal/codes"
)

// graphWire 画布 wire 最小形态:layers 为 JSON 数组字面(照 preflight 测试风格)
func graphWire(layers string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"canvas":{"width":794,"height":1123},"layers":[%s]}`, layers))
}

// templateTableWire 模板态顶层表(rowsPath 取行路径):链校验的槽位表形态
func templateTableWire(rowsPath string) string {
	return fmt.Sprintf(`{"type":"TableLayer","priority":50,"name":"卡片区","data":{"rowsPath":%q},"template":{"type":"TableRowTemplate","priority":0,"cells":[]}}`, rowsPath)
}

// pageWire 页 wire 字面量 → Page:RawMessage 字段承接原文,页级形状裁决留给编译期
func pageWire(t *testing.T, literal string) Page {
	t.Helper()
	var p Page
	if err := json.Unmarshal([]byte(literal), &p); err != nil {
		t.Fatalf("页字面量解码: %v", err)
	}
	return p
}

// mustCode 断言编译失败且 code 对号(照 preflight 测试风格)
func mustCode(t *testing.T, got *Error, wantCode string) {
	t.Helper()
	if got == nil {
		t.Fatalf("编译应失败(code=%s),实得通过", wantCode)
	}
	if got.Code != wantCode {
		t.Fatalf("code = %s, 期望 %s(消息:%s)", got.Code, wantCode, got.Message)
	}
}

// mustJSONEqual 断言产物 JSON 与期望字面量语义相等(键序不敏感)
func mustJSONEqual(t *testing.T, got json.RawMessage, want string) {
	t.Helper()
	var a, b any
	if err := json.Unmarshal(got, &a); err != nil {
		t.Fatalf("产物不是合法 JSON: %v(原文:%s)", err, got)
	}
	if err := json.Unmarshal([]byte(want), &b); err != nil {
		t.Fatalf("期望字面量不是合法 JSON: %v", err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("产物不符\n实得: %s\n期望: %s", got, want)
	}
}

// TestCompile_LegalPages 帧序 = 页序(逐帧解码非空)、flow 节点按键面构造
// (mode/quota/omitIfEmpty 原样 + 注入 frame)、页 id 与 name 不进编译产物
func TestCompile_LegalPages(t *testing.T) {
	// 链上帧须各有一张模板态槽位表且 rowsPath 链内一致(引擎语义)
	g0, g1 := graphWire(templateTableWire("certificates")), graphWire(templateTableWire("certificates"))
	pages := []Page{
		pageWire(t, fmt.Sprintf(`{"id":"pg_main","name":"主页","graph":%s,"flow":{"mode":"fixed","quota":3}}`, g0)),
		pageWire(t, fmt.Sprintf(`{"id":"pg_cont","graph":%s,"flow":{"mode":"paged","omitIfEmpty":true}}`, g1)),
	}
	products, verr := Compile(pages)
	if verr != nil {
		t.Fatalf("合法页序列应通过: %v", verr)
	}
	if len(products.Frames) != 2 || products.Frames[0] == nil || products.Frames[1] == nil {
		t.Fatalf("帧序应等于页序且逐帧解码非空,实得 %+v", products.Frames)
	}
	chainJSON, err := json.Marshal(products.Chain)
	if err != nil {
		t.Fatalf("流链序列化: %v", err)
	}
	mustJSONEqual(t, chainJSON, `[{"frame":0,"mode":"fixed","quota":3},{"frame":1,"mode":"paged","omitIfEmpty":true}]`)
	// 页 id 与 name 是编辑层字段,不出编译产物
	for _, needle := range []string{"pg_main", "pg_cont", "主页"} {
		if bytes.Contains(chainJSON, []byte(needle)) {
			t.Fatalf("编译产物不得携带页 id/name:%q 混入流链 %s", needle, chainJSON)
		}
	}
}

// TestCompile_AllPagesWithoutFlow_EmptyChainLegal 全页无 flow = 空链缺省形态
// (nil)= 合法的纯文档级管线;无 flow 页不吃行、照常占帧位(帧序 = 页序)
func TestCompile_AllPagesWithoutFlow_EmptyChainLegal(t *testing.T) {
	g0, g1 := graphWire(``), graphWire(templateTableWire("certificates"))
	cases := map[string][]Page{
		"flow键全缺席": {
			pageWire(t, fmt.Sprintf(`{"id":"a","graph":%s}`, g0)),
			pageWire(t, fmt.Sprintf(`{"id":"b","graph":%s}`, g1)),
		},
		"flow全null(键缺席与null同义)": {
			pageWire(t, fmt.Sprintf(`{"id":"a","graph":%s,"flow":null}`, g0)),
			pageWire(t, fmt.Sprintf(`{"id":"b","graph":%s,"flow":null}`, g1)),
		},
		"缺席与null混合": {
			pageWire(t, fmt.Sprintf(`{"id":"a","graph":%s}`, g0)),
			pageWire(t, fmt.Sprintf(`{"id":"b","graph":%s,"flow":null}`, g1)),
		},
		"零页": nil,
	}
	for name, pages := range cases {
		products, verr := Compile(pages)
		if verr != nil {
			t.Fatalf("%s:全页无 flow 应按纯文档管线通过: %v", name, verr)
		}
		if products.Chain != nil {
			t.Fatalf("%s:全页无 flow 的流链应为空链缺省形态(nil),实得 %v", name, products.Chain)
		}
		if len(products.Frames) != len(pages) {
			t.Fatalf("%s:帧数应等于页数(无 flow 页照常占帧位),实得 %d 帧", name, len(products.Frames))
		}
	}
}

// TestCompile_PageShapeCodes 页级形状错逐项打 page_invalid(ADR 0025:页级形状
// 归服务端自有码;flow 语义错不在此列,透传引擎 sentinel)
func TestCompile_PageShapeCodes(t *testing.T) {
	g := graphWire(``)
	cases := map[string]struct {
		pages    []Page
		wantCode string
	}{
		"id缺失": {
			[]Page{pageWire(t, fmt.Sprintf(`{"graph":%s}`, g))},
			codes.PageInvalid,
		},
		"id为null": {
			[]Page{pageWire(t, fmt.Sprintf(`{"id":null,"graph":%s}`, g))},
			codes.PageInvalid,
		},
		"id空字符串": {
			[]Page{pageWire(t, fmt.Sprintf(`{"id":"","graph":%s}`, g))},
			codes.PageInvalid,
		},
		"id非字符串": {
			[]Page{pageWire(t, fmt.Sprintf(`{"id":123,"graph":%s}`, g))},
			codes.PageInvalid,
		},
		"id页内重复": {
			[]Page{
				pageWire(t, fmt.Sprintf(`{"id":"pg_x","graph":%s}`, g)),
				pageWire(t, fmt.Sprintf(`{"id":"pg_x","graph":%s}`, g)),
			},
			codes.PageInvalid,
		},
		"flow非对象数组": {
			[]Page{pageWire(t, fmt.Sprintf(`{"id":"a","graph":%s,"flow":[]}`, g))},
			codes.PageInvalid,
		},
		"flow非对象字符串": {
			[]Page{pageWire(t, fmt.Sprintf(`{"id":"a","graph":%s,"flow":"fixed"}`, g))},
			codes.PageInvalid,
		},
		"flow非对象数字": {
			[]Page{pageWire(t, fmt.Sprintf(`{"id":"a","graph":%s,"flow":3}`, g))},
			codes.PageInvalid,
		},
		"name非字符串": {
			[]Page{pageWire(t, fmt.Sprintf(`{"id":"a","name":123,"graph":%s}`, g))},
			codes.PageInvalid,
		},
	}
	for _, tc := range cases {
		_, verr := Compile(tc.pages)
		mustCode(t, verr, tc.wantCode)
	}
}

// TestCompile_ShapeMessageCarriesIndexAndID 失败消息带页序号(1 起,对齐前端
// 页签「第 N 页」)与 id(定位作者改哪页)
func TestCompile_ShapeMessageCarriesIndexAndID(t *testing.T) {
	g := graphWire(``)
	pages := []Page{
		pageWire(t, fmt.Sprintf(`{"id":"pg_ok","graph":%s}`, g)),
		pageWire(t, fmt.Sprintf(`{"id":"pg_2","name":123,"graph":%s}`, g)),
	}
	_, verr := Compile(pages)
	mustCode(t, verr, codes.PageInvalid)
	for _, needle := range []string{"第 2 页", "pg_2"} {
		if !strings.Contains(verr.Message, needle) {
			t.Fatalf("失败消息应带页序号与 id(缺 %q):%s", needle, verr.Message)
		}
	}
}

// TestCompile_FlowSemanticCodesPassthrough flow 语义错透传引擎稳定 code——
// example 侧不复刻/抢判任何流校验规则,页 flow 原样并入链节点交引擎裁决
// (守卫矩阵:非法输入防线用例随矩阵钉死;原 preflight 包链三码矩阵的
// pages 形态表达面并入此处)
func TestCompile_FlowSemanticCodesPassthrough(t *testing.T) {
	g := graphWire(``)
	tplGraph := func(rowsPath string) json.RawMessage { return graphWire(templateTableWire(rowsPath)) }
	cases := map[string]struct {
		pages    []Page
		wantCode string
	}{
		"双paged": {
			[]Page{
				pageWire(t, fmt.Sprintf(`{"id":"a","graph":%s,"flow":{"mode":"paged"}}`, g)),
				pageWire(t, fmt.Sprintf(`{"id":"b","graph":%s,"flow":{"mode":"paged"}}`, g)),
			},
			codes.FlowChainInvalid,
		},
		"paged携带quota": {
			[]Page{
				pageWire(t, fmt.Sprintf(`{"id":"a","graph":%s,"flow":{"mode":"fixed"}}`, g)),
				pageWire(t, fmt.Sprintf(`{"id":"b","graph":%s,"flow":{"mode":"paged","quota":2}}`, g)),
			},
			codes.FlowChainInvalid,
		},
		"未知键": {
			[]Page{pageWire(t, fmt.Sprintf(`{"id":"a","graph":%s,"flow":{"mode":"fixed","bogus":1}}`, g))},
			codes.FlowChainInvalid,
		},
		"flow空对象mode缺失": {
			[]Page{pageWire(t, fmt.Sprintf(`{"id":"a","graph":%s,"flow":{}}`, g))},
			codes.FlowChainInvalid,
		},
		"rowsPath链内不一致": {
			[]Page{
				pageWire(t, fmt.Sprintf(`{"id":"a","graph":%s,"flow":{"mode":"fixed"}}`, tplGraph("certificates"))),
				pageWire(t, fmt.Sprintf(`{"id":"b","graph":%s,"flow":{"mode":"paged"}}`, tplGraph("other"))),
			},
			codes.FlowRowsPathInconsistent,
		},
		// 链外帧同 rowsPath 碰撞(不限定模板态):链上仅页 0,页 1 无 flow
		"链外页同rowsPath碰撞": {
			[]Page{
				pageWire(t, fmt.Sprintf(`{"id":"a","graph":%s,"flow":{"mode":"paged"}}`, tplGraph("certificates"))),
				pageWire(t, fmt.Sprintf(`{"id":"b","graph":%s}`, tplGraph("certificates"))),
			},
			codes.FlowRowsPathInconsistent,
		},
		"链上帧无槽位表": {
			[]Page{pageWire(t, fmt.Sprintf(`{"id":"a","graph":%s,"flow":{"mode":"fixed"}}`, g))},
			codes.FlowChainInvalid,
		},
		"链上帧双顶层表": {
			[]Page{pageWire(t, fmt.Sprintf(`{"id":"a","graph":%s,"flow":{"mode":"fixed"}}`, graphWire(templateTableWire("a")+","+templateTableWire("a"))))},
			codes.PaginateTargetInvalid,
		},
		// 链外帧双顶层表照打(链校验约束全部帧,不只链上)
		"链外页双顶层表": {
			[]Page{
				pageWire(t, fmt.Sprintf(`{"id":"a","graph":%s,"flow":{"mode":"fixed"}}`, tplGraph("a"))),
				pageWire(t, fmt.Sprintf(`{"id":"b","graph":%s}`, graphWire(templateTableWire("b")+","+templateTableWire("b")))),
			},
			codes.PaginateTargetInvalid,
		},
	}
	for _, tc := range cases {
		_, verr := Compile(tc.pages)
		mustCode(t, verr, tc.wantCode)
	}
}

// TestCompile_GraphDecodeKeepsExistingCodes graph 非法 JSON 与解码失败沿用既有
// code 分流(spec:不归 page_invalid)——编译内部解码帧与预检同管线同码
// (原 preflight 解码三码矩阵并入)
func TestCompile_GraphDecodeKeepsExistingCodes(t *testing.T) {
	cases := map[string]struct {
		graph    string
		wantCode string
	}{
		"graph非JSON对象": {`42`, codes.InvalidJSON},
		"graph字段类型不符":  {`{"canvas":{"width":"abc"}}`, codes.InvalidJSON},
		"未知图层类型": {
			`{"canvas":{"width":794,"height":1123},"layers":[{"type":"GhostLayer","priority":0}]}`,
			codes.UnknownLayerType,
		},
		"template与rows双键": {
			`{"canvas":{"width":794,"height":1123},"layers":[{"type":"TableLayer","priority":0,"data":{"rowsPath":"rows"},"template":{"type":"TableRowTemplate","cells":[]},"rows":[]}]}`,
			codes.TemplateRowsConflict,
		},
		"模板态缺rowsPath": {
			`{"canvas":{"width":794,"height":1123},"layers":[{"type":"TableLayer","priority":0,"template":{"type":"TableRowTemplate","cells":[]}}]}`,
			codes.RowsPathMissing,
		},
	}
	for _, tc := range cases {
		_, verr := Compile([]Page{pageWire(t, fmt.Sprintf(`{"id":"a","graph":%s}`, tc.graph))})
		mustCode(t, verr, tc.wantCode)
	}
}

// TestPageName 页显示名解码:字符串原名、缺席/null/非字符串归空串(编译期已
// 打回非字符串,此处兜防御;缺省名由前端渲染,服务端不补)
func TestPageName(t *testing.T) {
	cases := map[string]struct {
		page Page
		want string
	}{
		"字符串":  {pageWire(t, `{"id":"a","name":"主页","graph":{}}`), "主页"},
		"缺席":   {pageWire(t, `{"id":"a","graph":{}}`), ""},
		"null": {pageWire(t, `{"id":"a","name":null,"graph":{}}`), ""},
		"非字符串": {pageWire(t, `{"id":"a","name":123,"graph":{}}`), ""},
	}
	for name, tc := range cases {
		if got := PageName(tc.page); got != tc.want {
			t.Fatalf("[%s] PageName = %q, 期望 %q", name, got, tc.want)
		}
	}
}
