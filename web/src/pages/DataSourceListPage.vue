<script setup lang="ts">
// 数据源管理列表页（25 票，spec §4.1 /datasources 行；28 票加卡片删除）：
// 数据源自 23 票起是独立 HTTP 资源，此前 web 侧唯一入口是编辑器抽屉——新建须
// 「进模板 → 未绑『创建并绑定』或已绑『另存为新』」绕行。本页做独立盘点面：
// 摘要卡片 = name + templateCount + updatedAt（GET /api/datasources 摘要恰这三
// 字段），主区进 /datasources/{id} 编辑页，头部「＋ 新建数据源」→ /datasources/new。
// 卡片操作条「删除」（#6e）：弹确认框 → DELETE /api/datasources/{id} → 204 后
// 本地移除卡片，失败（含服务端兜底 409 data_source_in_use）稳定码在前在框内
// 回显可重试；templateCount > 0 时删除钮禁用（引用面卡片 pill 可见，title 注记
// 先解绑）——引用不悬空。
import { onMounted, ref } from 'vue'

import { api, formatApiError, type DataSourceSummary } from '../api'
import ConfirmDialog from '../components/ConfirmDialog.vue'

const sources = ref<DataSourceSummary[] | null>(null)
const errorText = ref<string | null>(null)

// 卡片删除态（28 票）：确认框目标卡、错误文案与在途态由本页持有（组件纯呈现）
const deleteOpen = ref(false)
const deleteTarget = ref<DataSourceSummary | null>(null)
const deleteError = ref<string | null>(null)
const deleting = ref(false)

onMounted(async () => {
    try {
        sources.value = await api.listDataSources()
    } catch (e) {
        errorText.value = formatApiError(e)
    }
})

function openDelete(s: DataSourceSummary): void {
    deleteTarget.value = s
    deleteError.value = null
    deleteOpen.value = true
}

function closeDelete(): void {
    deleteOpen.value = false
    deleteError.value = null
    deleteTarget.value = null
}

/** 确认删除：DELETE /datasources/{id} → 204 后本地移除卡片（空表自然落空态）；
 *  失败文案留在确认框内（稳定码在前），可重试或取消 */
async function runDelete(): Promise<void> {
    if (deleting.value || !deleteTarget.value) return
    deleting.value = true
    deleteError.value = null
    const target = deleteTarget.value
    try {
        await api.deleteDataSource(target.id)
        sources.value = (sources.value ?? []).filter((d) => d.id !== target.id)
        closeDelete()
    } catch (e) {
        deleteError.value = formatApiError(e)
    } finally {
        deleting.value = false
    }
}

function formatUpdatedAt(iso: string): string {
    const t = new Date(iso)
    return Number.isNaN(t.getTime()) ? iso : t.toLocaleString('zh-CN', { hour12: false })
}

// ── 档案卡纯呈现辅助（登记处语言：索引杠 / 进场 stagger）──

/** 卡片索引杠：列表位次两位补零（#03）——盘点面按位次编号，与实体 id 无关 */
function indexOf(index: number): string {
    return `#${String(index + 1).padStart(2, '0')}`
}

/** 进场 stagger：前 8 张依次延迟 60ms，其后不再累加 */
function riseDelay(index: number): string {
    return `${Math.min(index, 7) * 60}ms`
}
</script>

