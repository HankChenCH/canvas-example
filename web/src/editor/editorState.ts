// 工程数据快照宿主聚合纯逻辑（工单 04 + 模板页序列工票 03，spec §6）：快照组装
// （flush 侧）、快照宽容解析（载入侧）、悬空页键剔除。与 Vue/DOM 解耦，EditorPage
// 只做响应式接线。分层纪律：切片从内核导入，宿主只拼装不解释——frames 容器（页 id
// → 切片）归宿主，切片内容（guides/lockedPaths）原样透传，恢复权威在内核
// restoreEditorState。
//
// 逐页键 = 页 id（工票 03）：页 id 稳定不随改名/重排变——帧名键时代的两条补丁逻辑
// （改名迁移 migrateFrameStateKey、重名覆盖保守清理 dropFrameStateKey）随键面切换
// 撤销：改名不动键，删页按 id 无条件剔除（id 页内唯一，无「同名幸存」歧义）。
import type { CanvasEditorState, EditorPrefsState } from '@hankchen/canvas-editor'

import type { TemplateEditorStateSnapshot } from '../api'

/** 快照 schemaVersion（宿主自声明）：v2 = 逐页键由帧名切页 id（模板页序列工票 03）。
 *  内核 EDITOR_STATE_SCHEMA_VERSION = 1 是帧名键时代的内核写侧标记（内核不在本工票
 *  范围，不动）；宿主快照形状（frames 容器键语义）归宿主自管，故版本常量落宿主侧。
 *  读侧宽容读不分支版本（未知版本照常提取，迁移器不存在的既定立场），此常量只为
 *  写侧标记。 */
export const EDITOR_STATE_SNAPSHOT_SCHEMA_VERSION = 2

/** 切片拷贝：数组与 guide 条目逐元素复制——快照自持，不与映射/内核共享可变对象 */
function copySlice(slice: CanvasEditorState): CanvasEditorState {
    return {
        guides: slice.guides.map((guide) => ({ ...guide })),
        lockedPaths: [...slice.lockedPaths],
    }
}

/** 组装完整快照（flush 侧）：页 id 与切片按页序对齐入容器，无切片（null）的页不
 *  产生键（恢复端缺键 = 全默认态）。页 id 页内唯一（服务端校验 + 前端生成保证），
 *  无键覆盖问题。frames/prefs 取拷贝——快照自持，与映射/内核后续变更隔离。 */
export function buildEditorStateSnapshot(params: {
    pageIds: readonly string[]
    slices: ReadonlyArray<CanvasEditorState | null | undefined>
    prefs: EditorPrefsState
}): TemplateEditorStateSnapshot {
    const frames: Record<string, CanvasEditorState> = {}
    params.pageIds.forEach((id, i) => {
        const slice = params.slices[i]
        if (id && slice) frames[id] = copySlice(slice)
    })
    return { schemaVersion: EDITOR_STATE_SNAPSHOT_SCHEMA_VERSION, frames, prefs: { ...params.prefs } }
}

/** 宽容解析结果：frames 逐键轻校验后收集（非法键丢弃），prefs 双布尔齐才收
 *  （null = 不应用，内核自持默认/现值）；raw 本身非对象同落全默认态，不抛错 */
export interface ParsedEditorStateSnapshot {
    frames: Record<string, CanvasEditorState>
    prefs: EditorPrefsState | null
}

/** 快照宽容解析（载入侧，spec「宽容读、无迁移器」）：null/形状不合法 = 全默认态；
 *  不分支 schemaVersion（版本常量只为写侧标记，读侧未知键/未知版本一律忽略）；
 *  frames 内单键形状不合法只丢该键、合法兄弟键保留——与内核 restore 的「整体拒」
 *  同口径：guides/lockedPaths 两数组键齐才算切片，内容合法性由内核恢复端兜底。 */
export function parseTemplateEditorStateSnapshot(raw: unknown): ParsedEditorStateSnapshot {
    if (raw === null || typeof raw !== 'object' || Array.isArray(raw)) {
        return { frames: {}, prefs: null }
    }
    const obj = raw as Record<string, unknown>
    const frames: Record<string, CanvasEditorState> = {}
    const rawFrames = obj.frames
    if (typeof rawFrames === 'object' && rawFrames !== null && !Array.isArray(rawFrames)) {
        for (const [id, value] of Object.entries(rawFrames)) {
            if (isCanvasEditorStateShape(value)) frames[id] = copySlice(value)
        }
    }
    const prefs = isEditorPrefsShape(obj.prefs)
        ? {
              rulersVisible: obj.prefs.rulersVisible,
              anchorExpanded: obj.prefs.anchorExpanded,
              // 新层样式槽宽容透传（editor 0.5.0）：旧快照缺字段/畸形值由内核
              // applyEditorPrefs 逐字段收口（pickNextTextStylePatch 不符保留现值），宿主不校验
              nextTextStyle: obj.prefs.nextTextStyle,
              // 建层比例槽已随 editor 0.7.0 退役（ADR 0021）：旧快照残键按未知键
              // 宽容丢弃（spec §3 读侧语义），不再透传
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

/** 悬空页键剔除（载入侧第二缝，spec「删页/回滚遗留键 → 宽容读丢弃」）：只取现役
 *  页 id 对应的切片——快照里删页/重建遗留的键不进会话内存映射，随下次 flush 自然
 *  消失。 */
export function pickPageStates(
    frames: Readonly<Record<string, CanvasEditorState>>,
    pageIds: readonly string[],
): Record<string, CanvasEditorState> {
    const picked: Record<string, CanvasEditorState> = {}
    for (const id of pageIds) {
        const state = frames[id]
        if (state) picked[id] = state
    }
    return picked
}
