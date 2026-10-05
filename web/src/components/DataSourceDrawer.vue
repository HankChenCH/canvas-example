<script setup lang="ts">
/**
 * DataSourceDrawer：数据源抽屉（17 票，spec §4.3 双通道精度锚点①）。
 *
 * 一个抽屉、两条保存通道，段间距 + 各自按钮区分：
 * - 数据源段：schema / data 两个 JSON 文本域 + 「保存数据源」钮 → 宿主走
 *   PUT /templates/{id}/dataset；schema_invalid / dataset_schema_mismatch 等由
 *   宿主回显在段内（datasetError）；段内注记「渲染读取已保存的数据集」。
 * - 流链段：flowChain JSON 文本域 + 「保存流链」钮 → 宿主走文档级 PUT
 *   （canvases 一并整存）；flow_chain_invalid 等编译码由宿主回显在段内
 *   （flowChainError）；段内注记合法形态（paged 至多一个且链尾）。
 *
 * 状态边界：文本域草稿（defineModel 三连）、段内独立未保存标记、错误文案、
 * 保存在途态全部由宿主持有——本组件纯呈现，不持业务状态（抽屉标记与全局
 * saveState 指示灯分离，宿主自行计算）。Teleport body + 自带令牌块（脱离宿主
 * DOM 子树，HelpDialog/ContextMenu 先例）；非模态，画布保持可交互。
 */
const schemaText = defineModel<string>('schemaText', { required: true })
const dataText = defineModel<string>('dataText', { required: true })
const flowChainText = defineModel<string>('flowChainText', { required: true })

defineProps<{
    /** 抽屉开合（宿主顶栏「数据源」钮驱动） */
    open: boolean
    /** 段内独立未保存标记（文本域 vs 载入基线，宿主计算；≠ 全局 saveState） */
    datasetDirty: boolean
    flowChainDirty: boolean
    /** 各自保存通道在途态（数据源段 PUT /dataset；流链段 = 文档级 PUT） */
    datasetSaving: boolean
    flowChainSaving: boolean
    /** 段内错误回显：稳定 code 在前的可读文案（spec §2.1 按 code 判定） */
    datasetError: string | null
    flowChainError: string | null
}>()

const emit = defineEmits<{
    close: []
    saveDataset: []
    saveFlowchain: []
}>()
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

            <!-- 段一：数据源（schema/data 文本域 + 显式「保存数据源」→ PUT /dataset） -->
            <section class="cn-dsw__section" data-dataset-section aria-label="数据源">
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
                <p class="cn-dsw__note">渲染读取已保存的数据集——渲染终图以服务端存储态为准，保存前渲染用旧数据。</p>
                <p v-if="datasetError" class="cn-dsw__error" data-dataset-error>{{ datasetError }}</p>
                <footer class="cn-dsw__footer">
                    <span v-if="datasetDirty" class="cn-dsw__unsaved" data-dataset-unsaved>● 未保存</span>
                    <button
                        type="button"
                        class="cn-dsw__save"
                        data-save-dataset
                        :disabled="datasetSaving"
                        @click="emit('saveDataset')"
                    >
                        {{ datasetSaving ? '保存中…' : '保存数据源' }}
                    </button>
                </footer>
            </section>

            <!-- 段二：流链（flowChain 文本域 + 「保存流链」→ 文档级 PUT，canvases 一并整存） -->
            <section class="cn-dsw__section" data-flowchain-section aria-label="流链">
                <label class="cn-dsw__label" for="dsw-flowchain">flowChain（FlowChainNode[] JSON；空链填 null）</label>
                <textarea
                    id="dsw-flowchain"
                    v-model="flowChainText"
                    class="cn-dsw__input"
                    data-flowchain-input
                    rows="6"
                    spellcheck="false"
                ></textarea>
                <p class="cn-dsw__note">
                    合法形态：节点为封闭 4 键 <code>{ frame, mode, quota?, omitIfEmpty? }</code
                    >，frame 为帧下标（0 起）、mode ∈ fixed｜paged、quota 仅 fixed 合法；paged 至多一个且必须在链尾；
                    空链（null）= 不经流链逐帧全量填充。保存走文档级通道，各帧内容一并整存。
                </p>
                <p v-if="flowChainError" class="cn-dsw__error" data-flowchain-error>{{ flowChainError }}</p>
                <footer class="cn-dsw__footer">
                    <span v-if="flowChainDirty" class="cn-dsw__unsaved" data-flowchain-unsaved>● 未保存</span>
                    <button
                        type="button"
                        class="cn-dsw__save"
                        data-save-flowchain
                        :disabled="flowChainSaving"
                        @click="emit('saveFlowchain')"
                    >
                        {{ flowChainSaving ? '保存中…' : '保存流链' }}
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

/* 段间距 + 各自按钮区分（spec §4.3）：段间 border-top 分隔，钮在各段页脚右对齐 */
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

.cn-dsw__note {
    margin: 8px 0 0;
    color: var(--cn-muted);
    font-size: 11px;
    line-height: 1.6;
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
    justify-content: space-between;
    gap: 8px;
    margin-top: 10px;
}

/* 段内独立未保存小标记（spec §4.4）：琥珀色 ● 与全局指示灯同词汇、独立计算 */
.cn-dsw__unsaved {
    color: #f59e0b;
    font-size: 12px;
    white-space: nowrap;
}

.cn-dsw__save {
    margin-left: auto;
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
