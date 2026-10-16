<script setup lang="ts">
/**
 * NamePromptDialog：通用名字输入模态弹窗（22 票由另存为弹窗 SaveAsDialog 泛化）。
 *
 * 两个消费面：EditorPage「另存为」（默认文案即另存为语义）与 TemplateListPage
 * 「新建空白模板」（标题/注记/确认钮文案经 props 定制）。模态名字输入：确认后
 * 业务编排归宿主（另存为两连调用 / 新建单调用），成功后宿主负责导航。Enter 确认 /
 * Esc 取消（输入法合成中的 Enter 只上屏不确认）；点遮罩取消。在途态下确认与取消
 * 均禁用——半途中断会让「已建副本/模板」与界面状态脱钩。
 *
 * 数据源选择段（新建模板选数据源，可选能力）：仅当宿主传 dataSourceOptions 时
 * 渲染（另存为消费面不传 = 整段不渲染，零感知）——首项「暂不绑定」，其余项带
 * 「N 个模板引用」影响面注记（DataSourceDrawer 绑定段同词汇）；空列表给「去创建」
 * 链接（/datasources/new）。选择经 defineModel 回宿主，随建随绑。
 *
 * 状态边界：名字草稿（defineModel）、数据源选择（defineModel）、错误文案、在途态
 * 全部由宿主持有——本组件纯呈现，不持业务状态（DataSourceDrawer 先例）。
 * Teleport body + 自带令牌块。
 */
import { nextTick, ref, watch } from 'vue'

import type { DataSourceSummary } from '../api'

const name = defineModel<string>('name', { required: true })
const dataSourceId = defineModel<number | null>('dataSourceId')

const props = defineProps<{
    /** 弹窗开合（宿主动作钮驱动） */
    open: boolean
    /** 编排在途态（期间确认/取消禁用） */
    saving: boolean
    /** 错误回显：稳定码在前的可读文案（spec §2.1 按 code 判定） */
    error: string | null
    /** 弹窗标题（默认另存为语义，保持 18 票文案逐字不变） */
    title?: string
    /** 输入框标签 */
    label?: string
    /** 语义注记（说明确认后会发生什么） */
    note?: string
    /** 确认钮文案（在途态由 confirmBusyText 接管） */
    confirmText?: string
    confirmBusyText?: string
    /** 数据源候选（undefined = 不渲染数据源段，另存为消费面） */
    dataSourceOptions?: DataSourceSummary[]
}>()

const emit = defineEmits<{
    confirm: []
    cancel: []
}>()

const inputEl = ref<HTMLInputElement | null>(null)

/** 开启即聚焦全选：预填宿主给定的名字草稿，直接键入即整体替换 */
watch(
    () => props.open,
    async (open) => {
        if (!open) return
        await nextTick()
        inputEl.value?.focus()
        inputEl.value?.select()
    },
)

/** 输入法合成中的 Enter 只上屏不确认（中文名输入常态） */
function isImeComposing(event: KeyboardEvent): boolean {
    return event.isComposing || event.keyCode === 229
}

function onKeydown(event: KeyboardEvent): void {
    if (!props.open || props.saving) return
    if (event.key === 'Escape') {
        event.preventDefault()
        emit('cancel')
        return
    }
    if (event.key === 'Enter' && !isImeComposing(event)) {
        event.preventDefault()
        emit('confirm')
    }
}

/** 下拉 → 模型：'' = 暂不绑定（null），其余转数字 id */
function onDataSourceChange(event: Event): void {
    const value = (event.target as HTMLSelectElement).value
    dataSourceId.value = value === '' ? null : Number(value)
}
</script>

