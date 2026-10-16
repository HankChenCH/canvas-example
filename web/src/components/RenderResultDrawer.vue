<script setup lang="ts">
/**
 * RenderResultDrawer：渲染结果抽屉（19 票，spec §4.6 / 渲染历史对比升级）。
 *
 * 渲染成功后宿主自动打开：记录切换条（服务端截最新 20 条，id 降序最新在前，
 * chip = #id · 草稿/vN · 时间）+ 当前记录缩略图网格（每张标注 frame/name，网格
 * 直用 images[].url 即 /renders 直链）、点击放大（同 /renders 直链的覆盖层）、
 * 单张 `<a download>`（建议名走 renderImageFileName 纯逻辑）与「打开原图」
 * （新标签页直开——对比两版渲染时可两开并排）。切换条状态归宿主（activeRecordId
 * 由宿主持有，本组件 emit select），默认宿主置为本次渲染记录。
 *
 * 注记两处（spec §4.6 词汇进 UI）：「渲染结果即其标注内容：草稿 = 保存后的当前
 * 草稿，vN = 该发布版本快照」与「预览仅断行参考，以终图为准」。
 *
 * 状态边界：渲染历史记录、当前记录 id、抽屉开合、模板名（下载建议名用）由宿主
 * 持有；放大态是纯呈现态归本组件（抽屉关闭或记录切换即清）。Teleport body +
 * 自带令牌块（DataSourceDrawer 先例）；非模态，画布保持可交互。
 */
import { computed, nextTick, ref, watch } from 'vue'

import type { RenderRecord } from '../api'
import { formatTimestamp } from '../editor/format'
import { renderImageFileName, renderVersionLabel } from '../editor/render'

const props = defineProps<{
    /** 抽屉开合（渲染成功宿主自动置位；关闭钮回落） */
    open: boolean
    /** 渲染历史记录（id 降序最新在前；服务端截最新 20 条） */
    records: RenderRecord[]
    /** 当前展示记录 id（null 或不在列表 = 会话尚无渲染，显空态） */
    activeRecordId: number | null
    /** 模板名（单张下载建议名的基段） */
    templateName: string
    /** 抽屉顶部偏移（缺省 92px = 编辑器顶栏 48 + 工具栏 44 之下；无工具栏宿主
     *  如列表页可传更小值贴顶） */
    top?: string
}>()

const emit = defineEmits<{
    close: []
    /** 切换当前展示记录（状态归宿主，本组件纯呈现） */
    select: [id: number]
}>()

/** 当前记录（id 寻址；切换条与网格共用） */
const active = computed(() => props.records.find((r) => r.id === props.activeRecordId) ?? null)

/** 点击放大的图（images 扁平下标 → 图与下标成对；null = 无放大覆盖层） */
const zoomedIndex = ref<number | null>(null)
const zoomEl = ref<HTMLDivElement | null>(null)

const zoomed = computed(() => {
    if (zoomedIndex.value === null || !active.value) return null
    const image = active.value.images[zoomedIndex.value]
    if (!image) return null
    return { image, index: zoomedIndex.value }
})

// 放大覆盖层为轻呈现态：出现即聚焦（Esc 关闭走同焦点元素）；抽屉关闭或记录
// 更替（切换记录/再次渲染）即清
watch(zoomedIndex, async (index) => {
    if (index === null) return
    await nextTick()
    zoomEl.value?.focus()
})
watch(
    () => [props.open, props.activeRecordId] as const,
    () => {
        zoomedIndex.value = null
    },
)
</script>

