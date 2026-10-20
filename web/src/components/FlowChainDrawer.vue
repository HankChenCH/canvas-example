<script setup lang="ts">
/**
 * FlowChainDrawer：流链编辑器（29 票引入；页模型适配工票 03——「链草稿编辑」改
 * 「页 flow 编辑」）。
 *
 * flow 附着于页对象（PageSlot.flow，无平行数组草稿）：每页一行 = 参与模式三选
 * （off = 无 flow 键 = 链外页 / fixed / paged）+ fixed 的 quota 输入 + 零行跳页
 * 勾选；行序 = 页序、行标 = 页名（缺省「第 N 页」）。本地校验错误（宿主
 * validatePageFlows 产物）行内集中回显并禁用保存，服务端编译码（flow_chain_invalid
 * 等）单独回显。
 *
 * 状态边界：页序列（flow 真值源）、校验/服务端错误、独立未保存标记、在途态
 * 全部由宿主持有——本组件纯呈现，变更经 setRole/setQuota/setOmitIfEmpty
 * 三事件上抛（单向数据流，DataSourceDrawer 先例）。Teleport body + 自带令牌块；
 * 非模态，画布保持可交互。
 */
import type { PageFlowRole, PageFlowRow, PageTableInfo } from '../editor/pageflow'

defineProps<{
    /** 抽屉开合（页 tab 条「流链」钮驱动） */
    open: boolean
    /** 页行摘要（页序对齐）：页 id（行 key）+ 页名 + 顶层表信息 + flow 现值 */
    rows: ReadonlyArray<PageFlowRow>
    /** 本地校验错误（非空时保存禁用） */
    errors: readonly string[]
    /** 服务端错误回显（保存失败稳定 code 在前的可读文案） */
    error: string | null
    /** 段内独立未保存标记（各页 flow canonical 串 vs 载入基线） */
    dirty: boolean
    /** 保存（文档级 PUT）在途态 */
    saving: boolean
}>()

const emit = defineEmits<{
    close: []
    /** 保存流链：宿主走文档级 PUT（各页一并整存），错误段内回显 */
    save: []
    setRole: [index: number, role: PageFlowRole]
    setQuota: [index: number, quota: number | null]
    setOmitIfEmpty: [index: number, value: boolean]
}>()

/** 参与模式三选（segmented 单选）：off = 无 flow 键 = 链外页 */
const ROLES: ReadonlyArray<{ value: PageFlowRole; label: string }> = [
    { value: 'off', label: '不在链' },
    { value: 'fixed', label: 'fixed' },
    { value: 'paged', label: 'paged' },
]

/** 页 rowsPath 提示：有模板态表显路径，无表/非模板态给行动指引，解析失败不显示 */
function tableHint(rows: ReadonlyArray<PageFlowRow>, index: number): string {
    const table: PageTableInfo | null | undefined = rows[index]?.table
    if (!table) return ''
    if (table.tableCount === 0) return '无顶层表格图层'
    if (!table.hasTemplate) return '顶层表非模板态'
    return table.rowsPath === null ? '' : `rowsPath: ${table.rowsPath}`
}

/** quota 输入桥：空串 = null（不携带 = 容量型）；负数/小数就地钳制为非负整数 */
function onQuotaInput(emitFn: (q: number | null) => void, event: Event): void {
    const raw = (event.target as HTMLInputElement).value.trim()
    if (raw === '') {
        emitFn(null)
        return
    }
    const n = Number(raw)
    emitFn(Number.isFinite(n) ? Math.max(0, Math.trunc(n)) : null)
}
</script>

