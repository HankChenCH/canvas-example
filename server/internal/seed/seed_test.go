package seed

import (
	"bytes"
	"encoding/json"
	"errors"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example/server/internal/pages"
)

// TestLoadRealSeed 以仓库内真实 seed(pages fixture)为对象:同时是 seed 目录
// 完整性的回归哨兵
func TestLoadRealSeed(t *testing.T) {
	content, err := Load("../../seed")
	if err != nil {
		t.Fatalf("Load 真实 seed: %v", err)
	}
	if content.DataSource.Name != DefaultDataSourceName {
		t.Fatalf("默认数据源名 = %q, 期望 %q", content.DataSource.Name, DefaultDataSourceName)
	}
	if content.DataSource.Schema == nil || content.DataSource.Data == nil {
		t.Fatal("seed 必含数据源 schema 与 data(spec §5.2;数据源是独立实体)")
	}
	tmpl := content.Template
	if tmpl.Name != DefaultTemplateName {
		t.Fatalf("默认模板名 = %q, 期望 %q", tmpl.Name, DefaultTemplateName)
	}
	// 页序列形状(ADR 0025):页 id 落 fixture、页名 主页/续页、flow 形状
	// fixed / paged+omitIfEmpty 与翻转前流链一致(flow 按语义比对,fixture 为
	// 缩进排版不钉空白)
	if len(tmpl.Pages) != 2 {
		t.Fatalf("seed 应两页: %d", len(tmpl.Pages))
	}
	flowIs := func(i int, want string) bool {
		var buf bytes.Buffer
		if err := json.Compact(&buf, tmpl.Pages[i].Flow); err != nil {
			return false
		}
		return buf.String() == want
	}
	var ids []string
	for i, want := range []struct {
		name string
		flow string
	}{{"主页", `{"mode":"fixed"}`}, {"续页", `{"mode":"paged","omitIfEmpty":true}`}} {
		p := tmpl.Pages[i]
		var name string
		if err := json.Unmarshal(p.Name, &name); err != nil || name != want.name {
			t.Fatalf("页 %d name = %s, 期望 %q (%v)", i, p.Name, want.name, err)
		}
		var id string
		if err := json.Unmarshal(p.ID, &id); err != nil || id == "" {
			t.Fatalf("页 %d id 须为非空字符串(页 id 落 fixture): %s (%v)", i, p.ID, err)
		}
		ids = append(ids, id)
		if ids[0] == ids[len(ids)-1] && i == 1 {
			t.Fatalf("页 id 页内重复: %v", ids)
		}
		if !flowIs(i, want.flow) {
			t.Fatalf("页 %d flow = %s, 期望 %s", i, p.Flow, want.flow)
		}
		if !json.Valid(p.Graph) {
			t.Fatalf("页 %s graph 非法 JSON", name)
		}
	}
	// 模板只持引用:绑定 id 由启动序接线,Load 不产引用
	if tmpl.DataSourceID != nil {
		t.Fatalf("Load 不得预设 data_source_id: %v", tmpl.DataSourceID)
	}
	// 编译哨兵:seed 页序列应过编译边界(种子模板直接就是可渲染的页形状)
	if _, verr := pages.Compile(tmpl.Pages); verr != nil {
		t.Fatalf("seed 页序列应过编译: %v", verr)
	}
}

