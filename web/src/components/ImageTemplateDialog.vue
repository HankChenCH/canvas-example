<script setup lang="ts">
/**
 * ImageTemplateDialog：底图快捷建模板弹窗（24 票，spec §4.1 列表页新建入口之一）。
 *
 * 形态骨架与 NamePromptDialog 同款（模态名字输入 + Enter 确认 / Esc 取消 / 点遮罩
 * 取消 / 在途双禁用），增加本机选图区：选图事件把 File 交宿主解码像素尺寸
 * （createImageBitmap 属 DOM 通道，归宿主），选中结果经 imageName/imageInfo 两
 * props 回显。状态边界：名字草稿（defineModel）、已选图、错误文案、在途态全部
 * 由宿主持有——本组件纯呈现。Teleport body + 自带令牌块。
 */
import { nextTick, ref, watch } from 'vue'

const name = defineModel<string>('name', { required: true })

const props = defineProps<{
    /** 弹窗开合（宿主动作钮驱动） */
    open: boolean
    /** 编排在途态（期间确认/取消/换图禁用） */
    saving: boolean
    /** 错误回显：稳定码在前的可读文案（spec §2.1 按 code 判定） */
    error: string | null
    /** 已选图片文件名（null = 未选） */
    imageName: string | null
    /** 已选图片描述行（像素尺寸回显，null = 未选） */
    imageInfo: string | null
    /** 弹窗标题 */
    title?: string
    /** 名字输入框标签 */
    label?: string
    /** 语义注记（说明确认后会发生什么） */
    note?: string
    /** 确认钮文案（在途态由 confirmBusyText 接管） */
    confirmText?: string
    confirmBusyText?: string
}>()

const emit = defineEmits<{
    confirm: []
    cancel: []
    /** 本机选图：File 原样交宿主（解码尺寸 + 组装上传字节） */
    select: [file: File]
}>()

const inputEl = ref<HTMLInputElement | null>(null)

/** 开启即聚焦全选名字草稿（与 NamePromptDialog 同款） */
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

/** 选图后清空 input value：同一文件可重复选择（change 事件恒触发） */
function onFileChange(event: Event): void {
    const el = event.target as HTMLInputElement
    const file = el.files?.[0] ?? null
    el.value = ''
    if (file) emit('select', file)
}
</script>

<template>
    <Teleport to="body">
        <div
            v-if="open"
            class="cn-itd"
            data-image-prompt-backdrop
            @keydown="onKeydown"
            @click.self="emit('cancel')"
        >
            <section
                class="cn-itd__panel"
                data-image-prompt-dialog
                role="dialog"
                aria-modal="true"
                :aria-label="title"
            >
                <h2 class="cn-itd__title">{{ title }}</h2>
                <label class="cn-itd__label" for="itd-name">{{ label }}</label>
                <input
                    id="itd-name"
                    ref="inputEl"
                    v-model="name"
                    class="cn-itd__input"
                    data-image-prompt-input
                    type="text"
                    :aria-label="label"
                    :disabled="saving"
                    spellcheck="false"
                />
                <!-- 本机选图：原生 input 隐藏、label 作选择落区；accept 限图片 -->
                <label class="cn-itd__file" data-image-prompt-file-label>
                    <input
                        type="file"
                        accept="image/*"
                        class="cn-itd__file-input"
                        data-image-prompt-file
                        :disabled="saving"
                        @change="onFileChange"
                    />
                    <svg class="cn-itd__file-icon" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                        <path d="M12 15V4m0 0l-4 4m4-4l4 4" />
                        <path d="M4 15v3a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-3" />
                    </svg>
                    选择底图图片…
                </label>
                <p v-if="imageName" class="cn-itd__image" data-image-prompt-image>
                    {{ imageName }}<template v-if="imageInfo"> · {{ imageInfo }}</template>
                </p>
                <p class="cn-itd__note">{{ note }}</p>
                <p v-if="error" class="cn-itd__error" data-image-prompt-error>{{ error }}</p>
                <footer class="cn-itd__footer">
                    <button
                        type="button"
                        class="cn-itd__cancel"
                        data-image-prompt-cancel
                        :disabled="saving"
                        @click="emit('cancel')"
                    >
                        取消
                    </button>
                    <button
                        type="button"
                        class="cn-itd__confirm"
                        data-image-prompt-confirm
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
/* 令牌与 panel-theme.css 同值：Teleport 后脱离宿主 DOM 子树，主题自带
   （NamePromptDialog 先例）；样式同其升级语言（blur 遮罩 + hairline 顶线 +
   落区化选图），文件选择区为本票原生交互。 */
