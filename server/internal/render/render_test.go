package render

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/resolver"

	"example/server/internal/pages"
	"example/server/internal/store"
)

// seedDir seed 目录绝对路径:用例会 Chdir,相对路径随之失效——包初始化时解析一次
var seedDir = func() string {
	abs, err := filepath.Abs(filepath.Join("..", "..", "seed"))
	if err != nil {
		panic(err)
	}
	return abs
}()

// fakeDownloader Downloader 手工桩:记录调用并返回预设内容/错误(errChain 可
// 携带 context 错误,模拟下载流自身超时的双 %w 错误链)
type fakeDownloader struct {
	calls   []string
	content []byte
	err     error
}

func (f *fakeDownloader) Download(_ context.Context, rawURL string) ([]byte, error) {
	f.calls = append(f.calls, rawURL)
	if f.err != nil {
		return nil, f.err
	}
	return f.content, nil
}

// chdirToSeedAssets 切到临时 CWD 并把 seed/assets 整树符号链接到 assets/
// (字体 + student-1..5.png 随包资源,CWD 相对解析,spec §1.2)
func chdirToSeedAssets(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	if err := os.Symlink(filepath.Join(seedDir, "assets"), filepath.Join(tmp, "assets")); err != nil {
		t.Fatalf("链接 seed assets: %v", err)
	}
	t.Chdir(tmp)
}

// localLogo 本地徽标路径(离线,零物化直画)
const localLogo = "assets/u/student-1.png"

// readSeedJSON 读 seed fixture 文件
func readSeedJSON(t *testing.T, name string) json.RawMessage {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(seedDir, name))
	if err != nil {
		t.Fatalf("读 seed %s: %v", name, err)
	}
	return json.RawMessage(b)
}

// wirePage 页构造器:id/name 引号补齐,graph 原文,flow 可选(空串 = 键缺席 =
// 链外页)
func wirePage(id, name, graph, flow string) pages.Page {
	p := pages.Page{
		ID:    json.RawMessage(strconv.Quote(id)),
		Name:  json.RawMessage(strconv.Quote(name)),
		Graph: json.RawMessage(graph),
	}
	if flow != "" {
		p.Flow = json.RawMessage(flow)
	}
	return p
}

// newOfflineRecord 真实 seed 页序列(pages fixture,ADR 0025)与配套 dataset:
// 徽标换本地路径以离线可跑(spec §5.3:徽标是唯一外网点);数据源内容与模板记录
// 分离传递(数据源为独立实体,spec §2.4 数据源段)
func newOfflineRecord(t *testing.T) (*store.TemplateRecord, json.RawMessage) {
	t.Helper()
	dataset := map[string]any{}
	if err := json.Unmarshal(readSeedJSON(t, "dataset.json"), &dataset); err != nil {
		t.Fatal(err)
	}
	org := dataset["org"].(map[string]any)
	org["logo"] = localLogo // 本地路径零物化直画(离线)
	raw, err := json.Marshal(dataset)
	if err != nil {
		t.Fatal(err)
	}
	var doc []pages.Page
	if err := json.Unmarshal(readSeedJSON(t, "pages.json"), &doc); err != nil {
		t.Fatalf("seed pages.json 解码: %v", err)
	}
	rec := &store.TemplateRecord{ID: 1, Pages: doc}
	return rec, raw
}

// newService 逐用例独立 Service(注入假下载器,缓存根 .cache/ 按用例临时 CWD)
func newService(t *testing.T, fd *fakeDownloader) *Service {
	t.Helper()
	svc := &Service{}
	svc.rs = newDefaultResolver(resolver.WithDownloader(fd))
	return svc
}

// pngSize 解码 PNG 头取尺寸
func pngSize(t *testing.T, png []byte) (int, int) {
	t.Helper()
	cfg, _, err := image.DecodeConfig(bytes.NewReader(png))
	if err != nil {
		t.Fatalf("解码 PNG: %v", err)
	}
	return cfg.Width, cfg.Height
}

