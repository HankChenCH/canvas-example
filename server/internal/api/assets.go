// 资源上传端点(spec §2.4 #8):POST /api/assets,multipart 字段 file。
// 落盘 assets/u/<16位随机hex>.<原扩展名>,扩展名按 [A-Za-z0-9]{1,8} 白名单否则落
// bin,不校验图片类型;响应 url 前导斜杠形态(spec §2.2),可原样写入 graph spec.src。
package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// assetExtWhitelist 原文件名后缀白名单(spec §2.4 #8)
var assetExtWhitelist = regexp.MustCompile(`^[A-Za-z0-9]{1,8}$`)

// handleUploadAsset POST /api/assets → 201 {"url":"/assets/u/<16位hex>.<ext>"}
func (s *Server) handleUploadAsset(w http.ResponseWriter, r *http.Request) {
	// 流式解析(MultipartReader 不落内存缓冲):整体 10MB 上限由全局
	// MaxBytesReader 施加,读超限在 NextPart/io.Copy 处浮出(spec §2.1 含 multipart)
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, codeInvalidJSON, "请求须为 multipart/form-data(字段 file)")
		return
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, codeInvalidJSON, "multipart 请求缺少 file 字段")
			return
		}
		if err != nil {
			writeMultipartError(w, err)
			return
		}
		if part.FormName() != "file" {
			_ = part.Close()
			continue
		}
		name, err := saveAssetFile(part)
		_ = part.Close()
		if err != nil {
			writeMultipartError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"url": "/assets/u/" + name})
		return
	}
}

// writeMultipartError multipart 读写错误 → HTTP:超限 413,其余归 400 invalid_json
// (§2.5 请求体形态错误段,demo 粒度不另立码)
func writeMultipartError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, codeRequestTooLarge, "请求体超过 10MB 上限")
		return
	}
	writeError(w, http.StatusBadRequest, codeInvalidJSON, "multipart 请求解析失败: "+err.Error())
}

// saveAssetFile 流式落盘 assets/u/<16位随机hex>.<原扩展名>;写失败清理半成品。
// 随机名 O_EXCL 独占创建,碰撞防御性重试(16 位 hex 碰撞概率可忽略)。
// 落点目录就地补建(启动已建 assets/u,此处容错测试/异常 CWD)
func saveAssetFile(part *multipart.Part) (string, error) {
	ext := assetExtension(part.FileName())
	for {
		name := randomHex16() + ext
		path := filepath.Join("assets", "u", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", fmt.Errorf("建上传目录: %w", err)
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("创建上传文件: %w", err)
		}
		_, err = io.Copy(f, part)
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(path)
			return "", err
		}
		return name, nil
	}
}

// assetExtension 原文件名后缀白名单(spec §2.4 #8):[A-Za-z0-9]{1,8} 原样保留
// (多段后缀取最后一段),否则落 .bin;空文件名同此
func assetExtension(filename string) string {
	ext := strings.TrimPrefix(filepath.Ext(filename), ".")
	if assetExtWhitelist.MatchString(ext) {
		return "." + ext
	}
	return ".bin"
}

// randomHex16 16 位随机 hex(8 字节 crypto/rand;Read 自 Go 1.24 恒成功)
func randomHex16() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