.cn-itd {
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
    animation: cn-itd-fade 0.22s ease-out backwards;
}

@keyframes cn-itd-fade {
    from {
        opacity: 0;
    }
}

@keyframes cn-itd-pop {
    from {
        opacity: 0;
        transform: translateY(10px) scale(0.985);
    }
}

.cn-itd__panel {
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
    animation: cn-itd-pop 0.3s cubic-bezier(0.2, 0.7, 0.3, 1) backwards;
}

/* 金→青 hairline 顶线：与列表页页头铭线同一词汇 */
.cn-itd__panel::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 2px;
    background: linear-gradient(90deg, rgba(227, 195, 127, 0.85), rgba(56, 189, 248, 0.55) 45%, transparent);
}

.cn-itd__title {
    margin: 0 0 12px;
    color: var(--cn-fg);
    font-family: var(--cn-font-display);
    font-size: 16px;
    font-weight: 600;
    letter-spacing: 0.03em;
    line-height: 1.4;
}

.cn-itd__label {
    display: block;
    margin-bottom: 5px;
    color: var(--cn-accent);
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.04em;
    line-height: 1.4;
}

.cn-itd__input {
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

.cn-itd__input:focus {
    border-color: rgba(56, 189, 248, 0.65);
    box-shadow: 0 0 0 3px rgba(56, 189, 248, 0.14);
    outline: none;
}

/* 文件选择落区：原生 input 视觉隐藏（包裹式 label 关联，点击即开选图器），
   上传图形 + 虚线框的次级动作面，hover 点亮青色 */
.cn-itd__file {
    display: block;
    margin-top: 12px;
    padding: 14px 12px;
    border: 1px dashed var(--cn-line-strong);
    border-radius: 10px;
    color: var(--cn-fg-2);
    font-size: 12px;
    text-align: center;
    cursor: pointer;
    transition:
        border-color 0.2s,
        background 0.2s,
        color 0.2s;
}

.cn-itd__file:hover {
    border-color: var(--cn-accent);
    background: rgba(56, 189, 248, 0.05);
    color: var(--cn-fg);
}

.cn-itd__file-icon {
    display: block;
    margin: 0 auto 6px;
    color: var(--cn-accent);
}

.cn-itd__file-input {
    display: none;
}

/* 已选图回显：文件名 + 像素尺寸（画布将采用的宽高），卡片化的确认chip */
.cn-itd__image {
    margin: 10px 0 0;
    padding: 8px 10px;
    border: 1px solid var(--cn-line);
    border-radius: 8px;
    background: var(--cn-bg);
    color: var(--cn-fg-2);
    font-family: var(--cn-font-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
    font-size: 11px;
    line-height: 1.5;
    overflow-wrap: anywhere;
}

.cn-itd__image::before {
    content: '✓ ';
    color: var(--cn-accent);
}

.cn-itd__note {
    margin: 10px 0 0;
    color: var(--cn-muted);
    font-size: 11px;
    line-height: 1.6;
}

.cn-itd__error {
    margin: 10px 0 0;
    color: #fca5a5;
    font-size: 12px;
    line-height: 1.5;
    overflow-wrap: anywhere;
}

.cn-itd__footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 16px;
    padding-top: 14px;
    border-top: 1px solid var(--cn-line);
}

.cn-itd__cancel {
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

.cn-itd__cancel:hover:not(:disabled) {
    border-color: var(--cn-line-strong);
    color: var(--cn-fg);
}

.cn-itd__confirm {
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

.cn-itd__confirm:hover:not(:disabled) {
    filter: brightness(1.1);
}

.cn-itd__confirm:active:not(:disabled) {
    transform: translateY(1px);
}

.cn-itd__confirm:focus-visible {
    outline: 2px solid var(--cn-accent);
    outline-offset: 2px;
}

.cn-itd__confirm:disabled,
.cn-itd__cancel:disabled,
.cn-itd__input:disabled {
    opacity: 0.55;
    cursor: not-allowed;
}

@media (prefers-reduced-motion: reduce) {
    .cn-itd,
    .cn-itd__panel {
        animation: none;
    }
}
</style>