<template>
    <Teleport to="body">
        <aside v-if="open" class="cn-fcd" data-flowchain-drawer aria-label="流链编辑器">
            <header class="cn-fcd__header">
                <h2 class="cn-fcd__title">流链（行流分配）</h2>
                <button type="button" class="cn-fcd__close" data-drawer-close aria-label="关闭流链编辑器" @click="emit('close')">
                    ✕
                </button>
            </header>

            <section class="cn-fcd__section" data-flowchain-section aria-label="流链页列表">
                <p class="cn-fcd__note">
                    流链声明各页如何消费数据行的行流：不在链 = 链外页（不吃行、照常进产物）；
                    fixed = 单页固定份额（装满可用区）；paged = 链尾吃尽剩余、按页高自动切多页。
                    保存走文档级通道，各页内容一并整存。
                </p>

                <!-- 每页一行：参与模式三选 + fixed 配额 + 零行跳页（flow 附着于页对象，
                     行 key = 页 id，删页/重排后行身份稳定） -->
                <div
                    v-for="(row, i) in rows"
                    :key="row.id"
                    class="cn-fcd__page"
                    :data-flowchain-page="i"
                >
                    <p class="cn-fcd__page-label">
                        <span>{{ row.label }}</span>
                        <span v-if="tableHint(rows, i)" class="cn-fcd__rows-path">{{ tableHint(rows, i) }}</span>
                    </p>
                    <div class="cn-fcd__roles" role="group" :aria-label="`${row.label} 参与模式`">
                        <button
                            v-for="r in ROLES"
                            :key="r.value"
                            type="button"
                            class="cn-fcd__role"
                            :class="{ 'is-active': (row.flow === null ? 'off' : row.flow.mode) === r.value }"
                            :data-chain-role="r.value"
                            :aria-pressed="(row.flow === null ? 'off' : row.flow.mode) === r.value"
                            @click="emit('setRole', i, r.value)"
                        >
                            {{ r.label }}
                        </button>
                    </div>
                    <template v-if="row.flow !== null && row.flow.mode === 'fixed'">
                        <label class="cn-fcd__label" :for="`fcd-quota-${i}`">配额 quota（空 = 不携带，按可用区容量装）</label>
                        <input
                            :id="`fcd-quota-${i}`"
                            type="number"
                            min="0"
                            step="1"
                            class="cn-fcd__input"
                            :value="row.flow.quota ?? ''"
                            :data-flowchain-quota="i"
                            @input="onQuotaInput((q) => emit('setQuota', i, q), $event)"
                        />
                    </template>
                    <label v-if="row.flow !== null" class="cn-fcd__omit">
                        <input
                            type="checkbox"
                            :checked="row.flow.omitIfEmpty"
                            :data-flowchain-omit="i"
                            @change="emit('setOmitIfEmpty', i, ($event.target as HTMLInputElement).checked)"
                        />
                        零行时不产该页（omitIfEmpty）
                    </label>
                </div>

                <!-- 本地校验错误：行内集中回显 + 保存禁用（权威在服务端保存预检） -->
                <div v-if="errors.length > 0" class="cn-fcd__errors" data-flowchain-errors>
                    <p v-for="(e, i) in errors" :key="i">{{ e }}</p>
                </div>
                <p v-if="error" class="cn-fcd__error" data-flowchain-error>{{ error }}</p>

                <footer class="cn-fcd__footer">
                    <span v-if="dirty" class="cn-fcd__unsaved" data-flowchain-unsaved>● 未保存</span>
                    <button
                        type="button"
                        class="cn-fcd__save"
                        data-save-flowchain
                        title="保存流链（文档级 PUT：name + 各页一并整存）"
                        :disabled="saving || errors.length > 0"
                        @click="emit('save')"
                    >
                        {{ saving ? '保存中…' : '保存流链' }}
                    </button>
                </footer>
            </section>
        </aside>
    </Teleport>
</template>

<style scoped>
/* 令牌与 panel-theme.css 同值：Teleport 后脱离宿主 DOM 子树，主题自带（抽屉先例） */
.cn-fcd {
    --cn-bg: #0b1220;
    --cn-bg-elevated: #101a2e;
    --cn-fg: #e6edf7;
    --cn-fg-2: #c3cddd;
    --cn-muted: #7c8ca5;
    --cn-line: #1e2a40;
    --cn-line-strong: #2a3a58;
    --cn-accent: #38bdf8;
    --cn-on-accent: #06202b;

    position: fixed;
    top: 92px; /* 顶栏 48 + 工具栏 44 之下 */
    right: 12px;
    bottom: 40px; /* 状态栏可见——独立标记与全局指示灯可同屏对照 */
    z-index: 30;
    display: flex;
    flex-direction: column;
    width: 470px;
    max-width: calc(100vw - 24px);
    overflow-y: auto;
    background: var(--cn-bg-elevated);
    border: 1px solid var(--cn-line);
    border-radius: 12px;
    box-shadow: 0 24px 64px rgba(2, 6, 23, 0.6);
}

.cn-fcd__header {
    display: flex;
    flex: none;
    align-items: center;
    justify-content: space-between;
    padding: 12px 16px;
    border-bottom: 1px solid var(--cn-line);
}

