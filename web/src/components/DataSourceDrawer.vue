<script setup lang="ts">
/**
 * DataSourceDrawer：数据源抽屉（23 票重构：数据源为独立实体、模板持引用，
 * spec §4.3 两段式——29 票流链段迁出为独立的流链编辑器 FlowChainDrawer）。
 *
 * 一个抽屉、两段，段间距 + 各自按钮区分：
 * - 绑定段：当前绑定展示 + 数据源列表下拉 + 绑定/解绑动作钮 → 宿主走
 *   PUT /templates/{id}/datasource（引用列整存替换）；绑定列表来自
 *   GET /datasources 摘要（含 templateCount 共享影响面）。
 * - 数据源内容段：编辑的是「绑定的数据源实体」——name/schema/data 三个输入
 *   + 「保存数据源」→ PUT /datasources/{boundId}（影响所有引用它的模板，段内
 *   注记）；未绑时同三输入 + 「创建并绑定」→ POST /datasources + 绑定。已绑时
 *   另有「另存为新数据源」（宿主弹名字框 → POST + 重绑，copy-on-write）。
 *   schema_invalid / dataset_schema_mismatch 等由宿主回显在段内（contentError）。
 *
 * 状态边界：下拉选中项、文本域草稿（defineModel 三连）、段内独立未保存标记、
 * 错误文案、保存在途态全部由宿主持有——本组件纯呈现，不持业务状态。
 * Teleport body + 自带令牌块（脱离宿主 DOM 子树，HelpDialog/ContextMenu 先例）；
 * 非模态，画布保持可交互。
 */
import { computed } from 'vue'

import type { DataSourceSummary } from '../api'

const selectedBindId = defineModel<number | null>('selectedBindId', { required: true })
const nameText = defineModel<string>('nameText', { required: true })
const schemaText = defineModel<string>('schemaText', { required: true })
const dataText = defineModel<string>('dataText', { required: true })

const props = defineProps<{
    /** 抽屉开合（宿主顶栏「数据源」钮驱动） */
    open: boolean
    /** 绑定段：当前绑定（null = 未绑）与数据源列表摘要 */
    boundId: number | null
    boundName: string | null
    sources: DataSourceSummary[]
    /** 绑定动作在途与段内错误（PUT /templates/{id}/datasource） */
    bindSaving: boolean
    bindError: string | null
    /** 内容段：独立未保存标记、在途态、错误回显（稳定 code 在前的可读文案） */
    contentDirty: boolean
    contentSaving: boolean
    contentError: string | null
}>()

const emit = defineEmits<{
    close: []
    /** 绑定/解绑动作：按选中项与当前绑定的差值由宿主发绑定通道 */
    bind: []
    /** 内容段保存：已绑 = PUT 实体；未绑 = 创建并绑定 */
    saveContent: []
    /** 另存为新数据源（仅已绑可用；宿主弹名字框编排 POST + 重绑） */
    saveAsNew: []
}>()

/** 下拉值 ↔ 可空 id 桥：'' = 未绑定（null） */
const selectValue = computed({
    get: () => (selectedBindId.value === null ? '' : String(selectedBindId.value)),
    set: (v: string) => {
        selectedBindId.value = v === '' ? null : Number(v)
    },
})

/** 绑定动作钮：选中即当前绑定 → 无变更禁用；选中 null 且当前未绑同此 */
const bindActionDisabled = computed(() => selectedBindId.value === props.boundId)
const bindActionLabel = computed(() => (selectedBindId.value === null ? '解绑' : '绑定'))
</script>

