<script setup lang="ts">
/**
 * VersionHistoryDrawer：版本历史抽屉（template-draft-publish 批 B ③，spec §6 / 票 05）。
 *
 * 版本史 = 发布史（版本线线性：发布把草稿固化为新版本，回滚把历史版本内容复制回
 * 草稿、永不造版）：列出已发布版本（版本号 + 发布时间，version 降序由服务端保证；
 * 空列表 = 未发布推导态，给行动指引空态；null = 列表未知（未加载或拉取失败）显
 * 「版本列表不可用」——未知 ≠ 未发布，不假显「尚未发布过版本」，评审修复 F1），
 * 行内「回滚」钮 → 宿主弹 ConfirmDialog 二次确认（服务端零守卫，票 04：覆盖目标
 * = 最后保存的草稿）；行内「渲染」钮 → 宿主 POST render {version:N} 渲染该版本
 * 快照（版本级预览：不触草稿列、不产生版本行；结果进渲染历史抽屉支持记录切换
 * 对比）。
 *
 * 状态边界：版本列表（含未知态）、回滚确认目标、渲染在途、错误与在途态全由宿主
 * 持有——本组件纯呈现（FlowChainDrawer/DataSourceDrawer 先例：Teleport body +
 * 自带令牌块，非模态，画布保持可交互）。仍不做（票 05 图级 Q3）：版本画布内
 * 预览/对比、版本删除/重命名、发布备注——UI 只露版本号 + 时间 + 动作。
 */
import { computed } from 'vue'

import type { TemplateVersionSummary } from '../api'
import { formatTimestamp } from '../editor/format'
import { versionListState } from '../editor/publish'

const props = defineProps<{
    /** 抽屉开合（顶栏「版本」钮驱动） */
    open: boolean
    /** 版本列表（version 降序；null = 未知/未加载或拉取失败，≠ 未发布；空数组 = 未发布） */
    versions: readonly TemplateVersionSummary[] | null
    /** 回滚在途（行内钮禁用） */
    reverting: boolean
    /** 在途回滚目标版本（该行显「回滚中…」，null = 无在途） */
    revertingVersion: number | null
    /** 行内渲染在途（渲染钮禁用） */
    rendering: boolean
    /** 在途渲染目标版本（该行显「渲染中…」，null = 无在途） */
    renderingVersion: number | null
}>()

const emit = defineEmits<{
    close: []
    /** 行内回滚：宿主走 ConfirmDialog → POST revert → 会话替换（spec §6） */
    revert: [version: TemplateVersionSummary]
    /** 行内渲染：宿主 POST render {version:N} → 渲染历史刷新 + 结果抽屉 */
    render: [version: TemplateVersionSummary]
}>()

/** 列表态分支（versionListState 纯逻辑单测钉死三态：unknown/unpublished/ready） */
const listState = computed(() => versionListState(props.versions))
/** 行迭代（ready 态专用；unknown/unpublished 不可达，?? [] 仅类型面收窄） */
const rows = computed(() => props.versions ?? [])
</script>

<template>
    <Teleport to="body">
        <aside v-if="open" class="cn-vhd" data-version-drawer aria-label="版本历史">
            <header class="cn-vhd__header">
                <h2 class="cn-vhd__title">版本历史</h2>
                <button type="button" class="cn-vhd__close" data-drawer-close aria-label="关闭版本历史" @click="emit('close')">
                    ✕
                </button>
            </header>

            <section class="cn-vhd__section">
                <p class="cn-vhd__note">
                    版本史即发布史：发布把草稿固化为新版本；回滚把历史版本内容复制回草稿（不产生新版本），确认后当前草稿被覆盖。
                    行内「渲染」查看该版本渲染效果，不影响草稿与版本线。
                </p>

                <!-- 列表未知：null = 未加载或拉取失败（≠ 未发布，不撒谎——评审修复
                     F1）；发布/回滚成功或重进编辑器后列表随宿主刷新恢复 -->
                <p v-if="listState === 'unknown'" class="cn-vhd__empty" data-version-unavailable>
                    版本列表不可用（拉取失败或尚未加载）——重进编辑器，或在发布/回滚成功后自动恢复。
                </p>

                <!-- 空态：空 versions = 未发布（推导态，spec §3），给行动指引 -->
                <p v-else-if="listState === 'unpublished'" class="cn-vhd__empty" data-version-empty>
                    尚未发布过版本——顶栏「发布」把当前草稿固化为 v1。
                </p>

                <!-- 版本行：版本号 + 发布时间 + 行内渲染/回滚（version 降序，最新在前） -->
                <template v-else>
                    <div v-for="v in rows" :key="v.version" class="cn-vhd__row" :data-version-row="v.version">
                        <span class="cn-vhd__version">v{{ v.version }}</span>
                        <span class="cn-vhd__time">发布于 {{ formatTimestamp(v.createdAt) }}</span>
                        <button
                            type="button"
                            class="cn-vhd__render"
                            data-version-render
                            :disabled="rendering || reverting"
                            :title="`渲染 v${v.version} 查看效果（版本快照渲染，不影响草稿与版本线；结果进渲染结果抽屉）`"
                            @click="emit('render', v)"
                        >
                            {{ rendering && renderingVersion === v.version ? '渲染中…' : '渲染' }}
                        </button>
                        <button
                            type="button"
                            class="cn-vhd__revert"
                            data-version-revert
                            :disabled="reverting || rendering"
                            :title="`回滚到 v${v.version}：把该版本内容复制回草稿（覆盖当前草稿，需确认）`"
                            @click="emit('revert', v)"
                        >
                            {{ reverting && revertingVersion === v.version ? '回滚中…' : '回滚' }}
                        </button>
                    </div>
                </template>
            </section>
        </aside>
    </Teleport>
