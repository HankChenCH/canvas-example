package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestProbeFont_RealSeedFont 真实 seed 字体探针通过(顺带守卫 seed 完整性)
func TestProbeFont_RealSeedFont(t *testing.T) {
	if err := probeFont(filepath.Join("seed", "assets", "fonts", "NotoSansSC-Regular.otf")); err != nil {
		t.Fatalf("真实字体探针应通过: %v", err)
	}
}

func TestProbeFont_MissingFails(t *testing.T) {
	err := probeFont(filepath.Join(t.TempDir(), "absent.otf"))
	if err == nil {
		t.Fatal("字体缺失应 fail-fast")
	}
	if !strings.Contains(err.Error(), "字体启动预检失败") {
		t.Fatalf("错误信息应含预检上下文: %v", err)
	}
}

func TestProbeFont_CorruptFails(t *testing.T) {
	// 非 sfnt 字节(magic bytes 分派不中)必须探针失败,不静默降级
	p := filepath.Join(t.TempDir(), "fake.otf")
	if err := os.WriteFile(p, []byte("definitely not a font"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := probeFont(p); err == nil {
		t.Fatal("损坏字体应 fail-fast")
	}
}