<template>
    <Teleport to="body">
        <aside v-if="open" class="cn-dsw" data-datasource-drawer aria-label="数据源抽屉">
            <header class="cn-dsw__header">
                <h2 class="cn-dsw__title">数据源</h2>
                <button type="button" class="cn-dsw__close" data-drawer-close aria-label="关闭数据源抽屉" @click="emit('close')">
                    ✕
                </button>
            </header>

            <!-- 段一：绑定（模板持引用——绑定/解绑走 PUT /templates/{id}/datasource） -->
            <section class="cn-dsw__section" data-bind-section aria-label="数据源绑定">
                <p class="cn-dsw__note">
                    数据源是独立资源，模板只保存引用：
                    <strong data-bound-name>{{ boundId === null ? '未绑定' : `「${boundName}」` }}</strong>
                    <!-- 25 票：数据源独立管理页入口（离开编辑器时脏文档 confirm 守卫照旧） -->
                    <RouterLink to="/datasources" class="cn-dsw__link" data-datasource-manage-link>
                        独立管理页 →
                    </RouterLink>
                </p>
                <label class="cn-dsw__label" for="dsw-bind">数据源列表（括注为引用它的模板数）</label>
                <select id="dsw-bind" v-model="selectValue" class="cn-dsw__select" data-bind-select aria-label="选择数据源">
                    <option value="">（未绑定）</option>
                    <option v-for="s in sources" :key="s.id" :value="String(s.id)">
                        {{ s.name }}（{{ s.templateCount }} 个模板引用）
                    </option>
                </select>
                <p v-if="bindError" class="cn-dsw__error" data-bind-error>{{ bindError }}</p>
                <footer class="cn-dsw__footer">
                    <button
                        type="button"
                        class="cn-dsw__save"
                        data-bind-action
                        :disabled="bindActionDisabled || bindSaving"
                        @click="emit('bind')"
                    >
                        {{ bindSaving ? '处理中…' : bindActionLabel }}
                    </button>
                </footer>
            </section>

            <!-- 段二：数据源内容（编辑绑定的数据源实体；未绑 = 创建并绑定起笔态） -->
            <section class="cn-dsw__section" data-dataset-section aria-label="数据源内容">
                <label class="cn-dsw__label" for="dsw-name">数据源名</label>
                <input
                    id="dsw-name"
                    v-model="nameText"
                    class="cn-dsw__input"
                    data-source-name-input
                    spellcheck="false"
                />
                <label class="cn-dsw__label" for="dsw-schema">schema（draft-07 JSON）</label>
                <textarea
                    id="dsw-schema"
                    v-model="schemaText"
                    class="cn-dsw__input"
                    data-schema-input
                    rows="7"
                    spellcheck="false"
                ></textarea>
                <label class="cn-dsw__label" for="dsw-data">data（JSON）</label>
                <textarea
                    id="dsw-data"
                    v-model="dataText"
                    class="cn-dsw__input"
                    data-dataset-input
                    rows="10"
                    spellcheck="false"
                ></textarea>
                <p class="cn-dsw__note" data-content-note>
                    <template v-if="boundId !== null">
                        保存写入数据源实体本身——所有引用它的模板同享（渲染读取已保存的数据集，保存前渲染用旧数据）。
                    </template>
                    <template v-else>
                        未绑定数据源：「创建并绑定」将以此内容新建数据源实体并绑定到本模板。
                    </template>
                </p>
                <p v-if="contentError" class="cn-dsw__error" data-dataset-error>{{ contentError }}</p>
                <footer class="cn-dsw__footer">
                    <span v-if="contentDirty" class="cn-dsw__unsaved" data-dataset-unsaved>● 未保存</span>
                    <button
                        v-if="boundId !== null"
                        type="button"
                        class="cn-dsw__ghost"
                        data-save-as-new
                        title="以当前草稿内容另存为一个新数据源实体并绑定到本模板（不影响现共享实体）"
                        :disabled="contentSaving"
                        @click="emit('saveAsNew')"
                    >
                        另存为新数据源
                    </button>
                    <button
                        type="button"
                        class="cn-dsw__save"
                        data-save-datasource
                        :disabled="contentSaving"
                        @click="emit('saveContent')"
                    >
                        {{ contentSaving ? '保存中…' : boundId !== null ? '保存数据源' : '创建并绑定' }}
                    </button>
                </footer>
            </section>
        </aside>
    </Teleport>
</template>

