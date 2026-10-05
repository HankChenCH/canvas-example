// 渲染终图纯逻辑（19 票，spec §4.6）：单张终图下载文件名。与 Vue/DOM 解耦
// （TDD 缝），EditorPage 与结果抽屉只做响应式桥接。
import type { RenderImage } from '../api'

/** 文件名非法字符折叠（下载建议名共用口径；预览导出 previewFileName 同消费） */
export function sanitizeFileBase(raw: string): string {
    return raw.replace(/[/\\:*?"<>|]/g, '_').trim()
}

/** 单张终图下载文件名（结果抽屉 `<a download>` 建议名）：模板名-终图-页序-帧名
 *  ——页序取 images 扁平下标（帧序×页序，1 起），同帧多页不重名且按页序可排序；
 *  url 同源（/renders）浏览器直接采纳建议名。模板名空落 template 兜底、帧名空
 *  落 帧N 兜底（服务端帧名恒在，此处只防展示侧意外）。 */
export function renderImageFileName(templateName: string, image: RenderImage, index: number): string {
    const base = sanitizeFileBase(templateName) || 'template'
    const name = sanitizeFileBase(image.name) || `帧${image.frame}`
    return `${base}-终图-${index + 1}-${name}.png`
}