<template>
    <Teleport to="body">
        <aside v-if="open" class="cn-rrd" data-render-drawer aria-label="渲染结果抽屉" :style="top ? { top } : undefined">
            <header class="cn-rrd__header">
                <h2 class="cn-rrd__title">渲染结果</h2>
                <button type="button" class="cn-rrd__close" data-render-drawer-close aria-label="关闭渲染结果抽屉" @click="emit('close')">
                    ✕
                </button>
            </header>

            <!-- 记录切换条（渲染历史对比）：id 降序 chips，点击切换当前展示记录 -->
            <div v-if="records.length > 0" class="cn-rrd__switch" data-render-record-switch aria-label="渲染历史记录">
                <button
                    v-for="r in records"
                    :key="r.id"
                    type="button"
                    class="cn-rrd__chip"
                    :class="{ 'cn-rrd__chip--active': r.id === activeRecordId }"
                    :data-render-record-chip="r.id"
                    :title="`查看渲染记录 #${r.id}（${renderVersionLabel(r.version)}）`"
                    @click="emit('select', r.id)"
                >
                    #{{ r.id }} · {{ renderVersionLabel(r.version) }} · {{ formatTimestamp(r.createdAt) }}
                </button>
            </div>

            <div v-if="active" class="cn-rrd__body">
                <p class="cn-rrd__meta" data-render-meta>
                    渲染记录 #{{ active.id }} · {{ renderVersionLabel(active.version) }} · {{ active.createdAt }}
                </p>
                <p class="cn-rrd__note" data-render-note-stored>
                    渲染结果即其标注内容：草稿 = 保存后的当前草稿，vN = 该发布版本快照。
                </p>
                <!-- 缩略图网格直用 images[].url（/renders 直链，spec §4.6） -->
                <div class="cn-rrd__grid" data-render-grid>
                    <figure v-for="(img, i) in active.images" :key="i" class="cn-rrd__cell">
                        <button
                            type="button"
                            class="cn-rrd__thumb"
                            :data-render-thumb="i"
                            :title="`放大查看（${img.url}）`"
                            @click="zoomedIndex = i"
                        >
                            <img :src="img.url" :alt="`帧 ${img.frame} · ${img.name}`" loading="lazy" />
                        </button>
                        <figcaption class="cn-rrd__caption">
                            <span class="cn-rrd__label" :data-render-caption="i">帧 {{ img.frame }} · {{ img.name }}</span>
                            <span class="cn-rrd__actions">
                                <a
                                    class="cn-rrd__download"
                                    :href="img.url"
                                    target="_blank"
                                    rel="noopener"
                                    :data-render-open="i"
                                    :title="`新标签页打开原图（对比时可与另一记录并排）`"
                                >原图</a>
                                <a
                                    class="cn-rrd__download"
                                    :href="img.url"
                                    :download="renderImageFileName(templateName, img, i)"
                                    :data-render-download="i"
                                >下载</a>
                            </span>
                        </figcaption>
                    </figure>
                </div>
                <p class="cn-rrd__note" data-render-note-preview>预览仅断行参考，以终图为准。</p>
            </div>
            <p v-else class="cn-rrd__empty" data-render-empty>尚无渲染结果</p>
        </aside>

        <!-- 点击放大（spec §4.6 /renders 直链）：全屏覆盖层，点任意处或 Esc 关闭 -->
        <div
            v-if="open && zoomed"
            ref="zoomEl"
            class="cn-rrd__zoom"
            data-render-zoom
            tabindex="-1"
            aria-label="终图放大视图"
            @click="zoomedIndex = null"
            @keydown.esc.prevent="zoomedIndex = null"
        >
            <img
                class="cn-rrd__zoom-img"
                :src="zoomed.image.url"
                :alt="`帧 ${zoomed.image.frame} · ${zoomed.image.name}`"
            />
            <p class="cn-rrd__zoom-caption">
                <span>帧 {{ zoomed.image.frame }} · {{ zoomed.image.name }}（点击任意处关闭）</span>
                <span class="cn-rrd__actions">
                    <a
                        class="cn-rrd__download"
                        :href="zoomed.image.url"
                        target="_blank"
                        rel="noopener"
                        data-render-zoom-open
                    >原图</a>
                    <a
                        class="cn-rrd__download"
                        :href="zoomed.image.url"
                        :download="renderImageFileName(templateName, zoomed.image, zoomed.index)"
                        data-render-zoom-download
                        @click.stop
                    >下载本页</a>
                </span>
            </p>
        </div>
    </Teleport>
</template>

