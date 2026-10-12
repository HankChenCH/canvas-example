// 展示格式化纯逻辑（评审修复 F4 抽取缝）：RFC3339(UTC) 时间戳本地化单一实现。
// 此前 VersionHistoryDrawer（formatCreatedAt）、TemplateListPage / DataSourceListPage
// （formatUpdatedAt）三处同形函数体各抄一份，且列表页用它格式化版本 createdAt
// （发布时间）名实不符——收敛为如实命名的 formatTimestamp，时刻语义由调用点标签
// 自明（「更新于」「发布于」）。与 Vue/DOM 解耦（editor/*.ts 惯例，TDD 缝）。

/** ISO 时间戳 → zh-CN 本地时间文本（24 小时制）；解析失败原样回显（宽容面，
 *  不伪造非法时间的显示） */
export function formatTimestamp(iso: string): string {
    const t = new Date(iso)
    return Number.isNaN(t.getTime()) ? iso : t.toLocaleString('zh-CN', { hour12: false })
}