// TestLoadRealSeed_FourFitShowcase 图片层适配四档同画布对照(image-fit-fill-tile
// 06 票,ADR 0023/0024):主页四档 fit 各一——徽标 cover 缺省不落键、照片格
// contain、装饰纹理带 fill、防伪水印 tile;fit 条件写键落键殿后(shape/align/
// position/fit)。fill/tile 素材是静态引用,内在尺寸即 tile 平铺单元(1:1 零参数),
// PNG 头尺寸一并钉死
func TestLoadRealSeed_FourFitShowcase(t *testing.T) {
	content, err := Load("../../seed")
	if err != nil {
		t.Fatalf("Load 真实 seed: %v", err)
	}
	if len(content.Template.Pages) != 2 {
		t.Fatalf("应有两页: %d", len(content.Template.Pages))
	}
	var main map[string]any
	if err := json.Unmarshal(content.Template.Pages[0].Graph, &main); err != nil {
		t.Fatal(err)
	}
	layers := main["layers"].([]any)
	byName := func(name string) map[string]any {
		for _, raw := range layers {
			l := raw.(map[string]any)
			if l["name"] == name {
				return l
			}
		}
		t.Fatalf("主页帧缺图层 %q", name)
		return nil
	}

	// cover:机构徽标(72×72)不落 fit 键 = 缺省裁切对照
	badge := byName("机构徽标")["spec"].(map[string]any)
	if _, has := badge["fit"]; has {
		t.Fatal("徽标层不落 fit 键(cover 缺省,条件写键)")
	}
	// contain:照片格(表格行内表达式图片)两帧同构,此处钉主页
	cell := byName("证书卡片区")["template"].(map[string]any)["cells"].([]any)[0].(map[string]any)
	if got := cell["content"].(map[string]any)["spec"].(map[string]any)["fit"]; got != "contain" {
		t.Fatalf("照片格 fit = %v, 期望 contain", got)
	}

	// fill:装饰纹理带(794×70 @ (0,120))——纹理/渐变素材进固定占位框,
	// 比例失真不敏感(192×192 方图拉伸进 11.3:1 横带)
	band := byName("装饰纹理带")
	bandSpec := band["spec"].(map[string]any)
	if got := bandSpec["fit"]; got != "fill" {
		t.Fatalf("装饰纹理带 fit = %v, 期望 fill", got)
	}
	shape := bandSpec["shape"].(map[string]any)
	if shape["width"] != float64(794) || shape["height"] != float64(70) {
		t.Fatalf("装饰纹理带盒 = %vx%v, 期望 794x70", shape["width"], shape["height"])
	}
	bandPos := bandSpec["position"].(map[string]any)
	if bandPos["x"] != float64(0) || bandPos["y"] != float64(120) {
		t.Fatalf("装饰纹理带位置 = (%v,%v), 期望 (0,120)", bandPos["x"], bandPos["y"])
	}
	if got := band["data"].(map[string]any)["value"]; got != "assets/u/texture-fill.png" {
		t.Fatalf("装饰纹理带静态引用 = %v", got)
	}

	// tile:防伪水印(714×800 @ (40,190),priority 40 压表上层——绘制序在表之后,
	// 半透明水印盖表不夺文字;left/top 锚定全铺、边缘半块截断)
	mark := byName("防伪水印")
	markSpec := mark["spec"].(map[string]any)
	if got := markSpec["fit"]; got != "tile" {
		t.Fatalf("防伪水印 fit = %v, 期望 tile", got)
	}
	if got := markSpec["align"].(map[string]any); got["horizontal"] != "left" || got["vertical"] != "top" {
		t.Fatalf("防伪水印对齐 = %v, 期望 left/top(首块锚定盒左上,全铺无前缘空隙)", got)
	}
	markShape := markSpec["shape"].(map[string]any)
	if markShape["width"] != float64(714) || markShape["height"] != float64(800) {
		t.Fatalf("防伪水印盒 = %vx%v, 期望 714x800", markShape["width"], markShape["height"])
	}
	markPos := markSpec["position"].(map[string]any)
	if markPos["x"] != float64(40) || markPos["y"] != float64(190) {
		t.Fatalf("防伪水印位置 = (%v,%v), 期望 (40,190)", markPos["x"], markPos["y"])
	}
	if got := mark["priority"]; got != float64(40) {
		t.Fatalf("防伪水印 priority = %v, 期望 40(压表上层的设计点)", got)
	}
	if got := mark["data"].(map[string]any)["value"]; got != "assets/u/watermark-tile.png" {
		t.Fatalf("防伪水印静态引用 = %v", got)
	}

	// 两块新素材随 seed 分发,内在尺寸 = tile 平铺单元(1:1 零参数)
	for name, want := range map[string][2]int{
		"texture-fill.png":   {192, 192},
		"watermark-tile.png": {128, 128},
	} {
		b, err := os.ReadFile(filepath.Join("../../seed/assets/u", name))
		if err != nil {
			t.Fatalf("showcase 素材缺失 %s: %v", name, err)
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("%s 不是合法 PNG: %v", name, err)
		}
		if cfg.Width != want[0] || cfg.Height != want[1] {
			t.Fatalf("%s 内在尺寸 = %dx%d, 期望 %dx%d", name, cfg.Width, cfg.Height, want[0], want[1])
		}
	}
}