<template>
    <Teleport to="body">
        <div
            v-if="open"
            class="cn-npd"
            data-name-prompt-backdrop
            @keydown="onKeydown"
            @click.self="emit('cancel')"
        >
            <section
                class="cn-npd__panel"
                data-name-prompt-dialog
                role="dialog"
                aria-modal="true"
                :aria-label="title"
            >
                <h2 class="cn-npd__title">{{ title }}</h2>
                <label class="cn-npd__label" for="npd-name">{{ label }}</label>
                <input
                    id="npd-name"
                    ref="inputEl"
                    v-model="name"
                    class="cn-npd__input"
                    data-name-prompt-input
                    type="text"
                    :aria-label="label"
                    :disabled="saving"
                    spellcheck="false"
                />
                <!-- 数据源选择段（可选能力）：仅 dataSourceOptions 传入时渲染 -->
                <template v-if="dataSourceOptions !== undefined">
                    <label class="cn-npd__label cn-npd__label--ds" for="npd-datasource">数据源（可选）</label>
                    <select
                        id="npd-datasource"
                        class="cn-npd__input cn-npd__select"
                        data-name-prompt-datasource
                        :value="dataSourceId ?? ''"
                        :disabled="saving"
                        @change="onDataSourceChange"
                    >
                        <option value="">暂不绑定（可在编辑器顶栏「数据源」中绑定）</option>
                        <option v-for="s in dataSourceOptions" :key="s.id" :value="String(s.id)">
                            {{ s.name }}（{{ s.templateCount }} 个模板引用）
                        </option>
                    </select>
                    <p v-if="dataSourceOptions.length === 0" class="cn-npd__hint" data-name-prompt-ds-empty>
                        还没有数据源——可先「暂不绑定」创建，之后到
                        <RouterLink class="cn-npd__link" to="/datasources/new">数据源页</RouterLink>
                        创建并在编辑器内绑定。
                    </p>
                    <p v-else class="cn-npd__hint" data-name-prompt-ds-hint>
                        绑定后，文字/图片/二维码即可用 <code v-pre>{{字段}}</code> 表达式引用数据内容。
                    </p>
                </template>
                <p class="cn-npd__note">{{ note }}</p>
                <p v-if="error" class="cn-npd__error" data-name-prompt-error>{{ error }}</p>
                <footer class="cn-npd__footer">
                    <button
                        type="button"
                        class="cn-npd__cancel"
                        data-name-prompt-cancel
                        :disabled="saving"
                        @click="emit('cancel')"
                    >
                        取消
                    </button>
                    <button
                        type="button"
                        class="cn-npd__confirm"
                        data-name-prompt-confirm
                        :disabled="saving"
                        @click="emit('confirm')"
                    >
                        {{ saving ? confirmBusyText : confirmText }}
                    </button>
                </footer>
            </section>
        </div>
    </Teleport>
</template>

<style scoped>
/* 令牌与 panel-theme.css 同值：Teleport 后脱离宿主 DOM 子树，主题自带（DataSourceDrawer 先例）。
   模态：全屏遮罩（blur）截断交互，面板居中；金→青 hairline 顶线是印坊弹窗的识别面。 */
.cn-npd {
    position: fixed;
    inset: 0;
    z-index: 40;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 16px;
    background: rgba(2, 6, 23, 0.6);
    -webkit-backdrop-filter: blur(8px);
    backdrop-filter: blur(8px);
    animation: cn-npd-fade 0.22s ease-out backwards;
}

@keyframes cn-npd-fade {
    from {
        opacity: 0;
    }
}

@keyframes cn-npd-pop {
    from {
        opacity: 0;
        transform: translateY(10px) scale(0.985);
    }
}

.cn-npd__panel {
    --cn-bg: #0b1220;
    --cn-bg-elevated: #101a2e;
    --cn-fg: #e6edf7;
    --cn-fg-2: #c3cddd;
    --cn-muted: #7c8ca5;
    --cn-line: #1e2a40;
    --cn-line-strong: #2a3a58;
    --cn-accent: #38bdf8;
    --cn-on-accent: #06202b;
    --cn-font-display: 'Noto Serif SC', 'Songti SC', 'STSong', 'SimSun', serif;

    position: relative;
    width: 420px;
    max-width: 100%;
    padding: 20px;
    overflow: hidden;
    background: linear-gradient(180deg, #101a2e 0%, #0c1526 100%);
    border: 1px solid var(--cn-line);
    border-radius: 14px;
    box-shadow:
        inset 0 1px 0 rgba(148, 197, 255, 0.06),
        0 24px 64px rgba(2, 6, 23, 0.65);
    animation: cn-npd-pop 0.3s cubic-bezier(0.2, 0.7, 0.3, 1) backwards;
}

/* 金→青 hairline 顶线：与列表页页头铭线同一词汇 */
.cn-npd__panel::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 2px;
    background: linear-gradient(90deg, rgba(227, 195, 127, 0.85), rgba(56, 189, 248, 0.55) 45%, transparent);
}