// TestRun_SeedDocument 种子两页文档 × 5 行 dataset → 容量数学 [主页 2 行,
// 续页 2 行, 续页 1 行] = 3 页(spec §5.1);images 页归属 = [0 主页, 1 续页,
// 1 续页](spec §2.3 页序扁平,frame 字段语义延续为页下标,ADR 0025);页面
// 尺寸 = A4 794×1123
func TestRun_SeedDocument(t *testing.T) {
	chdirToSeedAssets(t)
	svc := newService(t, &fakeDownloader{})
	rec, dataset := newOfflineRecord(t)

	pages, perr := svc.Run(t.Context(), rec, dataset)
	if perr != nil {
		t.Fatalf("渲染失败: %s: %s", perr.Code, perr.Message)
	}
	if len(pages) != 3 {
		t.Fatalf("页数 = %d, 期望 3(spec §5.1 容量数学)", len(pages))
	}
	want := []struct {
		frame int
		name  string
	}{{0, "主页"}, {1, "续页"}, {1, "续页"}}
	for i, w := range want {
		if pages[i].Frame != w.frame || pages[i].Name != w.name {
			t.Fatalf("pages[%d] = {frame %d, name %q}, 期望 {frame %d, name %q}",
				i, pages[i].Frame, pages[i].Name, w.frame, w.name)
		}
		if w, h := pngSize(t, pages[i].PNG); w != 794 || h != 1123 {
			t.Fatalf("pages[%d] 尺寸 = %dx%d, 期望 794x1123", i, w, h)
		}
		// 背景随 canvas 字段走(spec §4):ADR 0022 迁移后种子的白色底来自
		// canvas.background(「页面底色」层退役),背景须经编译分页切页不丢、
		// 渲染铺装为不透明白——左缘背景区(表格区外)取样三页全验
		assertPixelColor(t, pages[i].PNG, 5, 600, 255, 255, 255, 255)
	}
}

// --- 前导斜杠上传引用归一(21 票,spec §2.2 双吃)---

// TestRun_LeadingSlashStaticAsset 静态形态:属性面板编辑产出 ExpressionValue
// 三键且 expression/value 同为前导斜杠 url(21 票复现 2),dataset 未绑直通后
// Image() = url 原文。渲染成功且终图含该图(像素取样比对资源本体色);
// 归一接线前 500 internal_error「读取图片 /assets/u/…」
func TestRun_LeadingSlashStaticAsset(t *testing.T) {
	chdirToSeedAssets(t)
	svc := newService(t, &fakeDownloader{})
	rec := &store.TemplateRecord{Pages: []pages.Page{wirePage("pg_a", "单页", `{
			"canvas":{"width":160,"height":400},
			"layers":[{"type":"ImageLayer","priority":0,
				"spec":{"shape":{"width":160,"height":400}},
				"data":{"valueType":"ExpressionValue","expression":"/assets/u/student-1.png","value":"/assets/u/student-1.png"}}]}`, "")}}

	pages, perr := svc.Run(t.Context(), rec, nil)
	if perr != nil {
		t.Fatalf("渲染失败: %s: %s", perr.Code, perr.Message)
	}
	if len(pages) != 1 {
		t.Fatalf("页数 = %d, 期望 1", len(pages))
	}
	ref, err := os.ReadFile(filepath.Join(seedDir, "assets", "u", "student-1.png"))
	if err != nil {
		t.Fatal(err)
	}
	assertPixelMatches(t, pages[0].PNG, 80, 200, ref, 80, 200)
}

// TestRun_LeadingSlashExpressionAsset 表达式形态:dataset 行内上传 url(前导
// 斜杠)经 {{row.photo}} 求值进入图层——求值产物在 hydrate 改写 wire 后
// FromGraph 重建时产生,server 侧 graph 字面遍历拦不到(21 票影响面),归一须
// 落在编译后渲染前。渲染成功且行内照片格呈现该图
func TestRun_LeadingSlashExpressionAsset(t *testing.T) {
	chdirToSeedAssets(t)
	svc := newService(t, &fakeDownloader{})
	rec := &store.TemplateRecord{Pages: []pages.Page{wirePage("pg_a", "单页", `{
			"canvas":{"width":794,"height":1123},
			"layers":[{"type":"TableLayer","name":"t","priority":50,
				"data":{"rowsPath":"certificates"},
				"template":{"type":"TableRowTemplate","spec":{"shape":{"width":714,"height":400}},
					"cells":[{"type":"TableCellLayer","priority":0,"spec":{"shape":{"width":160,"height":400}},
						"content":{"type":"ImageLayer","priority":0,
							"spec":{"shape":{"width":160,"height":400}},
							"data":{"valueType":"ExpressionValue","expression":"{{row.photo}}","value":"{{row.photo}}"}}}]}}]}`, "")}}

	pages, perr := svc.Run(t.Context(), rec, json.RawMessage(`{"certificates":[{"photo":"/assets/u/student-2.png"}]}`))
	if perr != nil {
		t.Fatalf("渲染失败: %s: %s", perr.Code, perr.Message)
	}
	if len(pages) != 1 {
		t.Fatalf("页数 = %d, 期望 1", len(pages))
	}
	ref, err := os.ReadFile(filepath.Join(seedDir, "assets", "u", "student-2.png"))
	if err != nil {
		t.Fatal(err)
	}
	assertPixelMatches(t, pages[0].PNG, 80, 200, ref, 80, 200)
}

