// 渲染终图纯逻辑（19 票，spec §4.6）：单张终图下载文件名 + 渲染历史版本标注 +
// 渲染促存链成算（渲染历史对比 / 草稿渲染促存链）。与 Vue/DOM 解耦（TDD 缝），
// EditorPage 与结果抽屉只做响应式桥接；HTTP 客户端与保存流经调用方注入
// （runPublishFlow 同款惯例，测试免 mock fetch）。
import { formatApiError } from '../api'
import type { RenderImage, RenderRecord } from '../api'

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

/** 渲染记录版本标注（渲染历史切换条/记录 meta 共用词汇）：null = draft 渲染或
 *  版本化前旧行（诚实语义，与服务端 renders.version NULL 同义），正整数 = vN */
export function renderVersionLabel(version: number | null): string {
    return version === null ? '草稿' : `v${version}`
}

export interface RenderFlowDeps {
    /** 文档级 dirty 现值（编排入口须先 computeDirty 同步即时比较，路由守卫同款
     *  口径）——dirty 渲染前先促存，渲的即所见 */
    dirty: boolean
    /** 保存流注入（现有 runDocumentSave 通道，含流链本地校验拦截面）：false =
     *  保存失败/中止——编排就此打住不渲染，错误面由保存步自带（内联状态栏反馈） */
    save: () => Promise<boolean>
    /** 渲染动作注入（api.renderTemplate 草稿哨兵）：失败以异常上抛，归稳定码在前
     *  可读文案 */
    render: () => Promise<RenderRecord>
}

export type RenderFlowResult =
    | { ok: true; record: RenderRecord; saved: boolean }
    | { ok: false; note: string | null }

/** 渲染促存链编排（对齐 runPublishFlow 促存先例）：dirty 时自动先走现有保存流、
 *  成功后渲草稿——「改了草稿渲染看效果」渲的即所见（否则结果对应旧草稿列，用户
 *  以为渲的是所改内容）。clean 直接渲染（不强制无谓 PUT）；保存失败（含流链本地
 *  校验不过）中止渲染，note 返回 null = 不覆盖保存步自带的错误回显；渲染打回
 *  （图病态 422 invalid_graph 等）归因稳定码在前文案。促存与发布促存同口径：
 *  保存写草稿列、不产生版本行，发布仍是显式动作。 */
export async function runRenderFlow(deps: RenderFlowDeps): Promise<RenderFlowResult> {
    let saved = false
    if (deps.dirty) {
        saved = await deps.save()
        if (!saved) return { ok: false, note: null }
    }
    try {
        return { ok: true, record: await deps.render(), saved }
    } catch (e) {
        return { ok: false, note: `渲染失败：${formatApiError(e)}` }
    }
}