.cn-fcd__title {
    margin: 0;
    color: var(--cn-fg);
    font-size: 14px;
    font-weight: 600;
    line-height: 1.4;
}

.cn-fcd__close {
    padding: 2px 8px;
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: var(--cn-muted);
    font-size: 13px;
    line-height: 1.4;
    cursor: pointer;
}

.cn-fcd__close:hover {
    background: rgba(148, 163, 184, 0.07);
    color: var(--cn-fg);
}

.cn-fcd__section {
    display: flex;
    flex-direction: column;
    padding: 12px 16px 14px;
}

.cn-fcd__note {
    margin: 0 0 4px;
    color: var(--cn-muted);
    font-size: 11px;
    line-height: 1.6;
}

/* 页行：页名 + rowsPath 提示，行间 border 分隔 */
.cn-fcd__page {
    display: flex;
    flex-direction: column;
    padding: 10px 0 12px;
}

.cn-fcd__page + .cn-fcd__page {
    border-top: 1px solid var(--cn-line);
}

.cn-fcd__page-label {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 8px;
    margin: 0 0 8px;
    color: var(--cn-fg);
    font-size: 13px;
    font-weight: 600;
    line-height: 1.4;
}

.cn-fcd__rows-path {
    color: var(--cn-muted);
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-size: 10px;
    font-weight: 400;
}

/* 参与模式 segmented：三选单选，选中 accent 底 */
.cn-fcd__roles {
    display: inline-flex;
    width: fit-content;
    overflow: hidden;
    border: 1px solid var(--cn-line-strong);
    border-radius: 8px;
}

.cn-fcd__role {
    padding: 4px 12px;
    border: 0;
    background: transparent;
    color: var(--cn-fg-2);
    font-size: 12px;
    line-height: 1.4;
    cursor: pointer;
}

.cn-fcd__role + .cn-fcd__role {
    border-left: 1px solid var(--cn-line);
}

.cn-fcd__role:hover:not(.is-active) {
    background: rgba(148, 163, 184, 0.07);
    color: var(--cn-fg);
}

.cn-fcd__role.is-active {
    background: var(--cn-accent);
    color: var(--cn-on-accent);
    font-weight: 600;
}

.cn-fcd__label {
    margin-top: 10px;
    margin-bottom: 4px;
    color: var(--cn-accent);
    font-size: 11px;
    font-weight: 600;
    line-height: 1.4;
}

.cn-fcd__input {
    box-sizing: border-box;
    width: 160px;
    padding: 6px 10px;
    border: 1px solid var(--cn-line);
    border-radius: 8px;
    background: var(--cn-bg);
    color: var(--cn-fg);
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-size: 12px;
    line-height: 1.5;
}

.cn-fcd__input:focus {
    border-color: var(--cn-line-strong);
    outline: none;
}

.cn-fcd__omit {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-top: 10px;
    color: var(--cn-fg-2);
    font-size: 12px;
    line-height: 1.4;
    cursor: pointer;
}

.cn-fcd__omit input {
    accent-color: var(--cn-accent);
}

/* 本地校验错误集中区与服务端错误位 */
.cn-fcd__errors {
    margin-top: 10px;
    padding: 8px 10px;
    border: 1px solid rgba(252, 165, 165, 0.35);
    border-radius: 8px;
    background: rgba(252, 165, 165, 0.06);
}

.cn-fcd__errors p,
.cn-fcd__error {
    margin: 0;
    color: #fca5a5;
    font-size: 12px;
    line-height: 1.5;
    overflow-wrap: anywhere;
}

.cn-fcd__errors p + p {
    margin-top: 4px;
}

.cn-fcd__error {
    margin-top: 8px;
}

.cn-fcd__footer {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 12px;
}

/* 段内独立未保存小标记：琥珀色 ● 与全局指示灯同词汇、独立计算 */
.cn-fcd__unsaved {
    margin-right: auto;
    color: #f59e0b;
    font-size: 12px;
    white-space: nowrap;
}

.cn-fcd__save {
    padding: 5px 12px;
    border: none;
    border-radius: 8px;
    background: var(--cn-accent);
    color: var(--cn-on-accent);
    font-size: 13px;
    font-weight: 600;
    cursor: pointer;
}

.cn-fcd__save:hover:not(:disabled) {
    filter: brightness(1.12);
}

.cn-fcd__save:disabled {
    opacity: 0.55;
    cursor: not-allowed;
}
</style>
