// 页 flow 编辑纯逻辑（模板页序列工票 03，spec 页形状契约）：流链结构化草稿的
// 「条目下标 = 帧下标」平行数组模型退役——flow 成为页的属性（PageSlot.flow），wire
// 转换（flowFromWire/wireFlowOf）与增删同步随页序列自然消失，本模块只留 UI 呈现面：
// 页 flow 角色读数、页 wire 顶层表信息解析（rowsPath 提示）与本地校验（UI 拦截面）。
// 与 Vue/DOM 解耦，全部纯函数（宿主 EditorPage 只做响应式桥接）。
//
// 归一口径（宽容未知键、丢弃非法 flow）单点在 pages.ts flowFromWire：存储态经服务端
// 保存预检必然合法，归一只会修复、不丢语义；本地校验只是 UI 拦截面（保存钮禁用 +
// 行内报错），校验权威仍在服务端保存预检——表信息解析失败时表相关规则跳过。
import type { PageFlow } from './pages'

/** 页的链角色（结构化编辑器三态）：off = 无 flow 键 = 链外页（不吃行、照常进产物） */
export type PageFlowRole = 'off' | 'fixed' | 'paged'

/** 页 flow 角色读数：flow null = off（键缺省与 off 同义，spec 定案不设第三态） */
export function flowRoleOf(flow: PageFlow | null): PageFlowRole {
    return flow === null ? 'off' : flow.mode
}

/** 页 wire 的顶层表信息（rowsPath 提示与表相关校验的输入） */
export interface PageTableInfo {
    /** 顶层 TableLayer 总数（≥2 服务端 paginate_target_invalid，前端只提示） */
    tableCount: number
    /** 首个模板态表的 rowsPath（无模板态表为 null） */
    rowsPath: string | null
    /** 是否存在模板态表（template 键在场） */
    hasTemplate: boolean
}

/** 流链编辑器行摘要：label 供行标与错误消息指认（页名缺省「第 N 页」由宿主渲染），
 *  table 为 null = wire 解析失败（表相关校验跳过——前端尽力提示，权威在服务端） */
export interface PageFlowRowInfo {
    id: string
    label: string
    table: PageTableInfo | null
}

/** 流链编辑器行（校验与抽屉呈现共用的页序对齐数组行）：flow = 该页现值（null = 链外页） */
export interface PageFlowRow extends PageFlowRowInfo {
    flow: PageFlow | null
}

/** 本地校验（UI 拦截面）：双 paged、paged 非链尾、链上页无模板态顶层表、链内
 *  rowsPath 不一致。flows 与 pages 按页序对齐（flow 附着于页对象，无平行数组）。
 *  返回中文错误消息数组，空数组 = 可保存。 */
export function validatePageFlows(
    flows: ReadonlyArray<PageFlow | null>,
    pages: ReadonlyArray<PageFlowRowInfo>,
): string[] {
    const errors: string[] = []
    const label = (i: number) => pages[i]?.label || `第 ${i + 1} 页`
    const indicesOf = (mode: 'fixed' | 'paged') =>
        flows.map((f, i) => (f !== null && f.mode === mode ? i : -1)).filter((i) => i >= 0)
    const onChain = indicesOf('fixed').concat(indicesOf('paged')).sort((a, b) => a - b)
    const paged = indicesOf('paged')

    if (paged.length > 1) {
        errors.push(`paged 至多一个（${paged.map(label).join('、')} 同时为 paged）`)
    }
    if (paged.length === 1 && onChain.length > 0 && paged[0] !== onChain[onChain.length - 1]) {
        errors.push(`paged 必须在链尾（${label(paged[0]!)} 之后还有链上的 ${label(onChain[onChain.length - 1]!)}）`)
    }

    // 表相关规则：链上页须有模板态顶层表、链内 rowsPath 一致（链外页不限定）
    let chainRowsPath: string | null = null
    let chainRowsPathPage = -1
    for (const i of onChain) {
        const info = pages[i]?.table
        if (!info) continue
        if (info.tableCount === 0 || !info.hasTemplate) {
            errors.push(`${label(i)} 没有模板态顶层表格图层（链上页须恰有一张，且为模板态）`)
            continue
        }
        if (info.rowsPath === null) continue
        if (chainRowsPath === null) {
            chainRowsPath = info.rowsPath
            chainRowsPathPage = i
        } else if (chainRowsPath !== info.rowsPath) {
            errors.push(`链内 rowsPath 不一致（${label(chainRowsPathPage)}: ${chainRowsPath} vs ${label(i)}: ${info.rowsPath}）`)
        }
    }
    return errors
}

/** 页 wire（canonical graphJson 串）顶层表信息解析：尽力而为——非 JSON/结构怪异
 *  返回 null（调用方跳过提示与表校验）；rowsPath 取首个模板态表 */
export function pageTableInfo(graphJson: string): PageTableInfo | null {
    let wire: unknown
    try {
        wire = JSON.parse(graphJson)
    } catch {
        return null
    }
    if (typeof wire !== 'object' || wire === null) return null
    const layers = (wire as { layers?: unknown }).layers
    if (!Array.isArray(layers)) return null
    const info: PageTableInfo = { tableCount: 0, rowsPath: null, hasTemplate: false }
    for (const l of layers) {
        if (typeof l !== 'object' || l === null) continue
        const node = l as { type?: unknown; data?: unknown; template?: unknown }
        if (node.type !== 'TableLayer') continue
        info.tableCount += 1
        if (node.template === undefined || node.template === null) continue
        info.hasTemplate = true
        if (info.rowsPath === null) {
            const rowsPath = (node.data as { rowsPath?: unknown } | null | undefined)?.rowsPath
            if (typeof rowsPath === 'string') info.rowsPath = rowsPath
        }
    }
    return info
}