// TestLoadRealSeed_CirclePhotoFrame 圆形照片框 showcase(image-clip-shape 06 票,
// ADR 0026):两页照片格 content 图片层 clip=circle——真实圆形学员头像位,非方盒
// (160×400)内切正圆 + 环带描边(四边 3px 同色,渲染端按 min 宽/top 首色取参);
// clip 条件写键殿后 spec 末位 fit 之后。徽标层不落 clip 键(矩形缺省对照)。
// fixture 假圆随之退役:学员照换 320×320 方形人像,行 1 横幅 320×160 保留错形
// 对照(contain 进圆内 letterbox 露层背景)
func TestLoadRealSeed_CirclePhotoFrame(t *testing.T) {
	content, err := Load("../../seed")
	if err != nil {
		t.Fatalf("Load 真实 seed: %v", err)
	}
	for i, page := range content.Template.Pages {
		var graph struct {
			Layers []map[string]any `json:"layers"`
		}
		if err := json.Unmarshal(page.Graph, &graph); err != nil {
			t.Fatal(err)
		}
		var table map[string]any
		for _, l := range graph.Layers {
			if l["name"] == "证书卡片区" {
				table = l
				break
			}
		}
		if table == nil {
			t.Fatalf("页 %d 缺证书卡片区", i)
		}
		cell := table["template"].(map[string]any)["cells"].([]any)[0].(map[string]any)
		spec := cell["content"].(map[string]any)["spec"].(map[string]any)

		// clip 条件写键:圆形档落键值 circle
		if got := spec["clip"]; got != "circle" {
			t.Fatalf("页 %d 照片格 clip = %v, 期望 circle", i, got)
		}
		if got := spec["fit"]; got != "contain" {
			t.Fatalf("页 %d 照片格 fit = %v, 期望 contain(先绘后裁,两声明正交)", i, got)
		}
		// 键序殿后:compact 后 fit 与 clip 相邻(shape/align/position/fit/clip)
		var compact bytes.Buffer
		if err := json.Compact(&compact, page.Graph); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(compact.String(), `"fit":"contain","clip":"circle"`) {
			t.Fatalf("页 %d 照片格 clip 键未殿后 fit 之后: %s", i, compact.String()[:min(200, compact.Len())])
		}
		// 环带描边:四边 3px 同色(渲染端解析 min 宽/top 首色)
		border := spec["shape"].(map[string]any)["border"].(map[string]any)
		for _, side := range []string{"top", "right", "bottom", "left"} {
			b := border[side].(map[string]any)
			if b["width"] != float64(3) || b["color"] != "#1f4e79" {
				t.Fatalf("页 %d 照片格 border.%s = %v, 期望 {3,#1f4e79}(圆周环带)", i, side, b)
			}
		}
		// 完整圆形框其余两步的声明:层背景填圆、对齐 center/center 定内切圆心
		if got := spec["shape"].(map[string]any)["backgroundColor"]; got != "#e8ecef" {
			t.Fatalf("页 %d 照片格层背景 = %v, 期望 #e8ecef(圆内 letterbox 底色)", i, got)
		}
		align := spec["align"].(map[string]any)
		if align["horizontal"] != "center" || align["vertical"] != "center" {
			t.Fatalf("页 %d 照片格对齐 = %v, 期望 center/center(内切圆落位)", i, align)
		}
	}

	// 徽标层不落 clip 键:矩形缺省对照(clip 条件写键;适用域 = 徽标本层)
	var main struct {
		Layers []map[string]any `json:"layers"`
	}
	if err := json.Unmarshal(content.Template.Pages[0].Graph, &main); err != nil {
		t.Fatal(err)
	}
	for _, l := range main.Layers {
		if l["name"] == "机构徽标" {
			if _, has := l["spec"].(map[string]any)["clip"]; has {
				t.Fatal("徽标层不落 clip 键(矩形缺省,条件写键)")
			}
		}
	}

	// fixture 假圆退役:学员照方形人像 320×320(×5),行 1 横幅 320×160
	for name, want := range map[string][2]int{
		"student-1.png":    {320, 320},
		"student-2.png":    {320, 320},
		"student-3.png":    {320, 320},
		"student-4.png":    {320, 320},
		"student-5.png":    {320, 320},
		"banner-photo.png": {320, 160},
	} {
		b, err := os.ReadFile(filepath.Join("../../seed/assets/u", name))
		if err != nil {
			t.Fatalf("照片 fixture 缺失 %s: %v", name, err)
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("%s 不是合法 PNG: %v", name, err)
		}
		if cfg.Width != want[0] || cfg.Height != want[1] {
			t.Fatalf("%s 内在尺寸 = %dx%d, 期望 %dx%d", name, cfg.Width, cfg.Height, want[0], want[1])
		}
	}
}

