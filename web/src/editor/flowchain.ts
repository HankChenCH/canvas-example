// 结构化流链草稿纯逻辑（29 票，spec §4.2 流链编辑器）：流链的 UI 真值源从
// JSON 文本草稿换成「按帧下标对齐的结构化条目数组」（FlowChainDraft），wire
// FlowChainNode[]（spec §3.1 封闭 4 键）只在保存/比较时派生。与 Vue/DOM 解耦，
// 全部纯函数（宿主 EditorPage 只做响应式桥接，frames.ts 同款 TDD 缝）。
//
// 设计约束：
// - 条目数组下标即帧下标——增帧 = 尾部追加 off 条目、删帧 = 条目 splice，
//   旧「链节点下标重写」（27 票 rewriteFlowChainForDeletion）的升序保持与
//   paged 链尾保持由数组结构性保证，不再需要显式重排；
// - 归一口径（宽容未知键、丢弃越界/非法节点）单点在本模块：存储态链经服务端
//   保存预检必然合法，归一只会修复、不丢语义；
// - 本地校验只是 UI 拦截面（保存钮禁用 + 行内报错），校验权威仍在服务端
//   保存预检（spec §3.4）——表信息解析失败时表相关规则跳过。

/** 帧的链角色（结构化编辑器三态）：off = 不在链上（hydrate 全量直通） */
export type ChainRole = 'off' | 'fixed' | 'paged'

/** 单帧链条目：quota 仅 fixed 携带（null = 不携带键 = 容量型按可用区装），
 *  omitIfEmpty = 零行跳帧（false 时不携带键，wire 缺省 false 同义） */
export interface FrameChainEntry {
    role: ChainRole
    quota: number | null
    omitIfEmpty: boolean
}

/** 结构化流链草稿：条目按帧下标对齐，长度与帧缓冲同长（增删帧同步维护） */
export type FlowChainDraft = FrameChainEntry[]

/** 空条目（不在链上） */
export function emptyFrameChainEntry(): FrameChainEntry {
    return { role: 'off', quota: null, omitIfEmpty: false }
}

/** wire 链 → 结构化草稿：null/undefined/空数组 = 空链 = 全 off；节点宽容归一
 *  （未知键忽略、非法/越界节点丢弃），返回数组恒长 frameCount */
export function draftFromFlowChain(flowChain: unknown, frameCount: number): FlowChainDraft {
    const draft: FlowChainDraft = []
    for (let i = 0; i < frameCount; i += 1) draft.push(emptyFrameChainEntry())
    if (!Array.isArray(flowChain)) return draft
    for (const node of flowChain) {
        const parsed = entryFromNode(node)
        if (parsed === null || parsed.frame < 0 || parsed.frame >= frameCount) continue
        draft[parsed.frame] = parsed.entry
    }
    return draft
}

/** 结构化草稿 → wire 链：全 off 返回 null（空链与 null 同义，spec §3.1）；
 *  链上节点按帧下标升序（条目序天然保证），quota/omitIfEmpty 仅在有意义时输出 */
export function flowChainFromDraft(draft: readonly FrameChainEntry[]): unknown {
    const chain: FlowChainNodeWire[] = []
    draft.forEach((entry, frame) => {
        if (entry.role === 'off') return
        const node: FlowChainNodeWire = { frame, mode: entry.role }
        if (entry.role === 'fixed' && entry.quota !== null) node.quota = entry.quota
        if (entry.omitIfEmpty) node.omitIfEmpty = true
        chain.push(node)
    })
    return chain.length === 0 ? null : chain
}

/** 删帧草稿同步（spec §4.2 删帧的结构化承接）：条目 splice——被删帧出链、
 *  其余条目自动前移，升序与 paged 链尾由数组结构保持（合法链入 → 合法链出） */
export function deleteFrameFromDraft(draft: readonly FrameChainEntry[], index: number): FlowChainDraft {
    const next = [...draft]
    next.splice(index, 1)
    return next
}

/** 增帧草稿同步（spec §4.2 增帧）：尾部追加 off 条目——尾部追加既有下标无一失效 */
export function appendFrameToDraft(draft: readonly FrameChainEntry[]): FlowChainDraft {
    return [...draft, emptyFrameChainEntry()]
}

/** 帧 wire 的顶层表信息（rowsPath 提示与表相关校验的输入） */
export interface FrameTableInfo {
    /** 顶层 TableLayer 总数（≥2 服务端 paginate_target_invalid，前端只提示） */
    tableCount: number
    /** 首个模板态表的 rowsPath（无模板态表为 null） */
    rowsPath: string | null
    /** 是否存在模板态表（template 键在场） */
    hasTemplate: boolean
}

