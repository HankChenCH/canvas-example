// 回滚会话转移纯逻辑（template-draft-publish 批 B ③ TDD 缝，spec §5 / 票 04）：
// revert 响应的全量模板记录 → 新会话态成算（帧槽/流链草稿/流链基线/dirty 基线）。
// 与 Vue/DOM 解耦，EditorPage 拿成算做原子会话替换（另存为「会话延续式重绑」同
// 口径，不整页重载）。
//
// 语义要点：
// - dirty 基线重置为新内容——回滚改写草稿 = 一次「载入」级会话替换，回滚后立即归
//   clean（基线 = 回滚记录字节，与 loadTemplate/另存为的基线口径同缝）；继续编辑
//   才再 dirty，满意后发布成新版本（版本线线性：回滚永不造版）；
// - 流链草稿与基线同步重置：草稿 = 结构化归一（draftFromFlowChain），段基线 = 草稿
//   派生链——结构化归一不产生伪 dirty；
// - 工程数据零特殊处理（spec §5）：frameStates 不进本模块 = 不动的结构性声明——
//   悬空帧键由宽容读丢弃（pickFrameStates 精神）、幸存同名帧参考线原样保留
//   （best-effort），flush 载荷按现役帧名组装自愈；
// - 任一帧解码失败抛错（loadFrameSlots 同缝，消息带帧下标）：宿主须在替换会话前
//   先成算，失败时原会话零变更、错误留在确认框内。
import type { TemplateRecord } from '../api'

import { baselineFromSlots, loadFrameSlots, type FrameBaseline, type FrameSlot } from './frames'
import { draftFromFlowChain, flowChainFromDraft, type FlowChainDraft } from './flowchain'

/** 回滚会话成算：调用方逐字段原子替换会话态（template/templateName/frameSlots/
 *  activeFrame/savedBaseline/flowChainDraft/flowChainBaselineChain），替换动作本身
 *  留在宿主（响应式接线不进纯逻辑） */
export interface RevertSessionPlan {
    /** 新帧槽（逐帧 decode 验形 + canonical 化） */
    slots: FrameSlot[]
    /** 新流链草稿（结构化条目，长度与帧数对齐） */
    flowChainDraft: FlowChainDraft
    /** 归一派生链（= 流链编辑器段基线；全 off = null） */
    normalizedChain: unknown
    /** 新 dirty 基线 = 回滚记录字节（名 + 各帧名 + 各帧 canonical 串 + 归一链） */
    baseline: FrameBaseline
}

/** 成算回滚会话（纯计算）：快照粒度 = 文档整体（canvases + flowChain，data_source_id
 *  是绑定不进快照——绑定按全量记录由宿主如实重注，与本模块无关） */
export function planRevertSession(record: TemplateRecord): RevertSessionPlan {
    const slots = loadFrameSlots(record.canvases)
    const flowChainDraft = draftFromFlowChain(record.flowChain, record.canvases.length)
    const normalizedChain = flowChainFromDraft(flowChainDraft)
    return {
        slots,
        flowChainDraft,
        normalizedChain,
        baseline: baselineFromSlots(record.name, slots, normalizedChain),
    }
}