// TestRun_LeadingSlashBackgroundAsset 画布背景图前导斜杠引用归一(ADR 0022 迁移,
// spec §5 example 段;21 票同门):「以图片新建模板」产出的 background.image 为
// 上传 url 前导斜杠形态,归一前 DrawImage 按字面路径读盘必败(500);归一后
// 整幅 cover 呈现该图——canvas 键序 width,height,background(spec §3 钉序)
func TestRun_LeadingSlashBackgroundAsset(t *testing.T) {
	chdirToSeedAssets(t)
	svc := newService(t, &fakeDownloader{})
	rec := &store.TemplateRecord{Pages: []pages.Page{wirePage("pg_a", "单页", `{
			"canvas":{"width":160,"height":400,"background":{"image":"/assets/u/student-3.png"}},
			"layers":[]}`, "")}}

	pages, perr := svc.Run(t.Context(), rec, nil)
	if perr != nil {
		t.Fatalf("渲染失败: %s: %s", perr.Code, perr.Message)
	}
	if len(pages) != 1 {
		t.Fatalf("页数 = %d, 期望 1", len(pages))
	}
	ref, err := os.ReadFile(filepath.Join(seedDir, "assets", "u", "student-3.png"))
	if err != nil {
		t.Fatal(err)
	}
	assertPixelMatches(t, pages[0].PNG, 80, 200, ref, 80, 200)
}

// TestRun_LeadingSlashFontPath 字体路径同归一(spec §2.2 双吃,ToLocalPath
// 消费点二):前导斜杠字体引用渲染期加载成功;归一接线前字体打开失败 500
func TestRun_LeadingSlashFontPath(t *testing.T) {
	chdirToSeedAssets(t)
	svc := newService(t, &fakeDownloader{})
	rec := &store.TemplateRecord{Pages: []pages.Page{wirePage("pg_a", "单页", `{
			"canvas":{"width":794,"height":1123},
			"layers":[{"type":"TextLayer","priority":0,
				"spec":{"shape":{"width":794,"height":100},
					"fontFamily":{"font":"/assets/fonts/NotoSansSC-Regular.otf","fontSize":30,"fontColor":"#333333","angle":0,"autowrap":false}},
					"data":{"valueType":"StaticValue","value":"结业证书"}}]}`, "")}}

	pages, perr := svc.Run(t.Context(), rec, nil)
	if perr != nil {
		t.Fatalf("渲染失败: %s: %s", perr.Code, perr.Message)
	}
	if len(pages) != 1 {
		t.Fatalf("页数 = %d, 期望 1", len(pages))
	}
}

// TestNormalizePages_RemoteUntouched 归一只动本地引用(直测):远程 URL 不回写
// 物化槽(交回渲染期惰性物化下载),本地引用归一为磁盘相对形态——以预写哨兵
// 探针判定:归一若误碰远程必覆写哨兵(21 票审查补)
func TestNormalizePages_RemoteUntouched(t *testing.T) {
	var wire canvas.Graph
	if err := json.Unmarshal([]byte(`{"canvas":{"width":10,"height":10},"layers":[
		{"type":"ImageLayer","priority":0,"spec":{"shape":{"width":10,"height":10}},
		 "data":{"valueType":"StaticValue","value":"https://remote.example/logo.png"}},
		{"type":"ImageLayer","priority":1,"spec":{"shape":{"width":10,"height":10}},
		 "data":{"valueType":"StaticValue","value":"/assets/u/x.png"}}]}`), &wire); err != nil {
		t.Fatal(err)
	}
	c, err := canvas.FromGraph(wire)
	if err != nil {
		t.Fatal(err)
	}
	var remote, local *layer.ImageLayer
	for _, l := range c.GetLayers() {
		img := l.(*layer.ImageLayer)
		switch *img.Image() {
		case "https://remote.example/logo.png":
			remote = img
		case "/assets/u/x.png":
			local = img
		}
	}
	remote.SetResolvedSrc("哨兵")
	local.SetResolvedSrc("哨兵")

	normalizePages([]*canvas.Canvas{c})

	if got := *remote.ResolvedSrc(); got != "哨兵" {
		t.Fatalf("远程引用不应被归一回写,物化槽被覆写为 %q", got)
	}
	if got := *local.ResolvedSrc(); got != "assets/u/x.png" {
		t.Fatalf("本地引用应归一为磁盘相对形态,实得 %q", got)
	}
}

