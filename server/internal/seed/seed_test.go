package seed

import (
	"encoding/json"
	"errors"
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
	if content.Name != DefaultTemplateName {
		t.Fatalf("默认模板名 = %q, 期望 %q", content.Name, DefaultTemplateName)
	}
	if len(content.Canvases) != 2 || content.Canvases[0].Name != "主页" || content.Canvases[1].Name != "续页" {
		t.Fatalf("帧名应为 主页/续页: %+v", content.Canvases)
	}
	for _, c := range content.Canvases {
		if !json.Valid(c.Graph) {
			t.Fatalf("帧 %s graph 非法 JSON", c.Name)
		}
	}
	var chain []map[string]any
	if err := json.Unmarshal(content.FlowChain, &chain); err != nil || len(chain) != 2 {
		t.Fatalf("flowChain 应为两节点数组: %s (%v)", content.FlowChain, err)
	}
	if content.DatasetSchema == nil || content.Dataset == nil {
		t.Fatal("seed 必含 datasetSchema 与 dataset(spec §5.2)")
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
