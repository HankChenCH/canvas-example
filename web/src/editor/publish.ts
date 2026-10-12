// 发布编排纯逻辑（template-draft-publish 批 B ② TDD 缝，spec §6 / 票 05）：
// 版本徽标推导 + 发布动作决策矩阵。与 Vue/DOM 解耦，HTTP 客户端与保存流经调用方
// 注入（newtemplate.ts 同款惯例，测试免 mock fetch）。
//
// 保存流不变声明（票 05 终裁）：编辑器永远打开草稿、保存 = 现有 PUT 通道、发布是
// 保存后的显式固化——dirty 自动促存是编排便利（「把我现在看到的定下来」，避免两步
// 制制造「以为发了、其实发的是旧内容」的陷阱），不是「发布即保存」的语义合并。
import { formatApiError } from '../api'
import type { PublishResult, TemplateVersionSummary } from '../api'

/** 最新已发布版本号（null = 未发布）：发布态永远是推导态非存储态（spec §3）。
 *  服务端保证 version 降序（最新在前），取 MAX 容错乱序、不依赖传输序。 */
export function latestVersionOf(versions: readonly TemplateVersionSummary[]): number | null {
    let max: number | null = null
    for (const v of versions) {
        if (max === null || v.version > max) max = v.version
    }
    return max
}

/** 版本徽标文案：「未发布」（空 versions = 未发布）或「vN」（最新已发布版本） */
export function versionBadgeOf(versions: readonly TemplateVersionSummary[]): string {
    const latest = latestVersionOf(versions)
    return latest === null ? '未发布' : `v${latest}`
}

/** 版本列表三态（评审修复 F1，spec §6）：徽标与抽屉共用的列表语义面——
 *  unknown = 列表未知（未加载或拉取失败，与徽标 v-if 退化同源的语义）；与空数组
 *  （unpublished = 未发布推导态，spec §3）语义分家：「不知道发没发过」不得冒充
 *  「没发过」。抽屉纯呈现，态判定单点在此（TDD 缝）。 */
export type VersionListState = 'unknown' | 'unpublished' | 'ready'

/** 版本抽屉列表态判定：null → unknown，空数组 → unpublished，非空 → ready */
export function versionListState(versions: readonly TemplateVersionSummary[] | null): VersionListState {
    if (versions === null) return 'unknown'
    return versions.length === 0 ? 'unpublished' : 'ready'
}

export interface PublishFlowDeps {
    /** 文档级 dirty 现值（编排入口须先 computeDirty 同步即时比较——防抖窗口内的
     *  最近编辑也要计入，路由守卫同款口径） */
    dirty: boolean
    /** 保存流注入（现有 runDocumentSave 通道，含流链本地校验拦截面）：false =
     *  保存失败/中止——编排就此打住，错误面由保存步自带（内联状态栏反馈） */
    save: () => Promise<boolean>
    /** 发布动作注入（api.publishTemplate）：422 invalid_graph / 404 等以异常上抛，
     *  由本编排归因为稳定码在前的可读文案 */
    publish: () => Promise<PublishResult>
}

export type PublishFlowResult =
    | { ok: true; version: number; duplicate: boolean; note: string; saved: boolean }
    | { ok: false; note: string | null }

/** 发布编排（票 05 终裁）：dirty 时自动先走现有保存流、成功后接着 publish——一次
 *  动作完成；clean 直接 publish（不强制无谓 PUT）。保存失败（含流链本地校验不过）
 *  中止发布，note 返回 null = 不覆盖保存步自带的错误回显（保留真实肇因）；发布
 *  打回（图病态 422 invalid_graph 等）归因稳定码在前文案。duplicate:true 是幂等
 *  跳过、同样算发布成功（徽标照常刷新）。 */
export async function runPublishFlow(deps: PublishFlowDeps): Promise<PublishFlowResult> {
    let saved = false
    if (deps.dirty) {
        saved = await deps.save()
        if (!saved) return { ok: false, note: null }
    }
    try {
        const result = await deps.publish()
        const duplicate = result.duplicate === true
        return {
            ok: true,
            version: result.version,
            duplicate,
            note: duplicate ? `内容与 v${result.version} 一致，跳过重复发布` : `已发布 v${result.version}`,
            saved,
        }
    } catch (e) {
        return { ok: false, note: `发布失败：${formatApiError(e)}` }
    }
}