<style scoped>
/* 令牌与 panel-theme.css 同值：Teleport 后脱离宿主 DOM 子树，主题自带（HelpDialog 先例） */
.cn-dsw {
    --cn-bg: #0b1220;
    --cn-bg-elevated: #101a2e;
    --cn-fg: #e6edf7;
    --cn-fg-2: #c3cddd;
    --cn-muted: #7c8ca5;
    --cn-line: #1e2a40;
    --cn-line-strong: #2a3a58;
    --cn-accent: #38bdf8;
    --cn-on-accent: #06202b;
    --cn-font-mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;

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

.cn-dsw__header {
    display: flex;
    flex: none;
    align-items: center;
    justify-content: space-between;
    padding: 12px 16px;
    border-bottom: 1px solid var(--cn-line);
}

.cn-dsw__title {
    margin: 0;
    color: var(--cn-fg);
    font-size: 14px;
    font-weight: 600;
    line-height: 1.4;
}

.cn-dsw__close {
    padding: 2px 8px;
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: var(--cn-muted);
    font-size: 13px;
    line-height: 1.4;
    cursor: pointer;
}

.cn-dsw__close:hover {
    background: rgba(148, 163, 184, 0.07);
    color: var(--cn-fg);
}

/* 段间距 + 各自按钮区分：段间 border-top 分隔，钮在各段页脚右对齐 */
.cn-dsw__section {
    display: flex;
    flex-direction: column;
    padding: 12px 16px 14px;
}

.cn-dsw__section + .cn-dsw__section {
    border-top: 1px solid var(--cn-line);
}

.cn-dsw__label {
    margin-bottom: 4px;
    color: var(--cn-accent);
    font-size: 11px;
    font-weight: 600;
    line-height: 1.4;
}

.cn-dsw__label:not(:first-child) {
    margin-top: 10px;
}

.cn-dsw__input {
    box-sizing: border-box;
    width: 100%;
    padding: 8px 10px;
    border: 1px solid var(--cn-line);
    border-radius: 8px;
    background: var(--cn-bg);
    color: var(--cn-fg);
    font-family: var(--cn-font-mono);
    font-size: 12px;
    line-height: 1.5;
    resize: vertical;
}

.cn-dsw__input:focus {
    border-color: var(--cn-line-strong);
    outline: none;
}

.cn-dsw__select {
    box-sizing: border-box;
    width: 100%;
    padding: 7px 10px;
    border: 1px solid var(--cn-line);
    border-radius: 8px;
    background: var(--cn-bg);
    color: var(--cn-fg);
    font-size: 12px;
    line-height: 1.5;
}

.cn-dsw__select:focus {
    border-color: var(--cn-line-strong);
    outline: none;
}

.cn-dsw__note {
    margin: 8px 0 0;
    color: var(--cn-muted);
    font-size: 11px;
    line-height: 1.6;
}

.cn-dsw__note strong {
    color: var(--cn-fg-2);
    font-weight: 600;
}

/* 25 票：绑定段内独立管理页链接（accent 色，跟随令牌主题） */
.cn-dsw__link {
    margin-left: 4px;
    color: var(--cn-accent);
    white-space: nowrap;
}

.cn-dsw__link:hover {
    filter: brightness(1.15);
}

.cn-dsw__note code {
    padding: 0 3px;
    border: 1px solid var(--cn-line);
    border-radius: 4px;
    background: var(--cn-bg);
    font-family: var(--cn-font-mono);
    font-size: 10px;
}

.cn-dsw__error {
    margin: 8px 0 0;
    color: #fca5a5;
    font-size: 12px;
    line-height: 1.5;
    overflow-wrap: anywhere;
}

.cn-dsw__footer {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 10px;
}

/* 段内独立未保存小标记：琥珀色 ● 与全局指示灯同词汇、独立计算 */
.cn-dsw__unsaved {
    margin-right: auto;
    color: #f59e0b;
    font-size: 12px;
    white-space: nowrap;
}

.cn-dsw__ghost {
    padding: 5px 12px;
    border: 1px solid var(--cn-line-strong);
    border-radius: 8px;
    background: transparent;
    color: var(--cn-fg-2);
    font-size: 13px;
    cursor: pointer;
}

.cn-dsw__ghost:hover:not(:disabled) {
    color: var(--cn-fg);
    border-color: var(--cn-accent);
}

.cn-dsw__ghost:disabled {
    opacity: 0.55;
    cursor: not-allowed;
}

.cn-dsw__save {
    padding: 5px 12px;
    border: none;
    border-radius: 8px;
    background: var(--cn-accent);
    color: var(--cn-on-accent);
    font-size: 13px;
    font-weight: 600;
    cursor: pointer;
}

.cn-dsw__save:hover:not(:disabled) {
    filter: brightness(1.12);
}

.cn-dsw__save:disabled {
    opacity: 0.55;
    cursor: not-allowed;
}
</style>
