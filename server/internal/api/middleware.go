package api

import (
	"encoding/json"
	"errors"
	"net/http"
)

// withBodyLimit 全局请求体上限(spec §2.1);超限不在中间件直接拒绝,而是
// handler 侧读 body 时得到 *http.MaxBytesError → 413 request_too_large
// (见 decodeJSON)。GET 无体,包一层无副作用。
func withBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

// decodeJSON 哨兵错误(值即稳定 code,spec §2.5 骨架段)
var (
	errInvalidJSON     = errors.New(codeInvalidJSON)
	errRequestTooLarge = errors.New(codeRequestTooLarge)
)

// decodeJSON 严格解码请求体为 dst:单 JSON 值 + 无尾随内容。超限与坏 JSON
// 分别归 errRequestTooLarge / errInvalidJSON,由 writeDecodeError 落 HTTP。
func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return errRequestTooLarge
		}
		return errInvalidJSON
	}
	if dec.More() { // 尾随非空白内容同样视为坏 JSON
		return errInvalidJSON
	}
	return nil
}

// writeDecodeError 解码哨兵 → HTTP 映射(spec §2.5:413/400)
func writeDecodeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errRequestTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, codeRequestTooLarge, "请求体超过 10MB 上限")
	case errors.Is(err, errInvalidJSON):
		writeError(w, http.StatusBadRequest, codeInvalidJSON, "请求体不是合法 JSON")
	default:
		writeError(w, http.StatusInternalServerError, codeInternalError, err.Error())
	}
}