</template>

<style scoped>
/* 令牌与 panel-theme.css 同值：Teleport 后脱离宿主 DOM 子树，主题自带（抽屉先例）。
   几何与 FlowChainDrawer 同位（顶栏 48 + 工具栏 44 之下），宽度收窄为列表形态。 */
.cn-vhd {
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
    bottom: 40px; /* 状态栏可见 */
    z-index: 30;
    display: flex;
    flex-direction: column;
    width: 430px;
    max-width: calc(100vw - 24px);
    overflow-y: auto;
    background: var(--cn-bg-elevated);
    border: 1px solid var(--cn-line);
    border-radius: 12px;
    box-shadow: 0 24px 64px rgba(2, 6, 23, 0.6);
}

.cn-vhd__header {
    display: flex;
    flex: none;
    align-items: center;
    justify-content: space-between;
    padding: 12px 16px;
    border-bottom: 1px solid var(--cn-line);
}

.cn-vhd__title {
    margin: 0;
    color: var(--cn-fg);
    font-size: 14px;
    font-weight: 600;
    line-height: 1.4;
}

.cn-vhd__close {
    padding: 2px 8px;
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: var(--cn-muted);
    font-size: 13px;
    line-height: 1.4;
    cursor: pointer;
}

.cn-vhd__close:hover {
    background: rgba(148, 163, 184, 0.07);
    color: var(--cn-fg);
}

.cn-vhd__section {
    display: flex;
    flex-direction: column;
    padding: 12px 16px 14px;
}

.cn-vhd__note {
    margin: 0 0 4px;
    color: var(--cn-muted);
    font-size: 11px;
    line-height: 1.6;
}

.cn-vhd__empty {
    margin: 12px 0;
    padding: 14px;
    border: 1px dashed var(--cn-line-strong);
    border-radius: 8px;
    color: var(--cn-muted);
    font-size: 12px;
    line-height: 1.6;
    text-align: center;
}

/* 版本行：号 + 时间 + 回滚，行间 border 分隔（FlowChainDrawer 帧行同词汇） */
.cn-vhd__row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 0;
}

.cn-vhd__row + .cn-vhd__row {
    border-top: 1px solid var(--cn-line);
}

.cn-vhd__version {
    flex: none;
    min-width: 34px;
    padding: 2px 8px;
    border: 1px solid var(--cn-line-strong);
    border-radius: 6px;
    background: var(--cn-bg);
    color: var(--cn-fg);
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-size: 12px;
    text-align: center;
}

.cn-vhd__time {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    color: var(--cn-muted);
    font-size: 12px;
    line-height: 1.5;
    text-overflow: ellipsis;
    white-space: nowrap;
}

/* 行内动作钮：渲染（次级查看动作）与回滚（覆盖性动作）同款底形，回滚 hover 提
   红以示覆盖语义区分 */
.cn-vhd__render,
.cn-vhd__revert {
    flex: none;
    padding: 4px 12px;
    border: 1px solid var(--cn-line-strong);
    border-radius: 8px;
    background: transparent;
    color: var(--cn-fg-2);
    font-size: 12px;
    cursor: pointer;
    transition:
        border-color 0.2s,
        color 0.2s;
}

.cn-vhd__render:hover:not(:disabled) {
    border-color: var(--cn-accent);
    color: var(--cn-accent);
}

.cn-vhd__revert:hover:not(:disabled) {
    border-color: #f87171;
    color: #fca5a5;
}

.cn-vhd__render:disabled,
.cn-vhd__revert:disabled {
    opacity: 0.55;
    cursor: not-allowed;
}
</style>