// TestNormalizePages_BackgroundNormalized 画布级背景图引用归一(直测,ADR 0022
// 迁移):背景图本地引用剥前导斜杠(画布级 SetBackground 重建实例——值对象不可变、
// 无分量 setter),背景色与远程 URL 分量不触碰(远程交渲染期惰性物化下载)
func TestNormalizePages_BackgroundNormalized(t *testing.T) {
	var wire canvas.Graph
	if err := json.Unmarshal([]byte(`{"canvas":{"width":10,"height":10,"background":{"color":"#ffffff","image":"/assets/u/x.png"}},"layers":[]}`), &wire); err != nil {
		t.Fatal(err)
	}
	c, err := canvas.FromGraph(wire)
	if err != nil {
		t.Fatal(err)
	}
	normalizePages([]*canvas.Canvas{c})

	bg := c.Background()
	if bg == nil {
		t.Fatal("背景不应被归一移除")
	}
	if bg.Image == nil || *bg.Image != "assets/u/x.png" {
		t.Fatalf("背景图本地引用应归一为磁盘相对形态,实得 %v", bg.Image)
	}
	if bg.Color == nil || *bg.Color != "#ffffff" {
		t.Fatalf("背景色分量不应被触碰,实得 %v", bg.Color)
	}
}

// TestNormalizePages_BackgroundRemoteUntouched 归一只动本地背景图引用(直测):
// 远程 URL 原样保留;以哨兵探针判定归一误触(21 票审查同款)
func TestNormalizePages_BackgroundRemoteUntouched(t *testing.T) {
	var wire canvas.Graph
	if err := json.Unmarshal([]byte(`{"canvas":{"width":10,"height":10,"background":{"image":"https://remote.example/bg.png"}},"layers":[]}`), &wire); err != nil {
		t.Fatal(err)
	}
	c, err := canvas.FromGraph(wire)
	if err != nil {
		t.Fatal(err)
	}
	normalizePages([]*canvas.Canvas{c})

	bg := c.Background()
	if bg == nil || bg.Image == nil || *bg.Image != "https://remote.example/bg.png" {
		t.Fatalf("远程背景图引用不应被归一,实得 %v", bg)
	}
}

// assertPixelMatches 渲染产物 (x, y) 像素与参考图 (rx, ry) 像素一致——渲染
// 成功之外的实质断言(终图包含该图,§6.3 第 4 项后半句);两图同为不透明
// 8bit 色,cover 同尺寸恒等,像素应精确相等
func assertPixelMatches(t *testing.T, png []byte, x, y int, ref []byte, rx, ry int) {
	t.Helper()
	got, _, err := image.Decode(bytes.NewReader(png))
	if err != nil {
		t.Fatalf("解码渲染 PNG: %v", err)
	}
	want, _, err := image.Decode(bytes.NewReader(ref))
	if err != nil {
		t.Fatalf("解码参考图: %v", err)
	}
	gr, gg, gb, ga := got.At(x, y).RGBA()
	wr, wg, wb, wa := want.At(rx, ry).RGBA()
	if gr != wr || gg != wg || gb != wb || ga != wa {
		t.Fatalf("渲染像素 (%d,%d) = (%d,%d,%d,%d), 参考图 (%d,%d) = (%d,%d,%d,%d)",
			x, y, gr, gg, gb, ga, rx, ry, wr, wg, wb, wa)
	}
}

// assertPixelColor 渲染产物 (x, y) 像素为指定 8bit RGBA 色——背景铺装断言用
// (canvas.background 白底经编译分页切页不丢、渲染铺装不透明)
func assertPixelColor(t *testing.T, png []byte, x, y int, r, g, b, a uint8) {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(png))
	if err != nil {
		t.Fatalf("解码渲染 PNG: %v", err)
	}
	gr, gg, gb, ga := img.At(x, y).RGBA()
	wr, wg, wb, wa := uint64(r)*0x101, uint64(g)*0x101, uint64(b)*0x101, uint64(a)*0x101
	if uint64(gr) != wr || uint64(gg) != wg || uint64(gb) != wb || uint64(ga) != wa {
		t.Fatalf("渲染像素 (%d,%d) = (%d,%d,%d,%d), 期望 (%d,%d,%d,%d)",
			x, y, gr, gg, gb, ga, wr, wg, wb, wa)
	}
}