func TestLoad_MissingFileFails(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(dir); err == nil {
		t.Fatal("缺 fixture 文件应报错(fail-fast)")
	}
}

func TestLoad_InvalidJSONFails(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"pages.json", "dataset.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`[]`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "dataset-schema.json"), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "dataset-schema.json") {
		t.Fatalf("应报 dataset-schema.json 非法 JSON, 实得 %v", err)
	}
}

// TestLoad_PagesShapeFails pages.json 非页对象数组 fail-fast(结构错误不静默
// 播种;页级形状裁决归编译边界,此处只守 JSON 结构)
func TestLoad_PagesShapeFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pages.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dataset-schema.json", "dataset.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "pages.json") {
		t.Fatalf("应报 pages.json 须为页对象数组, 实得 %v", err)
	}
}

// makeFakeSeedAssets 造一棵小型 seed/assets 树(不拉 16MB 真字体)
func makeFakeSeedAssets(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("fonts/regular.otf", "FONT")
	write("u/student-1.png", "PNG1")
	write("u/student-2.png", "PNG2")
	return root
}

func TestCopyAssets_FillsAndIdempotent(t *testing.T) {
	src := makeFakeSeedAssets(t)
	dst := filepath.Join(t.TempDir(), "assets")

	copied, err := CopyAssets(src, dst)
	if err != nil {
		t.Fatalf("首次补齐: %v", err)
	}
	if copied != 3 {
		t.Fatalf("首次补齐文件数 = %d, 期望 3", copied)
	}
	// 逐字节一致
	for _, rel := range []string{"fonts/regular.otf", "u/student-1.png", "u/student-2.png"} {
		b, err := os.ReadFile(filepath.Join(dst, rel))
		if err != nil {
			t.Fatalf("补齐产物缺失 %s: %v", rel, err)
		}
		want, _ := os.ReadFile(filepath.Join(src, rel))
		if string(b) != string(want) {
			t.Fatalf("%s 内容不一致", rel)
		}
	}
	// 幂等:重启再补齐,零拷贝
	copied, err = CopyAssets(src, dst)
	if err != nil {
		t.Fatalf("二次补齐: %v", err)
	}
	if copied != 0 {
		t.Fatalf("二次补齐文件数 = %d, 期望 0(已存在即跳过)", copied)
	}
}

func TestCopyAssets_DoesNotTouchUserUploads(t *testing.T) {
	src := makeFakeSeedAssets(t)
	dst := filepath.Join(t.TempDir(), "assets")
	if _, err := CopyAssets(src, dst); err != nil {
		t.Fatal(err)
	}
	// 用户上传的无关文件不在 seed 内,补齐不得删除/覆盖
	upload := filepath.Join(dst, "u", "upload-abc.png")
	if err := os.WriteFile(upload, []byte("USER"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CopyAssets(src, dst); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(upload); err != nil || string(b) != "USER" {
		t.Fatalf("用户上传文件被触碰: %v %q", err, b)
	}
}

func TestCopyAssets_MissingSourceFails(t *testing.T) {
	_, err := CopyAssets(filepath.Join(t.TempDir(), "nope"), filepath.Join(t.TempDir(), "assets"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("seed assets 缺失应报 NotExist, 实得 %v", err)
	}
}
