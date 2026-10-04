// Package paths:资源路径三合一归一(spec §2.2)。
package paths

import "strings"

// ToLocalPath 服务端本地资源引用归一:剥掉一个前导斜杠后按 CWD 相对读盘——
// URL/graph 引用串形态("/assets/u/x.png")与磁盘相对形态("assets/u/x.png")
// 落同一磁盘文件,两种形态双吃。渲染管线(13 票)与字体路径消费此规则。
func ToLocalPath(ref string) string {
	return strings.TrimPrefix(ref, "/")
}