// TestRun_UnboundDataset dataset 为 null 按未绑直通编译(spec §2.4 #7):模板态
// 零行空壳页合法——帧 0 fixed 直通 1 页,帧 1 paged omitIfEmpty 零行跳帧 0 页。
// 用无绑定图层的最小模板:种子模板的表达式徽标在未绑渲染时按原文路径直读必败
// (go-canvas 恒等直通语义,非服务端缺陷),表达式面属绑定态语义
func TestRun_UnboundDataset(t *testing.T) {
	chdirToSeedAssets(t)
	svc := newService(t, &fakeDownloader{})
	rec := &store.TemplateRecord{Pages: []pages.Page{
		wirePage("pg_a", "页一", minimalTemplateTable(), `{"mode":"fixed"}`),
		wirePage("pg_b", "页二", minimalTemplateTable(), `{"mode":"paged","omitIfEmpty":true}`),
	}}

	pagesOut, perr := svc.Run(t.Context(), rec, nil)
	if perr != nil {
		t.Fatalf("渲染失败: %s: %s", perr.Code, perr.Message)
	}
	if len(pagesOut) != 1 || pagesOut[0].Frame != 0 || pagesOut[0].Name != "页一" {
		t.Fatalf("未绑直通应为页一 1 页空壳,实得 %+v", pagesOut)
	}
}

// minimalTemplateTable 单模板表最小画布(无绑定图层,行高 400)
func minimalTemplateTable() string {
	return `{"canvas":{"width":794,"height":1123},"layers":[
		{"type":"TableLayer","name":"t","priority":50,"data":{"rowsPath":"certificates"},
		 "template":{"type":"TableRowTemplate","spec":{"shape":{"width":714,"height":400}},
		 "cells":[{"type":"TableCellLayer","priority":0,"spec":{"shape":{"width":714,"height":400}}}]}}]}`
}

// TestRun_AllPagesWithoutFlow 全页无 flow = 空链 = 纯文档管线(spec §3.1):逐帧
// 全量填充直通分页,各页单页。用 2 行 dataset(表顶 190 + 2×400 = 990 ≤ 1123
// 不溢出;5 行全量会按分页关语义合法报 content_overflow,不属本用例)
func TestRun_AllPagesWithoutFlow(t *testing.T) {
	chdirToSeedAssets(t)
	svc := newService(t, &fakeDownloader{})
	rec, _ := newOfflineRecord(t)
	for i := range rec.Pages { // 摘除页 flow:全页无 flow = 空链缺省形态
		rec.Pages[i].Flow = nil
	}

	pages, perr := svc.Run(t.Context(), rec, twoRowDataset(t))
	if perr != nil {
		t.Fatalf("渲染失败: %s: %s", perr.Code, perr.Message)
	}
	if len(pages) != 2 || pages[0].Frame != 0 || pages[1].Frame != 1 {
		t.Fatalf("空链应各页 1 页,实得 %+v", pages)
	}
}

