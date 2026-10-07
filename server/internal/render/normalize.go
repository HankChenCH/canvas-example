// 资源引用归一(spec §2.2 服务端归一规则,21 票):上传通道产出的前导斜杠
// 引用("/assets/u/x.png")与磁盘相对形态("assets/u/x.png")服务端双吃——
// 解析本地资源引用前剥掉一个前导 / 再按 CWD 相对读盘。
package render

import (
	"net/url"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/resolver"

	"example/server/internal/paths"
)

// normalizePages 渲染前把各页画布的本地资源引用归一为磁盘相对形态:对非远程
// 引用把 paths.ToLocalPath 结果回写图层物化槽(SetResolvedSrc/SetResolvedFont),
// 渲染端 DrawImage/DrawText 经 ResolvedSrc/ResolvedFont 读到的即是落盘路径。
//
// 时机在编译(hydrate 求值)之后:字面引用与表达式求值产物(dataset 行内上传
// url,hydrate 改写 wire 后 FromGraph 重建产生)同一覆盖。容器下钻表→行→
// 单元格→内容与 resolver.ResolveLayer 同构——本步补的正是 resolver 对本地
// 路径「直接使用、不回写」刻意留给宿主的空档,物化槽回写位语义与远程物化一致
// (layers 的 setter 标注「仅供 ResourceResolver 使用」对齐 PHP @internal,
// 本步以组装层物化编排者身份行使同一职责,非业务方改图)。
//
// 只看图层原始引用(rawImg/font):远程 URL 不动(交回惰性物化下载),物化
// 缓存路径(.cache/… 相对形态)天然不被剥斜杠,不存在误伤面。
func normalizePages(pages []*canvas.Canvas) {
	for _, page := range pages {
		for _, l := range page.GetLayers() {
			normalizeLayer(l)
		}
	}
}

// normalizeLayer 递归归一单图层:图片/字体叶子就地回写,容器下钻;未知类型
// 跳过(与 ResolveLayer 对外部图层类型同款容错)
func normalizeLayer(l layer.Layer) {
	switch t := l.(type) {
	case *layer.TableLayer:
		for _, row := range t.Rows() {
			normalizeLayer(row)
		}
	case *layer.TableRowLayer:
		for _, cell := range t.Cells() {
			normalizeLayer(cell)
		}
	case *layer.TableCellLayer:
		if content := t.ContentLayer(); content != nil {
			normalizeLayer(content)
		}
	case *layer.ImageLayer:
		if src := t.Image(); src != nil && *src != "" && !isRemoteURL(*src) {
			t.SetResolvedSrc(paths.ToLocalPath(*src))
		}
	case *layer.TextLayer:
		// 空串/纯数字 = 渲染端内置默认字体语义(resolver.materializeFont 同款
		// 跳过口径),不归一
		if font := t.Font(); font != "" && !resolver.IsNumeric(font) && !isRemoteURL(font) {
			t.SetResolvedFont(paths.ToLocalPath(font))
		}
	}
}

// isRemoteURL 远程 URL 判定:对齐 go-canvas resolver 的同款口径——凡可解析出
// scheme 与 host 即远程,其余一律按本地引用归一
func isRemoteURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}