<style scoped>
/* 令牌与 panel-theme.css 同值：Teleport 后脱离宿主 DOM 子树，主题自带（DataSourceDrawer 先例） */
.cn-rrd {
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
    top: 92px; /* 顶栏 48 + 工具栏 44 之下（DataSourceDrawer 同位） */
    right: 12px;
    bottom: 40px;
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

.cn-rrd__header {
    display: flex;
    flex: none;
    align-items: center;
    justify-content: space-between;
    padding: 12px 16px;
    border-bottom: 1px solid var(--cn-line);
}

.cn-rrd__title {
    margin: 0;
    color: var(--cn-fg);
    font-size: 14px;
    font-weight: 600;
    line-height: 1.4;
}

.cn-rrd__close {
    padding: 2px 8px;
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: var(--cn-muted);
    font-size: 13px;
    line-height: 1.4;
    cursor: pointer;
}

.cn-rrd__close:hover {
    background: rgba(148, 163, 184, 0.07);
    color: var(--cn-fg);
}

/* 记录切换条（渲染历史对比）：横向 chips 换行排列，当前记录高亮 */
.cn-rrd__switch {
    display: flex;
    flex: none;
    flex-wrap: wrap;
    gap: 6px;
    padding: 10px 16px 0;
}

.cn-rrd__chip {
    padding: 3px 9px;
    border: 1px solid var(--cn-line-strong);
    border-radius: 999px;
    background: var(--cn-bg);
    color: var(--cn-fg-2);
    font-family: var(--cn-font-mono);
    font-size: 10.5px;
    line-height: 1.5;
    white-space: nowrap;
    cursor: pointer;
    transition:
        border-color 0.2s,
        color 0.2s,
        background 0.2s;
}

.cn-rrd__chip:hover {
    border-color: var(--cn-accent);
    color: var(--cn-accent);
}

.cn-rrd__chip--active {
    border-color: var(--cn-accent);
    background: rgba(56, 189, 248, 0.12);
    color: var(--cn-accent);
}

.cn-rrd__body {
    display: flex;
    flex-direction: column;
    padding: 12px 16px 14px;
}

/* 渲染记录 id + 版本标注 + createdAt（spec §4.6）：ISO 串原样展示（mono），不做时区换算 */
.cn-rrd__meta {
    margin: 0;
    color: var(--cn-accent);
    font-family: var(--cn-font-mono);
    font-size: 11px;
    line-height: 1.6;
    overflow-wrap: anywhere;
}

.cn-rrd__note {
    margin: 8px 0 0;
    color: var(--cn-muted);
    font-size: 11px;
    line-height: 1.6;
}

/* 缩略图网格（spec §4.6）：两列 A4 竖版缩略图，点击放大 */
.cn-rrd__grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px;
    margin-top: 10px;
}

.cn-rrd__cell {
    display: flex;
    flex-direction: column;
    gap: 4px;
    margin: 0;
    min-width: 0;
}

.cn-rrd__thumb {
    display: block;
    width: 100%;
    padding: 0;
    border: 1px solid var(--cn-line);
    border-radius: 8px;
    background: var(--cn-bg);
    cursor: zoom-in;
    overflow: hidden;
}

.cn-rrd__thumb:hover {
    border-color: var(--cn-line-strong);
}

.cn-rrd__thumb img {
    display: block;
    width: 100%;
    height: auto;
}

.cn-rrd__caption {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 8px;
    min-width: 0;
}

.cn-rrd__label {
    overflow: hidden;
    color: var(--cn-fg-2);
    font-size: 11px;
    line-height: 1.5;
    text-overflow: ellipsis;
    white-space: nowrap;
}

/* 行内动作组（原图新开 + 下载建议名）：对比时可与另一记录的原图并排 */
.cn-rrd__actions {
    display: inline-flex;
    flex: none;
    align-items: baseline;
    gap: 8px;
}

.cn-rrd__download {
    color: var(--cn-accent);
    font-size: 11px;
    line-height: 1.5;
    text-decoration: none;
}

.cn-rrd__download:hover {
    text-decoration: underline;
}

.cn-rrd__empty {
    margin: 0;
    padding: 16px;
    color: var(--cn-muted);
    font-size: 12px;
}

/* 放大覆盖层（spec §4.6 点击放大）：全屏暗底 + 原图（/renders 直链）+ 页脚标注 */
.cn-rrd__zoom {
    position: fixed;
    inset: 0;
    z-index: 50;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 10px;
    padding: 24px;
    background: rgba(2, 6, 23, 0.78);
    cursor: zoom-out;
    outline: none;
}

.cn-rrd__zoom-img {
    display: block;
    max-width: 100%;
    max-height: calc(100vh - 96px);
    border: 1px solid var(--cn-line-strong);
    border-radius: 8px;
    background: #fff;
    box-shadow: 0 24px 64px rgba(2, 6, 23, 0.6);
}

.cn-rrd__zoom-caption {
    display: flex;
    align-items: baseline;
    gap: 14px;
    margin: 0;
    color: var(--cn-fg-2);
    font-size: 12px;
    line-height: 1.5;
}
</style>