// twoRowDataset seed dataset 截取前两行(徽标已换本地,离线)
func twoRowDataset(t *testing.T) json.RawMessage {
	t.Helper()
	dataset := map[string]any{}
	if err := json.Unmarshal(readSeedJSON(t, "dataset.json"), &dataset); err != nil {
		t.Fatal(err)
	}
	org := dataset["org"].(map[string]any)
	org["logo"] = localLogo
	certs := dataset["certificates"].([]any)
	dataset["certificates"] = certs[:2]
	raw, err := json.Marshal(dataset)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestRun_EmptyRowsOmitSkip 链尾 paged omitIfEmpty 在剩余行为空时跳帧(spec
// §10.3 零行跳帧):dataset 零行 → 主页空壳 1 页 + 续页跳帧
func TestRun_EmptyRowsOmitSkip(t *testing.T) {
	chdirToSeedAssets(t)
	svc := newService(t, &fakeDownloader{})
	rec, _ := newOfflineRecord(t)

	pages, perr := svc.Run(t.Context(), rec,
		json.RawMessage(`{"org":{"name":"瀚辰","logo":"`+localLogo+`"},"doc":{"title":"t"},"certificates":[]}`))
	if perr != nil {
		t.Fatalf("渲染失败: %s: %s", perr.Code, perr.Message)
	}
	if len(pages) != 1 || pages[0].Frame != 0 {
		t.Fatalf("零行 dataset 应仅主页 1 页空壳,实得 %+v", pages)
	}
}

// TestRun_ExternalPagesPageOnce 链外页分页关恒单页:三页文档只把中页挂链,
// 前后链外页各 1 页(页归属覆盖链间链外页的切段对齐)
func TestRun_ExternalPagesPageOnce(t *testing.T) {
	chdirToSeedAssets(t)
	svc := newService(t, &fakeDownloader{})

	table := func(rowsPath string) string {
		return fmt.Sprintf(`{"canvas":{"width":794,"height":1123},"layers":[
			{"type":"TableLayer","name":"t","priority":50,
			 "data":{"rowsPath":%q},
			 "template":{"type":"TableRowTemplate","spec":{"shape":{"width":714,"height":400}},
			 "cells":[{"type":"TableCellLayer","priority":0,"spec":{"shape":{"width":714,"height":400}}}]}}]}`,
			rowsPath)
	}
	rec := &store.TemplateRecord{
		ID: 1,
		Pages: []pages.Page{
			wirePage("pg_a", "前页", table("extra"), ""),
			wirePage("pg_b", "中页", table("certificates"), `{"mode":"fixed"}`),
			wirePage("pg_c", "后页", table("other"), ""),
		},
	}

	pages, perr := svc.Run(t.Context(), rec, json.RawMessage(`{
		"extra":[{"n":"e1"},{"n":"e2"}],
		"certificates":[{"n":"c1"},{"n":"c2"},{"n":"c3"}],
		"other":[{"n":"o1"}]}`))
	if perr != nil {
		t.Fatalf("渲染失败: %s: %s", perr.Code, perr.Message)
	}
	// 中页 fixed 容量 1123 装 2 行(400×2≤1123<400×3),链外前后页各 1 页
	if len(pages) != 3 {
		t.Fatalf("页数 = %d, 期望 3", len(pages))
	}
	for i, wantFrame := range []int{0, 1, 2} {
		if pages[i].Frame != wantFrame {
			t.Fatalf("pages[%d].frame = %d, 期望 %d", i, pages[i].Frame, wantFrame)
		}
	}
}

// TestRun_ErrorMapping 渲染期防御复跑的错误映射(spec §2.5):页形状/解码/填充/
// 编译码全 400
func TestRun_ErrorMapping(t *testing.T) {
	chdirToSeedAssets(t)
	svc := newService(t, &fakeDownloader{})

	cases := map[string]struct {
		record  *store.TemplateRecord
		dataset json.RawMessage
		want    string
	}{
		"未知图层类型": {
			&store.TemplateRecord{Pages: []pages.Page{wirePage("pg_a", "单页",
				`{"canvas":{"width":100,"height":100},"layers":[{"type":"GhostLayer","priority":0}]}`, "")}},
			nil,
			"unknown_layer_type",
		},
		"rowsPath 取不到行": {
			&store.TemplateRecord{Pages: []pages.Page{wirePage("pg_a", "单页",
				`{"canvas":{"width":794,"height":1123},"layers":[
				 {"type":"TableLayer","priority":50,"data":{"rowsPath":"nope"},
				  "template":{"type":"TableRowTemplate","spec":{"shape":{"width":714,"height":400}},"cells":[]}}]}`, "")}},
			json.RawMessage(`{"other":[]}`),
			"rows_path_invalid",
		},
		"双 paged": {
			&store.TemplateRecord{Pages: []pages.Page{
				wirePage("pg_a", "页一", `{"canvas":{"width":100,"height":100},"layers":[]}`, `{"mode":"paged"}`),
				wirePage("pg_b", "页二", `{"canvas":{"width":100,"height":100},"layers":[]}`, `{"mode":"paged"}`),
			}},
			nil,
			"flow_chain_invalid",
		},
		"fixed 首行超容": {
			&store.TemplateRecord{Pages: []pages.Page{wirePage("pg_a", "单页",
				`{"canvas":{"width":794,"height":1123},"layers":[
				 {"type":"TableLayer","priority":50,"data":{"rowsPath":"certificates"},
				  "template":{"type":"TableRowTemplate","spec":{"shape":{"width":714,"height":1200}},"cells":[]}}]}`,
				`{"mode":"fixed"}`)}},
			json.RawMessage(`{"certificates":[{"n":"x"}]}`),
			"content_overflow",
		},
		"资源表达式求值空串": {
			&store.TemplateRecord{Pages: []pages.Page{wirePage("pg_a", "单页",
				`{"canvas":{"width":100,"height":100},"layers":[
				 {"type":"ImageLayer","priority":0,
				  "spec":{"shape":{"width":10,"height":10}},
				  "data":{"valueType":"ExpressionValue","expression":"{{org.logo}}","value":"{{org.logo}}"}}]}`, "")}},
			json.RawMessage(`{"org":{"logo":""}}`),
			"expression_empty_resource",
		},
		// ADR 0020 单值门:已标记 Image/Qr 的表达式必须恰为一个裸路径片段,
		// 多片段拼接在求值前 fail-fast(求值端对齐 go-canvas c598233)
		"资源表达式多片段拼接": {
			&store.TemplateRecord{Pages: []pages.Page{wirePage("pg_a", "单页",
				`{"canvas":{"width":100,"height":100},"layers":[
				 {"type":"QrCodeLayer","priority":0,
				  "spec":{"shape":{"width":10,"height":10}},
				  "data":{"valueType":"ExpressionValue","expression":"https://verify.example/{{doc.title}}","value":"https://verify.example/{{doc.title}}"}}]}`, "")}},
			json.RawMessage(`{"doc":{"title":"HC-001"}}`),
			"expression_resource_single_ref",
		},
	}
	for name, tc := range cases {
		_, perr := svc.Run(t.Context(), tc.record, tc.dataset)
		if perr == nil {
			t.Fatalf("[%s] 期望失败,实得成功", name)
		}
		if perr.Status != 400 || perr.Code != tc.want {
			t.Fatalf("[%s] = %d/%s, 期望 400/%s(%s)", name, perr.Status, perr.Code, tc.want, perr.Message)
		}
	}
}

// TestRun_DeadlineExceeded 30s deadline 到点 → 504 render_deadline_exceeded
// (spec §2.6;渲染 ctx 已到点,ctx 归因优先于一切资源码)
func TestRun_DeadlineExceeded(t *testing.T) {
	chdirToSeedAssets(t)
	svc := newService(t, &fakeDownloader{})
	ctx, cancel := context.WithTimeout(t.Context(), -time.Second)
	defer cancel()
	rec, dataset := newOfflineRecord(t)

	_, perr := svc.Run(ctx, rec, dataset)
	if perr == nil || perr.Status != 504 || perr.Code != "render_deadline_exceeded" {
		t.Fatalf("期望 504/render_deadline_exceeded, 实得 %+v", perr)
	}
}

// TestRun_CancelledSilent 客户端断连 → render_cancelled,不写响应(spec §2.5:
// 仅日志)——Silent 标记由 handler 判定跳过 writeError
func TestRun_CancelledSilent(t *testing.T) {
	chdirToSeedAssets(t)
	svc := newService(t, &fakeDownloader{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	rec, dataset := newOfflineRecord(t)

	_, perr := svc.Run(ctx, rec, dataset)
	if perr == nil || !perr.Silent || perr.Code != "render_cancelled" {
		t.Fatalf("期望 Silent/render_cancelled, 实得 %+v", perr)
	}
}

// TestRun_DownloadOwnTimeoutIs502 下载流自身超时归 502(spec §2.6):http.Client
// 10s 超时的错误链同样含 context.DeadlineExceeded(实测),经 resolver 预归因成
// 渲染 deadline code——渲染 ctx 仍活时须还原为资源码 resource_download_failed,
// 不得误报 504(「先判 ctx 归因再判资源码」的分流本体)
func TestRun_DownloadOwnTimeoutIs502(t *testing.T) {
	chdirToSeedAssets(t)
	fd := &fakeDownloader{err: fmt.Errorf("下载请求失败: %w", context.DeadlineExceeded)}
	svc := newService(t, fd)
	rec, _ := newOfflineRecord(t)

	_, perr := svc.Run(t.Context(), rec, json.RawMessage(`{"org":{"logo":"https://remote.example/logo.png"},"doc":{"title":"t"},"certificates":[]}`))
	if perr == nil || perr.Status != 502 || perr.Code != "resource_download_failed" {
		t.Fatalf("期望 502/resource_download_failed, 实得 %+v", perr)
	}
}

// TestRun_DownloadFailed502 非 2xx/网络错误等无 ctx 归因的下载失败 → 502
func TestRun_DownloadFailed502(t *testing.T) {
	chdirToSeedAssets(t)
	fd := &fakeDownloader{err: errors.New("boom")}
	svc := newService(t, fd)
	rec, _ := newOfflineRecord(t)

	_, perr := svc.Run(t.Context(), rec, json.RawMessage(`{"org":{"logo":"https://remote.example/logo.png"},"doc":{"title":"t"},"certificates":[]}`))
	if perr == nil || perr.Status != 502 || perr.Code != "resource_download_failed" {
		t.Fatalf("期望 502/resource_download_failed, 实得 %+v", perr)
	}
}

// TestRun_ResourceSaveFailed 缓存根不可写 → 500 resource_save_failed
// (spec §2.5 物化落盘段;缓存根指向一个文件路径使 MkdirAll 失败)
func TestRun_ResourceSaveFailed(t *testing.T) {
	chdirToSeedAssets(t)
	blocked := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := &Service{}
	svc.rs = resolver.New(resolver.WithCacheRoot(blocked))
	rec, _ := newOfflineRecord(t)

	_, perr := svc.Run(t.Context(), rec, json.RawMessage(`{"org":{"logo":"https://remote.example/logo.png"},"doc":{"title":"t"},"certificates":[]}`))
	if perr == nil || perr.Status != 500 || perr.Code != "resource_save_failed" {
		t.Fatalf("期望 500/resource_save_failed, 实得 %+v", perr)
	}
}

// TestRun_CacheReuseAcrossRuns .cache/ 生效(spec §2.7 显式缓存根):同一
// Service 跨两次 Run,远程徽标只下载一次(命中即跳过);QR 物化同样落缓存
func TestRun_CacheReuseAcrossRuns(t *testing.T) {
	chdirToSeedAssets(t)
	png, err := os.ReadFile(filepath.Join(seedDir, "assets", "u", "student-1.png"))
	if err != nil {
		t.Fatal(err)
	}
	fd := &fakeDownloader{content: png}
	svc := newService(t, fd)
	// 种子全量 dataset(行字段齐),徽标换远程 URL 走假下载器
	rec, _ := newOfflineRecord(t)
	dataset := map[string]any{}
	if err := json.Unmarshal(readSeedJSON(t, "dataset.json"), &dataset); err != nil {
		t.Fatal(err)
	}
	dataset["org"].(map[string]any)["logo"] = "https://remote.example/logo.png"
	raw, err := json.Marshal(dataset)
	if err != nil {
		t.Fatal(err)
	}

	if _, perr := svc.Run(t.Context(), rec, raw); perr != nil {
		t.Fatalf("首次渲染失败: %s: %s", perr.Code, perr.Message)
	}
	first := len(fd.calls)
	if first == 0 {
		t.Fatal("首次渲染应发生远程下载")
	}
	if _, perr := svc.Run(t.Context(), rec, raw); perr != nil {
		t.Fatalf("二次渲染失败: %s: %s", perr.Code, perr.Message)
	}
	if len(fd.calls) != first {
		t.Fatalf("二次渲染不应再下载: 首次 %d 次,累计 %d 次", first, len(fd.calls))
	}
	entries, err := os.ReadDir(".cache")
	if err != nil || len(entries) == 0 {
		t.Fatalf(".cache/ 应在 CWD 下生成且有内容: %v", err)
	}
}

// TestRun_InvalidStoredPageFlow 存储态页 flow 非 JSON 对象(防御性,保存预检
// 已挡)→ 400 page_invalid
func TestRun_InvalidStoredPageFlow(t *testing.T) {
	chdirToSeedAssets(t)
	svc := newService(t, &fakeDownloader{})
	rec, dataset := newOfflineRecord(t)
	rec.Pages[0].Flow = json.RawMessage(`"fixed"`)

	_, perr := svc.Run(t.Context(), rec, dataset)
	if perr == nil || perr.Status != 400 || perr.Code != "page_invalid" {
		t.Fatalf("期望 400/page_invalid, 实得 %+v", perr)
	}
}

// --- WritePages 产物落盘 ---

// TestWritePages_Layout 产物落盘 renders/<id>/<seq>.png(seq 1 起,spec §2.4 #7),
// DB 存相对 path(spec §2.2)
func TestWritePages_Layout(t *testing.T) {
	t.Chdir(t.TempDir())
	pages := []Page{
		{Frame: 0, Name: "主页", PNG: []byte("PNG1")},
		{Frame: 1, Name: "续页", PNG: []byte("PNG2")},
		{Frame: 1, Name: "续页", PNG: []byte("PNG3")},
	}
	images, perr := WritePages(pages, 7)
	if perr != nil {
		t.Fatalf("落盘失败: %s: %s", perr.Code, perr.Message)
	}
	wantPaths := []string{"renders/7/1.png", "renders/7/2.png", "renders/7/3.png"}
	for i, img := range images {
		if img.Path != wantPaths[i] || img.Frame != pages[i].Frame || img.Name != pages[i].Name {
			t.Fatalf("images[%d] = %+v, 期望 path %s", i, img, wantPaths[i])
		}
		b, err := os.ReadFile(wantPaths[i])
		if err != nil || !bytes.Equal(b, pages[i].PNG) {
			t.Fatalf("产物文件 %s 不符: %v", wantPaths[i], err)
		}
	}
}

// TestWritePages_OutputWriteFailed 落盘失败 → 500 render_output_write_failed
// (spec §2.5 服务端自有码)
func TestWritePages_OutputWriteFailed(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	if err := os.MkdirAll("renders", 0o755); err != nil {
		t.Fatal(err)
	}
	// renders/7 已被文件占位,MkdirAll 必败
	if err := os.WriteFile(filepath.Join("renders", "7"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, perr := WritePages([]Page{{Frame: 0, Name: "主页", PNG: []byte("PNG")}}, 7)
	if perr == nil || perr.Status != 500 || perr.Code != "render_output_write_failed" {
		t.Fatalf("期望 500/render_output_write_failed, 实得 %+v", perr)
	}
	if !strings.Contains(perr.Message, "renders") {
		t.Fatalf("message 应含落盘路径上下文: %s", perr.Message)
	}
}
