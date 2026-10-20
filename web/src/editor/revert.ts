// 回滚会话转移纯逻辑（template-draft-publish 批 B ③ TDD 缝，spec §5 / 票 04；页
// 模型适配工票 03）：revert 响应的全量模板记录 → 新会话态成算（页槽序列/dirty 基线）。
// 与 Vue/DOM 解耦，EditorPage 拿成算做原子会话替换（另存为「会话延续式重绑」同
// 口径，不整页重载）。
//
// 语义要点：
// - dirty 基线重置为新内容——回滚改写草稿 = 一次「载入」级会话替换，回滚后立即归
//   clean（基线 = 回滚记录字节，与 loadTemplate/另存为的基线口径同缝）；继续编辑
//   才再 dirty，满意后发布成新版本（版本线线性：回滚永不造版）；
// - flow 随页槽载入（PageSlot.flow，宽容归一）：无平行数组第二份草稿，结构化归一
//   不产生伪 dirty 的旧顾虑随页模型自然消失；
// - 工程数据零特殊处理（spec §5）：pageStates 不进本模块 = 不动的结构性声明——页
//   id 随发布快照整体定着，快照页 id 与现会话页 id 按稳定 id 对叁（id 同 = 编辑状态
//   跟对页，spec US9 语义）；id 对不上的悬空键由宽容读丢弃（pickPageStates 精神），
//   flush 载荷按现役页 id 组装自愈；
// - 任一页解码失败抛错（loadPageSlots 同缝，消息带页下标）：宿主须在替换会话前
//   先成算，失败时原会话零变更、错误留在确认框内。
import type { TemplateRecord } from '../api'

import { baselineFromPages, loadPageSlots, type PageBaseline, type PageSlot } from './pages'

/** 回滚会话成算：调用方逐字段原子替换会话态（template/templateName/pageSlots/
 *  activePage/savedBaseline），替换动作本身留在宿主（响应式接线不进纯逻辑） */
export interface RevertSessionPlan {
    /** 新页槽序列（逐页 decode 验形 + canonical 化，flow 随页宽容归一） */
    slots: PageSlot[]
    /** 新 dirty 基线 = 回滚记录字节（名 + 各页 id/name/graph/flow canonical 串） */
    baseline: PageBaseline
}

/** 成算回滚会话（纯计算）：快照粒度 = 文档整体（pages 序列，页 id 在内；data_source_id
 *  是绑定不进快照——绑定按全量记录由宿主如实重注，与本模块无关） */
export function planRevertSession(record: TemplateRecord): RevertSessionPlan {
    const slots = loadPageSlots(record.pages)
    return { slots, baseline: baselineFromPages(record.name, slots) }
}