/** 校验用帧摘要：name 供错误消息指认，table 为 null = wire 解析失败（表相关
 *  校验跳过——前端尽力提示，权威在服务端） */
export interface FrameChainFrameInfo {
    name: string
    table: FrameTableInfo | null
}

/** 本地校验（UI 拦截面）：双 paged、paged 非链尾、链上帧无模板态顶层表、
 *  链内 rowsPath 不一致。返回中文错误消息数组，空数组 = 可保存。 */
export function validateFlowChainDraft(
    draft: readonly FrameChainEntry[],
    frames: readonly FrameChainFrameInfo[],
): string[] {
    const errors: string[] = []
    const label = (frame: number) => frames[frame]?.name || `帧 ${frame + 1}`
    const indicesOf = (role: ChainRole) => draft.map((e, i) => (e.role === role ? i : -1)).filter((i) => i >= 0)
    const onChain = indicesOf('fixed').concat(indicesOf('paged')).sort((a, b) => a - b)
    const paged = indicesOf('paged')

    if (paged.length > 1) {
        errors.push(`paged 至多一个（${paged.map(label).join('、')} 同时为 paged）`)
    }
    if (paged.length === 1 && onChain.length > 0 && paged[0] !== onChain[onChain.length - 1]) {
        errors.push(`paged 必须在链尾（${label(paged[0]!)} 之后还有链上的 ${label(onChain[onChain.length - 1]!)}）`)
    }

    // 表相关规则：链上帧须有模板态顶层表、链内 rowsPath 一致（链外帧不限定）
    let chainRowsPath: string | null = null
    let chainRowsPathFrame = -1
    for (const frame of onChain) {
        const info = frames[frame]?.table
        if (!info) continue
        if (info.tableCount === 0 || !info.hasTemplate) {
            errors.push(`${label(frame)} 没有模板态顶层表格图层（链上帧须恰有一张，且为模板态）`)
            continue
        }
        if (info.rowsPath === null) continue
        if (chainRowsPath === null) {
            chainRowsPath = info.rowsPath
            chainRowsPathFrame = frame
        } else if (chainRowsPath !== info.rowsPath) {
            errors.push(`链内 rowsPath 不一致（${label(chainRowsPathFrame)}: ${chainRowsPath} vs ${label(frame)}: ${info.rowsPath}）`)
        }
    }
    return errors
}

/** 帧 wire（canonical graphJson 串）顶层表信息解析：尽力而为——非 JSON/结构
 *  怪异返回 null（调用方跳过提示与表校验）；rowsPath 取首个模板态表 */
export function frameTableInfo(graphJson: string): FrameTableInfo | null {
    let wire: unknown
    try {
        wire = JSON.parse(graphJson)
    } catch {
        return null
    }
    if (typeof wire !== 'object' || wire === null) return null
    const layers = (wire as { layers?: unknown }).layers
    if (!Array.isArray(layers)) return null
    const info: FrameTableInfo = { tableCount: 0, rowsPath: null, hasTemplate: false }
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

/** wire 节点形态（仅本模块派生输出用；字段与 go-canvas paginate.FlowChainNode 对齐） */
interface FlowChainNodeWire {
    frame: number
    mode: string
    quota?: number
    omitIfEmpty?: boolean
}

/** 单节点宽容归一：frame 须整数字面量、mode 须 fixed|paged，否则 null（丢弃）；
 *  quota 仅非负整数落位（其余含 null 归 null = 不携带）、omitIfEmpty 仅 true 落位 */
function entryFromNode(node: unknown): { frame: number; entry: FrameChainEntry } | null {
    if (typeof node !== 'object' || node === null) return null
    const raw = node as Record<string, unknown>
    const frame = raw['frame']
    if (typeof frame !== 'number' || !Number.isInteger(frame)) return null
    const mode = raw['mode']
    if (mode !== 'fixed' && mode !== 'paged') return null
    const quotaRaw = raw['quota']
    const quota = typeof quotaRaw === 'number' && Number.isInteger(quotaRaw) && quotaRaw >= 0 ? quotaRaw : null
    return {
        frame,
        entry: {
            role: mode,
            quota: mode === 'fixed' ? quota : null,
            omitIfEmpty: raw['omitIfEmpty'] === true,
        },
    }
}
