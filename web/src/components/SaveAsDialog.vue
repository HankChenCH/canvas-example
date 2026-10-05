<script setup lang="ts">
/**
 * SaveAsDialog：另存为弹窗（18 票，spec §4.5 两连调用精度锚点③）。
 *
 * 模态名字输入：确认后宿主走两连调用（POST /templates + PUT /templates/{newId}/dataset，
 * 连带复制数据源），成功后宿主重绑会话状态并 router.replace 到副本。Enter 确认 /
 * Esc 取消（输入法合成中的 Enter 只上屏不确认）；点遮罩取消。在途态下确认与取消
 * 均禁用——半途中断会让「已建副本」与界面状态脱钩。
 *
 * 状态边界：名字草稿（defineModel）、错误文案、在途态全部由宿主持有——本组件
 * 纯呈现，不持业务状态（DataSourceDrawer 先例）。Teleport body + 自带令牌块。
 */
import { nextTick, ref, watch } from 'vue'

const name = defineModel<string>('name', { required: true })

const props = defineProps<{
    /** 弹窗开合（宿主顶栏「另存为」钮驱动） */
    open: boolean
    /** 两连调用在途态（期间确认/取消禁用） */
    saving: boolean
    /** 错误回显：稳定码在前的可读文案（spec §2.1 按 code 判定） */
    error: string | null
}>()

const emit = defineEmits<{
    confirm: []
    cancel: []
}>()

const inputEl = ref<HTMLInputElement | null>(null)

/** 开启即聚焦全选：预填当前模板名，直接键入即整体替换 */
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
</script>

<template>
    <Teleport to="body">
        <div v-if="open" class="cn-sad" data-save-as-backdrop @keydown="onKeydown" @click.self="emit('cancel')">
            <section class="cn-sad__panel" data-save-as-dialog role="dialog" aria-modal="true" aria-label="另存为">
                <h2 class="cn-sad__title">另存为副本</h2>
                <label class="cn-sad__label" for="sad-name">副本名</label>
                <input
                    id="sad-name"
                    ref="inputEl"
                    v-model="name"
                    class="cn-sad__input"
                    data-save-as-name-input
                    type="text"
                    aria-label="副本名"
                    :disabled="saving"
                    spellcheck="false"
                />
                <p class="cn-sad__note">
                    以当前内容（各帧 + 流链 + 数据源）创建副本模板，成功后跳转到副本继续编辑；原模板保持不变。
                </p>
                <p v-if="error" class="cn-sad__error" data-save-as-error>{{ error }}</p>
                <footer class="cn-sad__footer">
                    <button
                        type="button"
                        class="cn-sad__cancel"
                        data-save-as-cancel
                        :disabled="saving"
                        @click="emit('cancel')"
                    >
                        取消
                    </button>
                    <button
                        type="button"
                        class="cn-sad__confirm"
                        data-save-as-confirm
                        :disabled="saving"
                        @click="emit('confirm')"
                    >
                        {{ saving ? '另存中…' : '另存为' }}
                    </button>
                </footer>
            </section>
        </div>
    </Teleport>
</template>

<style scoped>
/* 令牌与 panel-theme.css 同值：Teleport 后脱离宿主 DOM 子树，主题自带（DataSourceDrawer 先例）。
   模态：全屏遮罩截断交互，面板居中。 */
.cn-sad {
    position: fixed;
    inset: 0;
    z-index: 40;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(2, 6, 23, 0.55);
}

.cn-sad__panel {
    --cn-bg: #0b1220;
    --cn-bg-elevated: #101a2e;
    --cn-fg: #e6edf7;
    --cn-fg-2: #c3cddd;
    --cn-muted: #7c8ca5;
    --cn-line: #1e2a40;
    --cn-line-strong: #2a3a58;
    --cn-accent: #38bdf8;
    --cn-on-accent: #06202b;

    width: 420px;
    max-width: calc(100vw - 32px);
    padding: 16px;
    background: var(--cn-bg-elevated);
    border: 1px solid var(--cn-line);
    border-radius: 12px;
    box-shadow: 0 24px 64px rgba(2, 6, 23, 0.6);
}

.cn-sad__title {
    margin: 0 0 10px;
    color: var(--cn-fg);
    font-size: 14px;
    font-weight: 600;
    line-height: 1.4;
}

.cn-sad__label {
    display: block;
    margin-bottom: 4px;
    color: var(--cn-accent);
    font-size: 11px;
    font-weight: 600;
    line-height: 1.4;
}

.cn-sad__input {
    box-sizing: border-box;
    width: 100%;
    padding: 7px 10px;
    border: 1px solid var(--cn-line);
    border-radius: 8px;
    background: var(--cn-bg);
    color: var(--cn-fg);
    font-size: 13px;
}

.cn-sad__input:focus {
    border-color: var(--cn-line-strong);
    outline: none;
}

.cn-sad__note {
    margin: 8px 0 0;
    color: var(--cn-muted);
    font-size: 11px;
    line-height: 1.6;
}

.cn-sad__error {
    margin: 8px 0 0;
    color: #fca5a5;
    font-size: 12px;
    line-height: 1.5;
    overflow-wrap: anywhere;
}

.cn-sad__footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 14px;
}

.cn-sad__cancel {
    padding: 5px 12px;
    border: 1px solid var(--cn-line);
    border-radius: 8px;
    background: transparent;
    color: var(--cn-fg-2);
    font-size: 13px;
    cursor: pointer;
}

.cn-sad__cancel:hover:not(:disabled) {
    border-color: var(--cn-line-strong);
}

.cn-sad__confirm {
    padding: 5px 12px;
    border: none;
    border-radius: 8px;
    background: var(--cn-accent);
    color: var(--cn-on-accent);
    font-size: 13px;
    font-weight: 600;
    cursor: pointer;
}

.cn-sad__confirm:hover:not(:disabled) {
    filter: brightness(1.12);
}

.cn-sad__confirm:disabled,
.cn-sad__cancel:disabled,
.cn-sad__input:disabled {
    opacity: 0.55;
    cursor: not-allowed;
}
</style>
