// 工程数据快照宿主聚合纯逻辑（工单 04，spec §6）：快照组装（flush 侧）、快照宽容
// 解析（载入侧）、悬空帧键剔除。与 Vue/DOM 解耦，EditorPage 只做响应式接线。
// 分层纪律：切片/常量从内核导入，宿主只拼装不解释——frames 容器（帧名 → 切片）
// 归宿主，切片内容（guides/lockedPaths）原样透传，恢复权威在内核 restoreEditorState。
import { EDITOR_STATE_SCHEMA_VERSION } from '@hankchen/canvas-editor'
import type { CanvasEditorState, EditorPrefsState } from '@hankchen/canvas-editor'

import type { TemplateEditorStateSnapshot } from '../api'

/** 切片拷贝：数组与 guide 条目逐元素复制——快照自持，不与映射/内核共享可变对象 */
function copySlice(slice: CanvasEditorState): CanvasEditorState {
    return {
        guides: slice.guides.map((guide) => ({ ...guide })),
        lockedPaths: [...slice.lockedPaths],
    }
}

/** 组装完整快照（flush 侧）：帧名与切片按槽位序对齐入容器，无切片（null）的帧不
 *  产生键（恢复端缺键 = 全默认态）；重名帧后者覆盖前者（帧名非唯一键，spec §3 形状
 *  的既定代价）。frames/prefs 取拷贝——快照自持，与映射/内核后续变更隔离。 */
export function buildEditorStateSnapshot(params: {
    frameNames: readonly string[]
    slices: ReadonlyArray<CanvasEditorState | null | undefined>
    prefs: EditorPrefsState
}): TemplateEditorStateSnapshot {
    const frames: Record<string, CanvasEditorState> = {}
    params.frameNames.forEach((name, i) => {
        const slice = params.slices[i]
        if (name && slice) frames[name] = copySlice(slice)
    })
    return { schemaVersion: EDITOR_STATE_SCHEMA_VERSION, frames, prefs: { ...params.prefs } }
}

/** 宽容解析结果：frames 逐键轻校验后收集（非法键丢弃），prefs 双布尔齐才收
 *  （null = 不应用，内核自持默认/现值）；raw 本身非对象同落全默认态，不抛错 */
export interface ParsedEditorStateSnapshot {
    frames: Record<string, CanvasEditorState>
    prefs: EditorPrefsState | null
}

/** 快照宽容解析（载入侧，spec §3「宽容读、无迁移器」）：null/形状不合法 = 全默认态；
 *  不分支 schemaVersion（版本常量只为写侧标记，读侧未知键/未知版本一律忽略）；
 *  frames 内单帧形状不合法只丢该键、合法兄弟帧保留——与内核 restore 的「整体拒」
 *  同口径：guides/lockedPaths 两数组键齐才算切片，内容合法性由内核恢复端兜底。 */
export function parseTemplateEditorStateSnapshot(raw: unknown): ParsedEditorStateSnapshot {
    if (raw === null || typeof raw !== 'object' || Array.isArray(raw)) {
        return { frames: {}, prefs: null }
    }
    const obj = raw as Record<string, unknown>
    const frames: Record<string, CanvasEditorState> = {}
    const rawFrames = obj.frames
    if (typeof rawFrames === 'object' && rawFrames !== null && !Array.isArray(rawFrames)) {
        for (const [name, value] of Object.entries(rawFrames)) {
            if (isCanvasEditorStateShape(value)) frames[name] = copySlice(value)
        }
    }
    const prefs = isEditorPrefsShape(obj.prefs)
        ? {
              rulersVisible: obj.prefs.rulersVisible,
              anchorExpanded: obj.prefs.anchorExpanded,
              // 新层样式槽宽容透传（editor 0.5.0）：旧快照缺字段/畸形值由内核
              // applyEditorPrefs 逐字段收口（pickNextTextStylePatch 不符保留现值），宿主不校验
              nextTextStyle: obj.prefs.nextTextStyle,
              // 建层比例槽宽容透传（editor 0.6.0）：同上逐字段收口（pickNextLayerRatio
              // 非六档枚举值回落 'free'），旧快照缺字段即回默认自由档
              nextLayerRatio: obj.prefs.nextLayerRatio,
          }
        : null
    return { frames, prefs }
}

/** 切片轻结构校验（对齐内核 parseCanvasEditorState 语义：两数组键齐 = 形状合法，
 *  guide 条目不逐项校验——id 无持久语义、重取号在内核） */
function isCanvasEditorStateShape(value: unknown): value is CanvasEditorState {
    return (
        typeof value === 'object' &&
        value !== null &&
        Array.isArray((value as CanvasEditorState).guides) &&
        Array.isArray((value as CanvasEditorState).lockedPaths)
    )
}

/** 偏好轻结构校验：双布尔键齐才整体收（applyEditorPrefs 整体替换，半键会写入
 *  undefined——宁可整桶不应用，内核自持现值） */
function isEditorPrefsShape(value: unknown): value is EditorPrefsState {
    return (
        typeof value === 'object' &&
        value !== null &&
        typeof (value as EditorPrefsState).rulersVisible === 'boolean' &&
        typeof (value as EditorPrefsState).anchorExpanded === 'boolean'
    )
}

/** 悬空帧键剔除（载入侧第二缝，spec §3「帧改名 → 旧键悬空，由宽容读丢弃」）：只取
 *  现役帧名对应的切片——快照里帧改名/删帧遗留的键不出现在会话内存映射，随下次
 *  flush 自然消失。 */
export function pickFrameStates(
    frames: Readonly<Record<string, CanvasEditorState>>,
    frameNames: readonly string[],
): Record<string, CanvasEditorState> {
    const picked: Record<string, CanvasEditorState> = {}
    for (const name of frameNames) {
        const state = frames[name]
        if (state) picked[name] = state
    }
    return picked
}

/** 会话内映射键随帧名迁移（重命名缝）：已访问帧的切片不因改名丢失。旧键必有切片
 *  才迁移（未访问帧无事可做）；同名目标键覆盖（帧名非唯一键的既定代价）。 */
export function migrateFrameStateKey(
    frames: Readonly<Record<string, CanvasEditorState>>,
    from: string,
    to: string,
): Record<string, CanvasEditorState> {
    const state = frames[from]
    if (!state) return { ...frames }
    const next = { ...frames }
    delete next[from]
    next[to] = state
    return next
}

/** 删帧出映射（删帧缝）：被删帧键剔除，其余键原样——无同名幸存帧才剔除（帧名非
 *  唯一键下取保守清理，同名帧共享键的别名代价不变）。 */
export function dropFrameStateKey(
    frames: Readonly<Record<string, CanvasEditorState>>,
    name: string,
    survivingNames: readonly string[],
): Record<string, CanvasEditorState> {
    if (survivingNames.includes(name)) return { ...frames }
    const next = { ...frames }
    delete next[name]
    return next
}
