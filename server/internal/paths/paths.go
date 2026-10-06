// Package paths:资源路径三合一归一(spec §2.2)。
package paths

import "strings"

// ToLocalPath 服务端本地资源引用归一:剥掉一个前导斜杠后按 CWD 相对读盘——
// URL/graph 引用串形态("/assets/u/x.png")与磁盘相对形态("assets/u/x.png")
// 落同一磁盘文件,两种形态双吃。消费点 = 渲染管线的渲染前归一步
// (internal/render normalizePages,21 票接线):图片引用与字体路径同规。
func ToLocalPath(ref string) string {
	return strings.TrimPrefix(ref, "/")
}
