package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// decodeRoute 测试专用「吃 body」路由:本票对外面只有 GET 读端点,413/400 的
// 端到端对号由 12 票 POST/PUT 端点承接;此处直接压 decodeJSON 契约本身
func decodeRoute() http.Handler {
	return withBodyLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v any
		if err := decodeJSON(r, &v); err != nil {
			writeDecodeError(w, err)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
}

func postBody(t *testing.T, h http.Handler, body []byte) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/whatever", bytes.NewReader(body)))
	var env map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec.Code, env
}

func TestDecode_OverLimit413(t *testing.T) {
	h := decodeRoute()
	// 10MB 上限(spec §2.1):体做成未闭合 JSON 字符串——语法上始终合法在途,
	// 解码器只能读到体尽才失败,超限由 MaxBytesError 触发(而非语法错先到)
	body := append([]byte(`{"pad":"`), bytes.Repeat([]byte("a"), 10<<20+1)...)
	code, env := postBody(t, h, body)
	if code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, 期望 413", code)
	}
	errObj := env["error"].(map[string]any)
	if errObj["code"] != "request_too_large" {
		t.Fatalf("code = %v, 期望 request_too_large", errObj["code"])
	}
}

func TestDecode_ExactlyLimitOK(t *testing.T) {
	h := decodeRoute()
	// 恰好 10MB 的完整闭合 JSON:不超限也不触发语法错,应正常通过
	payload := `{"pad":"` + strings.Repeat("a", 10<<20-10) + `"}`
	if len(payload) != 10<<20 {
		t.Fatalf("构造负载长度 = %d, 期望 %d", len(payload), 10<<20)
	}
	code, _ := postBody(t, h, []byte(payload))
	if code != http.StatusOK {
		t.Fatalf("恰好 10MB 应通过,status = %d", code)
	}
}

func TestDecode_InvalidJSON400(t *testing.T) {
	h := decodeRoute()
	for name, body := range map[string]string{
		"语法错误":    `{"name":`,
		"纯文本":     "not json",
		"空 body":  "",
		"尾随垃圾":    `{} trailing`,
		"两个 JSON": `{}{}`,
	} {
		code, env := postBody(t, h, []byte(body))
		if code != http.StatusBadRequest {
			t.Fatalf("[%s] status = %d, 期望 400", name, code)
		}
		errObj := env["error"].(map[string]any)
		if errObj["code"] != "invalid_json" {
			t.Fatalf("[%s] code = %v, 期望 invalid_json", name, errObj["code"])
		}
	}
}

func TestDecode_ValidJSONPasses(t *testing.T) {
	h := decodeRoute()
	code, _ := postBody(t, h, []byte(`{"name":"模板"}`))
	if code != http.StatusOK {
		t.Fatalf("合法 JSON 应通过,status = %d", code)
	}
}
