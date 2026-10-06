<script setup lang="ts">
/**
 * ConfirmDialog：通用确认模态弹窗（卡片操作修订：列表页删除模板引入）。
 *
 * 与 NamePromptDialog 同构（Teleport body + 自带令牌块；名字草稿/错误文案/
 * 在途态宿主持有，本组件纯呈现）。差异面：无输入框；**不做全局 Enter 确认**——
 * 破坏性动作的键盘确认只走聚焦按钮的原生 Space/Enter（开启即聚焦取消钮，
 * 误触先落取消）；Esc 与点遮罩均为取消。在途态下确认与取消均禁用。
 */
import { nextTick, ref, watch } from 'vue'

const props = defineProps<{
    /** 弹窗开合（宿主动作钮驱动） */
    open: boolean
    /** 编排在途态（期间确认/取消禁用） */
    busy: boolean
    /** 错误回显：稳定码在前的可读文案（spec §2.1 按 code 判定） */
    error: string | null
    /** 弹窗标题 */
    title: string
    /** 语义注记（说明确认后会发生什么） */
    note?: string
    /** 确认钮文案（在途态由 confirmBusyText 接管） */
    confirmText?: string
    confirmBusyText?: string
    /** 破坏性确认：确认钮红色系 */
    danger?: boolean
}>()

const emit = defineEmits<{
    confirm: []
    cancel: []
}>()

const cancelEl = ref<HTMLButtonElement | null>(null)

/** 开启即聚焦取消钮：破坏性确认的安全默认，误触 Enter/Space 先落取消 */
watch(
    () => props.open,
    async (open) => {
        if (!open) return
        await nextTick()
        cancelEl.value?.focus()
    },
)

function onKeydown(event: KeyboardEvent): void {
    if (!props.open || props.busy) return
    if (event.key === 'Escape') {
        event.preventDefault()
        emit('cancel')
    }
}
</script>

<template>
    <Teleport to="body">
        <div
            v-if="open"
            class="cn-cfd"
            data-confirm-backdrop
            @keydown="onKeydown"
            @click.self="emit('cancel')"
        >
            <section
                class="cn-cfd__panel"
                :class="{ 'cn-cfd__panel--danger': danger }"
                data-confirm-dialog
                role="dialog"
                aria-modal="true"
                :aria-label="title"
            >
                <h2 class="cn-cfd__title">{{ title }}</h2>
                <p class="cn-cfd__note">{{ note }}</p>
                <p v-if="error" class="cn-cfd__error" data-confirm-error>{{ error }}</p>
                <footer class="cn-cfd__footer">
                    <button
                        ref="cancelEl"
                        type="button"
                        class="cn-cfd__cancel"
                        data-confirm-cancel
                        :disabled="busy"
                        @click="emit('cancel')"
                    >
                        取消
                    </button>
                    <button
                        type="button"
                        class="cn-cfd__confirm"
                        :class="{ 'cn-cfd__confirm--danger': danger }"
                        data-confirm-confirm
                        :disabled="busy"
                        @click="emit('confirm')"
                    >
                        {{ busy ? confirmBusyText : confirmText }}
                    </button>
                </footer>
            </section>
        </div>
    </Teleport>
</template>

<style scoped>
/* 令牌与 panel-theme.css 同值：Teleport 后脱离宿主 DOM 子树，主题自带（NamePromptDialog 先例）。
   模态：全屏遮罩（blur）截断交互，面板居中；金→青 hairline 顶线同 NamePromptDialog。 */
.cn-cfd {
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
    animation: cn-cfd-fade 0.22s ease-out backwards;
}

@keyframes cn-cfd-fade {
    from {
        opacity: 0;
    }
}

@keyframes cn-cfd-pop {
    from {
        opacity: 0;
        transform: translateY(10px) scale(0.985);
    }
}

.cn-cfd__panel {
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
    animation: cn-cfd-pop 0.3s cubic-bezier(0.2, 0.7, 0.3, 1) backwards;
}

/* 金→青 hairline 顶线；破坏性确认时转红系以同构换义 */
.cn-cfd__panel::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 2px;
    background: linear-gradient(90deg, rgba(227, 195, 127, 0.85), rgba(56, 189, 248, 0.55) 45%, transparent);
}

.cn-cfd__panel--danger::before {
    background: linear-gradient(90deg, rgba(239, 68, 68, 0.8), rgba(239, 68, 68, 0.35) 45%, transparent);
}

.cn-cfd__title {
    margin: 0 0 12px;
    color: var(--cn-fg);
    font-family: var(--cn-font-display);
    font-size: 16px;
    font-weight: 600;
    letter-spacing: 0.03em;
    line-height: 1.4;
}

.cn-cfd__note {
    margin: 0;
    color: var(--cn-fg-2);
    font-size: 12px;
    line-height: 1.7;
}

.cn-cfd__error {
    margin: 10px 0 0;
    color: #fca5a5;
    font-size: 12px;
    line-height: 1.5;
    overflow-wrap: anywhere;
}

.cn-cfd__footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 16px;
    padding-top: 14px;
    border-top: 1px solid var(--cn-line);
}

.cn-cfd__cancel {
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

.cn-cfd__cancel:hover:not(:disabled) {
    border-color: var(--cn-line-strong);
    color: var(--cn-fg);
}

.cn-cfd__cancel:focus-visible {
    border-color: var(--cn-accent);
    outline: none;
}

.cn-cfd__confirm {
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

.cn-cfd__confirm:hover:not(:disabled) {
    filter: brightness(1.1);
}

.cn-cfd__confirm:active:not(:disabled) {
    transform: translateY(1px);
}

.cn-cfd__confirm--danger {
    background: linear-gradient(180deg, #f25656, #dc3535);
    color: #fff;
    box-shadow: 0 4px 14px rgba(239, 68, 68, 0.25);
}

.cn-cfd__confirm:focus-visible {
    outline: 2px solid var(--cn-accent);
    outline-offset: 2px;
}

.cn-cfd__confirm:disabled,
.cn-cfd__cancel:disabled {
    opacity: 0.55;
    cursor: not-allowed;
}

@media (prefers-reduced-motion: reduce) {
    .cn-cfd,
    .cn-cfd__panel {
        animation: none;
    }
}
</style>