<template>
    <main class="pc-atelier relative min-h-screen text-zinc-200">
        <div class="relative z-10 mx-auto max-w-5xl px-6 pb-16 pt-14">
            <RouterLink
                to="/"
                class="pc-rise mb-8 inline-flex items-center gap-1.5 rounded-lg border border-[#223252] px-3 py-1.5 font-mono text-xs text-zinc-400 transition-colors hover:border-sky-700 hover:text-sky-300"
            >
                ← 返回模板列表
            </RouterLink>

            <!-- 页头：金杠 kicker + 衬线大标题 + 新建动作 -->
            <header class="pc-rise" style="animation-delay: 60ms">
                <p class="pc-kicker mb-4">Data Source Registry · 数据源登记处</p>
                <div class="flex flex-wrap items-end justify-between gap-x-8 gap-y-5">
                    <div class="min-w-0">
                        <h1 class="pc-display mb-2 text-4xl leading-tight text-zinc-50">数据源管理</h1>
                        <p class="max-w-xl text-sm leading-6 text-zinc-500">
                            数据源是独立资源：模板经引用绑定，编辑共享实体影响所有引用它的模板
                        </p>
                    </div>
                    <RouterLink
                        to="/datasources/new"
                        data-new-datasource
                        class="flex-none rounded-lg bg-gradient-to-b from-sky-400 to-sky-500 px-3.5 py-2 text-sm font-semibold text-[#06202b] shadow-[0_0_0_1px_rgba(56,189,248,0.35),0_8px_24px_rgba(56,189,248,0.18)] transition hover:brightness-110 active:translate-y-px"
                        title="新建数据源：录入名字 + schema（draft-07）+ data（JSON），创建为独立实体；之后可在模板编辑器的数据源抽屉绑定使用"
                    >
                        ＋ 新建数据源
                    </RouterLink>
                </div>
                <div class="mt-8 h-px bg-gradient-to-r from-sky-400/50 via-sky-400/10 to-transparent" aria-hidden="true"></div>
            </header>

            <!-- 读取失败态（服务端未起等） -->
            <div
                v-if="errorText"
                class="pc-rise mt-6 flex items-start gap-2.5 rounded-xl border border-red-900/60 bg-red-950/40 px-4 py-3 text-sm leading-6 text-red-300"
            >
                <span class="mt-0.5 font-mono text-xs text-red-400/90" aria-hidden="true">✕</span>
                <span>数据源列表读取失败：{{ errorText }}</span>
            </div>

            <!-- 空态（合法初始态：引导新建） -->
            <div
                v-else-if="sources && sources.length === 0"
                class="pc-rise mt-6 rounded-xl border border-dashed border-[#24345a] bg-[#0a1322]/60 px-6 py-16 text-center"
            >
                <svg class="mx-auto mb-5 opacity-70" width="44" height="44" viewBox="0 0 24 24" fill="none" stroke="rgba(125,211,252,0.55)" stroke-width="1.1" aria-hidden="true">
                    <ellipse cx="12" cy="5" rx="8" ry="3" />
                    <path d="M4 5v14c0 1.7 3.6 3 8 3s8-1.3 8-3V5" />
                    <path d="M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3" />
                </svg>
                <h2 class="pc-display mb-2 text-lg text-zinc-300">登记处尚空</h2>
                <p class="mx-auto max-w-md text-sm leading-6 text-zinc-500">
                    暂无数据源——点击右上角「＋ 新建数据源」创建；创建后可在模板编辑器的数据源抽屉绑定使用。
                </p>
            </div>

            <!-- 盘点卡片（档案卡语言）：金色索引杠 + 引用计数 pill；主区进编辑页，
                 底部操作条「删除」（28 票，#6e——有引用禁用，服务端 409 兜底） -->
            <ul v-else-if="sources" class="mt-8 grid gap-4 sm:grid-cols-2">
                <li
                    v-for="(s, i) in sources"
                    :key="s.id"
                    class="pc-card pc-card--file pc-rise overflow-hidden rounded-xl"
                    :style="{ animationDelay: riseDelay(i) }"
                >
                    <RouterLink :to="`/datasources/${s.id}`" class="group block px-5 pb-4 pt-4 outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-sky-500/70">
                        <div class="flex items-center justify-between gap-3">
                            <span class="font-mono text-[11px] tracking-[0.2em] text-[#e3c37f]/85">{{ indexOf(i) }}</span>
                            <span
                                class="rounded-full border px-2 py-0.5 text-[11px] leading-4 transition-colors"
                                :class="
                                    s.templateCount > 0
                                        ? 'border-sky-800/70 bg-sky-950/40 text-sky-300'
                                        : 'border-zinc-800 bg-zinc-900/60 text-zinc-500'
                                "
                            >
                                {{ s.templateCount }} 个模板引用
                            </span>
                        </div>
                        <div class="mt-2 truncate text-[15px] font-medium text-zinc-100 transition-colors group-hover:text-sky-300">{{ s.name }}</div>
                        <div class="mt-1 font-mono text-[11px] text-zinc-500">
                            更新于 {{ formatUpdatedAt(s.updatedAt) }}
                        </div>
                    </RouterLink>
                    <div class="flex items-center justify-end border-t border-[#1b2740] px-4 py-2.5">
                        <button
                            type="button"
                            data-card-delete
                            class="rounded-md border border-red-900/60 px-2.5 py-1 text-xs text-red-300 transition-colors hover:border-red-600 hover:bg-red-950/40 disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:border-red-900/60 disabled:hover:bg-transparent"
                            :disabled="s.templateCount > 0"
                            :title="
                                s.templateCount > 0
                                    ? `被 ${s.templateCount} 个模板引用：先在编辑器解绑（或删除引用模板）后可删除`
                                    : '删除数据源：schema 与示例数据（data）随之删除，不可恢复'
                            "
                            @click="openDelete(s)"
                        >
                            删除
                        </button>
                    </div>
                </li>
            </ul>

            <!-- 载入骨架（读取在途）：四张档案卡占位呼吸 -->
            <div v-else class="mt-8 grid gap-4 sm:grid-cols-2" aria-busy="true">
                <div v-for="n in 4" :key="n" class="pc-card overflow-hidden rounded-xl">
                    <div class="animate-pulse px-5 py-4">
                        <div class="flex items-center justify-between">
                            <div class="h-2.5 w-8 rounded bg-zinc-800/80"></div>
                            <div class="h-4 w-20 rounded-full bg-zinc-800/60"></div>
                        </div>
                        <div class="mt-3 h-3.5 w-1/2 rounded bg-zinc-800/80"></div>
                        <div class="mt-2 h-2.5 w-1/3 rounded bg-zinc-800/60"></div>
                    </div>
                </div>
            </div>

            <!-- 页脚铭线 -->
            <footer class="mt-16 border-t border-[#16233c] pt-5">
                <p class="font-mono text-[11px] tracking-[0.25em] text-zinc-600">CANVAS NEXT · 证书批量生成示例</p>
            </footer>

            <!-- 删除确认框（spec §2.4 #6e，28 票）：错误留在框内可重试，开启聚焦取消钮；
                 被引用的实体到不了这里（删除钮禁用），服务端 409 data_source_in_use 兜底 -->
            <ConfirmDialog
                :open="deleteOpen"
                :busy="deleting"
                :error="deleteError"
                danger
                title="删除数据源"
                :note="`删除「${deleteTarget?.name ?? ''}」不可恢复：schema 与示例数据（data）一并删除。引用它的模板不受影响——被引用时删除会被拒绝（先解绑或删除引用模板）。`"
                confirm-text="删除"
                confirm-busy-text="删除中…"
                @confirm="runDelete"
                @cancel="closeDelete"
            />
        </div>
    </main>
</template>

<style scoped>
/* 档案卡：左侧金色索引杠（登记册抽屉的导引片），hover 时点亮 */
.pc-card--file::before {
    content: '';
    position: absolute;
    top: 0;
    bottom: 0;
    left: 0;
    width: 2px;
    background: linear-gradient(180deg, var(--pc-gold), transparent 85%);
    opacity: 0;
    transition: opacity 0.25s;
}

.pc-card--file:hover::before {
    opacity: 0.85;
}
</style>
