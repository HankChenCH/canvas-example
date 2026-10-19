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
)

// TestLoadRealSeed 以仓库内真实 seed(05 票 fixture 逐字节拷入)为对象:
// 同时是 seed 目录完整性的回归哨兵
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
	if len(tmpl.Canvases) != 2 || tmpl.Canvases[0].Name != "主页" || tmpl.Canvases[1].Name != "续页" {
		t.Fatalf("帧名应为 主页/续页: %+v", tmpl.Canvases)
	}
	for _, c := range tmpl.Canvases {
		if !json.Valid(c.Graph) {
			t.Fatalf("帧 %s graph 非法 JSON", c.Name)
		}
	}
	var chain []map[string]any
	if err := json.Unmarshal(tmpl.FlowChain, &chain); err != nil || len(chain) != 2 {
		t.Fatalf("flowChain 应为两节点数组: %s (%v)", tmpl.FlowChain, err)
	}
	// 模板只持引用:绑定 id 由启动序接线,Load 不产引用
	if tmpl.DataSourceID != nil {
		t.Fatalf("Load 不得预设 data_source_id: %v", tmpl.DataSourceID)
	}
}

// TestLoadRealSeed_FourFitShowcase 图片层适配四档同画布对照(image-fit-fill-tile
// 06 票,ADR 0023/0024):主页帧四档 fit 各一——徽标 cover 缺省不落键、照片格
// contain、装饰纹理带 fill、防伪水印 tile;fit 条件写键落键殿后(shape/align/
// position/fit)。fill/tile 素材是静态引用,内在尺寸即 tile 平铺单元(1:1 零参数),
// PNG 头尺寸一并钉死
func TestLoadRealSeed_FourFitShowcase(t *testing.T) {
	content, err := Load("../../seed")
	if err != nil {
		t.Fatalf("Load 真实 seed: %v", err)
	}
	if len(content.Template.Canvases) != 2 {
		t.Fatalf("应有两帧: %d", len(content.Template.Canvases))
	}
	var main map[string]any
	if err := json.Unmarshal(content.Template.Canvases[0].Graph, &main); err != nil {
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

func TestLoad_MissingFileFails(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(dir); err == nil {
		t.Fatal("缺 fixture 文件应报错(fail-fast)")
	}
}

func TestLoad_InvalidJSONFails(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"frame-main.json", "frame-continuation.json", "flow-chain.json", "dataset.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{}`), 0o644); err != nil {
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
