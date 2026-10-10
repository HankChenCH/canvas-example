// 字体清单(网络字体方案):seed/fonts.json 的 {label, ref} 白名单只读下发。
// 清单 ≠ 物化——服务端不代理、不下发字体字节;ref 是可直连的 https 字体文件
// 直链(TTF/OTF,Go 渲染端 opentype 可解析),浏览器 FontFace 预览与 Go 渲染端
// 远程物化(resolver cachedRemoteFile 下载进 .cache/)共用同一引用,graph 内
// 保存的 font 字段即该 URL,跨环境可移植。清单缺文件容忍(空清单,编辑器字体
// 字段退化为手输),JSON 病态属部署错误由调用方 fail-fast。
package api

import (
	"encoding/json"
	"net/http"
	"os"
)

// FontEntry 字体清单条目,与编辑器内核 FontCatalogEntry 同构
type FontEntry struct {
	// 展示名(下拉显示;与引用解耦,URL 不直接暴露给编辑者)
	Label string `json:"label"`
	// 字体引用(https 直链),与 TextLayer.font 同域
	Ref string `json:"ref"`
}

// LoadFonts 读字体清单 JSON;文件缺失返回空清单与 nil 错误(可选增强,不阻断),
// 其余读失败/JSON 病态返回错误
func LoadFonts(path string) ([]FontEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []FontEntry{}, nil
		}
		return nil, err
	}
	var fonts []FontEntry
	if err := json.Unmarshal(data, &fonts); err != nil {
		return nil, err
	}
	if fonts == nil {
		fonts = []FontEntry{}
	}
	return fonts, nil
}

// handleListFonts GET /api/fonts:200 + 配置清单(未配置 = [])
func (s *Server) handleListFonts(w http.ResponseWriter, _ *http.Request) {
	fonts := s.fonts
	if fonts == nil {
		fonts = []FontEntry{}
	}
	writeJSON(w, http.StatusOK, fonts)
}