.cn-npd__title {
    margin: 0 0 12px;
    color: var(--cn-fg);
    font-family: var(--cn-font-display);
    font-size: 16px;
    font-weight: 600;
    letter-spacing: 0.03em;
    line-height: 1.4;
}

.cn-npd__label {
    display: block;
    margin-bottom: 5px;
    color: var(--cn-accent);
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.04em;
    line-height: 1.4;
}

.cn-npd__input {
    box-sizing: border-box;
    width: 100%;
    padding: 8px 11px;
    border: 1px solid var(--cn-line);
    border-radius: 9px;
    background: var(--cn-bg);
    color: var(--cn-fg);
    font-size: 13px;
    transition:
        border-color 0.2s,
        box-shadow 0.2s;
}

.cn-npd__input:focus {
    border-color: rgba(56, 189, 248, 0.65);
    box-shadow: 0 0 0 3px rgba(56, 189, 248, 0.14);
    outline: none;
}

/* 数据源选择段：标签与名字输入拉开间距；select 沿用输入框底形（原生弹层），
   hint 小字解释「绑定 → 表达式可用」的因果关系 */
.cn-npd__label--ds {
    margin-top: 14px;
}

.cn-npd__select {
    appearance: none;
    background-image: linear-gradient(45deg, transparent 50%, var(--cn-muted) 50%),
        linear-gradient(135deg, var(--cn-muted) 50%, transparent 50%);
    background-position:
        calc(100% - 18px) calc(50% - 2px),
        calc(100% - 13px) calc(50% - 2px);
    background-size: 5px 5px;
    background-repeat: no-repeat;
    cursor: pointer;
}

.cn-npd__select:focus {
    cursor: default;
}

.cn-npd__hint {
    margin: 8px 0 0;
    color: var(--cn-muted);
    font-size: 11px;
    line-height: 1.6;
}

.cn-npd__hint code {
    padding: 0 3px;
    border: 1px solid var(--cn-line);
    border-radius: 4px;
    background: var(--cn-bg);
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-size: 10px;
}

.cn-npd__link {
    color: var(--cn-accent);
    text-decoration: none;
}

.cn-npd__link:hover {
    text-decoration: underline;
}

.cn-npd__note {
    margin: 10px 0 0;
    color: var(--cn-muted);
    font-size: 11px;
    line-height: 1.6;
}

.cn-npd__error {
    margin: 10px 0 0;
    color: #fca5a5;
    font-size: 12px;
    line-height: 1.5;
    overflow-wrap: anywhere;
}

.cn-npd__footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 16px;
    padding-top: 14px;
    border-top: 1px solid var(--cn-line);
}

.cn-npd__cancel {
    padding: 6px 13px;
    border: 1px solid var(--cn-line);
    border-radius: 9px;
    background: transparent;
    color: var(--cn-fg-2);
    font-size: 13px;
    cursor: pointer;
    transition:
        border-color 0.2s,
        color 0.2s;
}

.cn-npd__cancel:hover:not(:disabled) {
    border-color: var(--cn-line-strong);
    color: var(--cn-fg);
}

.cn-npd__cancel:focus-visible {
    border-color: var(--cn-accent);
    outline: none;
}

.cn-npd__confirm {
    padding: 6px 14px;
    border: none;
    border-radius: 9px;
    background: linear-gradient(180deg, color-mix(in srgb, var(--cn-accent) 86%, #fff), var(--cn-accent));
    color: var(--cn-on-accent);
    font-size: 13px;
    font-weight: 600;
    cursor: pointer;
    box-shadow: 0 4px 14px rgba(56, 189, 248, 0.22);
    transition:
        filter 0.2s,
        transform 0.15s;
}

.cn-npd__confirm:hover:not(:disabled) {
    filter: brightness(1.1);
}

.cn-npd__confirm:active:not(:disabled) {
    transform: translateY(1px);
}

.cn-npd__confirm:focus-visible {
    outline: 2px solid var(--cn-accent);
    outline-offset: 2px;
}

.cn-npd__confirm:disabled,
.cn-npd__cancel:disabled,
.cn-npd__input:disabled {
    opacity: 0.55;
    cursor: not-allowed;
}

@media (prefers-reduced-motion: reduce) {
    .cn-npd,
    .cn-npd__panel {
        animation: none;
    }
}
</style>
