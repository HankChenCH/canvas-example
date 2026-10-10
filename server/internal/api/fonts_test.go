package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"example/server/internal/store"
)

// 字体清单端点:GET /api/fonts 下发语义与 LoadFonts 容错口径,语义论述见
// fonts.go 包注,此处只锁行为。

// newFontTestStore 字体端点不依赖 seed 数据,开空库即够
func newFontTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("开测试库: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestLoadFonts(t *testing.T) {
	t.Run("合法清单解析出条目", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "fonts.json")
		body := `[
			{"label": "思源黑体", "ref": "https://cdn.example.com/sans.otf"},
			{"label": "马善政楷书", "ref": "https://cdn.example.com/msz.ttf"}
		]`
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("写清单: %v", err)
		}
		fonts, err := LoadFonts(path)
		if err != nil {
			t.Fatalf("LoadFonts: %v", err)
		}
		if len(fonts) != 2 {
			t.Fatalf("条目数 = %d, 期望 2", len(fonts))
		}
		if fonts[0].Label != "思源黑体" || fonts[0].Ref != "https://cdn.example.com/sans.otf" {
			t.Fatalf("首条目 = %+v, 与清单不符", fonts[0])
		}
	})

	t.Run("文件缺失容忍返回空清单", func(t *testing.T) {
		fonts, err := LoadFonts(filepath.Join(t.TempDir(), "fonts.json"))
		if err != nil {
			t.Fatalf("缺失文件应容忍, 得 err = %v", err)
		}
		if len(fonts) != 0 {
			t.Fatalf("缺失文件应得空清单, 得 %d 条", len(fonts))
		}
	})

	t.Run("JSON 病态报错", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "fonts.json")
		if err := os.WriteFile(path, []byte("{not-json"), 0o644); err != nil {
			t.Fatalf("写清单: %v", err)
		}
		if _, err := LoadFonts(path); err == nil {
			t.Fatal("病态 JSON 应报错")
		}
	})
}

func TestListFonts(t *testing.T) {
	t.Run("下发配置清单", func(t *testing.T) {
		st := newFontTestStore(t)
		want := []FontEntry{
			{Label: "思源黑体 Regular", Ref: "https://cdn.example.com/sans-regular.otf"},
			{Label: "马善政楷书", Ref: "https://cdn.example.com/msz.ttf"},
		}
		h := New(st, want)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/fonts", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, 期望 200\n%s", rec.Code, rec.Body.String())
		}
		var got []FontEntry
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("响应非 JSON 数组: %v\n%s", err, rec.Body.String())
		}
		if len(got) != len(want) {
			t.Fatalf("条目数 = %d, 期望 %d\n%s", len(got), len(want), rec.Body.String())
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("第 %d 条 = %+v, 期望 %+v", i, got[i], want[i])
			}
		}
	})

	t.Run("未配置返回空数组而非 null", func(t *testing.T) {
		st := newFontTestStore(t)
		h := New(st, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/fonts", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, 期望 200\n%s", rec.Code, rec.Body.String())
		}
		if got := rec.Body.String(); got != "[]\n" {
			t.Fatalf("body = %q, 期望 \"[]\"", got)
		}
	})
}
