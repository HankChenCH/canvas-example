<script setup lang="ts">
// 模板列表（spec §4.1 / 14 票目录页；22 票加从零新建入口；24 票加底图快捷入口；
// 卡片操作修订加删除与直渲染）：卡片 = name + updatedAt（GET /api/templates 摘要
// 恰这三字段）+ 操作条。渲染：直调 renderTemplate（POST /templates/{id}/render，
// 无 body），成功自动开结果抽屉（RenderResultDrawer 复用编辑器同款，只显本次会话
// 最近一次；top 传小值贴顶——列表页无编辑器工具栏），失败页面错误条回显稳定码在
// 前文案。删除：弹确认框（ConfirmDialog，开启聚焦取消钮防误触）→ deleteTemplate
// （DELETE /templates/{id}，spec §2.4 #10：级联清渲染记录行，产物文件保留）→
// 204 后本地移除卡片，失败在确认框内回显可重试。首访重定向由 router 守卫处理，
// 本页另负责两个新建入口——「＋ 新建空白模板」弹名字输入（NamePromptDialog）→
// createBlankTemplate 单调用 POST /api/templates（单帧空白 A4 图 + flowChain
// null，spec §2.4 #3 载荷形状，22 票）；「＋ 以图片新建」弹名字 + 选图
// （ImageTemplateDialog，选图即 createImageBitmap 解码像素尺寸就地回显）→
// createImageTemplate 先 POST /api/assets 后 POST /api/templates（画布宽高 =
// 图片像素宽高、单层「底图」铺满，24 票）→ 201 后跳 /editor/{id}。取消不建
// （弃置模板可经卡片删除回收）。
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import { api, formatApiError, type TemplateSummary } from '../api'
import { createBlankTemplate, createImageTemplate } from '../editor/newtemplate'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import ImageTemplateDialog from '../components/ImageTemplateDialog.vue'
import NamePromptDialog from '../components/NamePromptDialog.vue'
import RenderResultDrawer from '../components/RenderResultDrawer.vue'
import type { RenderRecord } from '../api'

const router = useRouter()
const templates = ref<TemplateSummary[] | null>(null)
const errorText = ref<string | null>(null)

// 新建弹窗态：名字草稿预填默认名、错误文案与在途态全由本页持有（组件纯呈现）
const NEW_TEMPLATE_DEFAULT_NAME = '未命名模板'
const newOpen = ref(false)
const newName = ref(NEW_TEMPLATE_DEFAULT_NAME)
const newError = ref<string | null>(null)
const newCreating = ref(false)

// 新建入口折叠态：两种新建方式收进一个触发钮的菜单——选中、点外（透明截断
// 层）与 Esc 均收起，收起后再由对应弹窗接管
const newMenuOpen = ref(false)

function pickNewBlank(): void {
    newMenuOpen.value = false
    openNewTemplate()
}

function pickNewImage(): void {
    newMenuOpen.value = false
    openImageTemplate()
}

// 底图新建弹窗态（24 票）：名字/错误/在途同惯例；已选底图持文件 + 浏览器解码出
// 的像素尺寸（createImageBitmap 属 DOM 通道，留在本页；尺寸同时作回显与载荷来源）
const imageOpen = ref(false)
const imageName = ref(NEW_TEMPLATE_DEFAULT_NAME)
const imageError = ref<string | null>(null)
const imageCreating = ref(false)
const pendingImage = ref<{ file: File; width: number; height: number } | null>(null)
const pendingImageName = ref<string | null>(null)
const pendingImageInfo = ref<string | null>(null)

// 卡片渲染态：在途以 id 记（渲染钮全局禁用，仅在途卡显「渲染中…」）；结果
// 抽屉复用编辑器同款记录面（record + 该卡名作下载建议名基段）
const renderingId = ref<number | null>(null)
const renderDrawerOpen = ref(false)
const lastRender = ref<RenderRecord | null>(null)
const lastRenderName = ref('')
// 卡片操作失败回显（渲染失败等非弹窗通道；新一轮操作开始即清）
const actionError = ref<string | null>(null)

// 卡片删除态：确认框目标卡、错误文案与在途态由本页持有
const deleteOpen = ref(false)
const deleteTarget = ref<TemplateSummary | null>(null)
const deleteError = ref<string | null>(null)
const deleting = ref(false)

onMounted(async () => {
    try {
        templates.value = await api.listTemplates()
    } catch (e) {
        errorText.value = formatApiError(e)
    }
})

function openNewTemplate(): void {
    newName.value = NEW_TEMPLATE_DEFAULT_NAME
    newError.value = null
    newOpen.value = true
}

function closeNewTemplate(): void {
    newOpen.value = false
    newError.value = null
}

/** 确认新建：createBlankTemplate 内做名字裁剪/空名拒绝与失败归因（稳定码在前）；
 *  成功跳编辑器（列表→编辑器跨组件导航，编辑器整页载入新模板） */
async function runCreateBlank(): Promise<void> {
    if (newCreating.value) return
    newCreating.value = true
    const result = await createBlankTemplate({ name: newName.value, createTemplate: api.createTemplate })
    newCreating.value = false
    if (!result.ok) {
        newError.value = result.message
        return
    }
    newOpen.value = false
    await router.push(`/editor/${result.record.id}`)
}

function openImageTemplate(): void {
    imageName.value = NEW_TEMPLATE_DEFAULT_NAME
    imageError.value = null
    pendingImage.value = null
    pendingImageName.value = null
    pendingImageInfo.value = null
    imageOpen.value = true
}

function closeImageTemplate(): void {
    imageOpen.value = false
    imageError.value = null
}

/** 选图即解码（createImageBitmap）：像素尺寸就地回显 + 解码失败就地拒绝（确认前
 *  的闸）；换图重解码覆盖旧选择 */
async function onImageSelected(file: File): Promise<void> {
    imageError.value = null
    try {
        const bitmap = await createImageBitmap(file)
        pendingImage.value = { file, width: bitmap.width, height: bitmap.height }
        pendingImageName.value = file.name
        pendingImageInfo.value = `${bitmap.width} × ${bitmap.height} px`
        bitmap.close()
    } catch {
        pendingImage.value = null
        pendingImageName.value = null
        pendingImageInfo.value = null
        imageError.value = '无法读取图片，请选择有效的图片文件'
    }
}

/** 确认底图新建：字节读自已选文件 → createImageTemplate 内做名字/选图校验、
 *  先上传后建模板与失败归因（稳定码在前）；成功跳编辑器 */
async function runCreateImage(): Promise<void> {
    if (imageCreating.value) return
    imageCreating.value = true
    const selected = pendingImage.value
    const result = await createImageTemplate({
        name: imageName.value,
        image: selected === null ? null : {
            name: selected.file.name,
            mime: selected.file.type,
            bytes: new Uint8Array(await selected.file.arrayBuffer()),
            width: selected.width,
            height: selected.height,
        },
        uploadAsset: api.uploadAsset,
        createTemplate: api.createTemplate,
    })
    imageCreating.value = false
    if (!result.ok) {
        imageError.value = result.message
        return
    }
    imageOpen.value = false
    await router.push(`/editor/${result.record.id}`)
}

/** 卡片直渲染（spec §4.6 同语义：以服务端存储态为准，无 body）：成功自动开
 *  结果抽屉并记住该卡名（下载建议名基段）；失败错误条回显（稳定码在前） */
async function runCardRender(t: TemplateSummary): Promise<void> {
    if (renderingId.value !== null) return
    renderingId.value = t.id
    actionError.value = null
    try {
        lastRender.value = await api.renderTemplate(t.id)
        lastRenderName.value = t.name
        renderDrawerOpen.value = true
    } catch (e) {
        actionError.value = `渲染失败：${formatApiError(e)}`
    } finally {
        renderingId.value = null
    }
}

function openDelete(t: TemplateSummary): void {
    deleteTarget.value = t
    deleteError.value = null
    deleteOpen.value = true
}

function closeDelete(): void {
    deleteOpen.value = false
    deleteError.value = null
    deleteTarget.value = null
}

/** 确认删除：DELETE /templates/{id} → 204 后本地移除卡片（空表自然落空态）；
 *  失败文案留在确认框内（稳定码在前），可重试或取消 */
async function runDelete(): Promise<void> {
    if (deleting.value || !deleteTarget.value) return
    deleting.value = true
    deleteError.value = null
    const target = deleteTarget.value
    try {
        await api.deleteTemplate(target.id)
        templates.value = (templates.value ?? []).filter((t) => t.id !== target.id)
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

// ── 印张卡片纯呈现辅助（蓝晒印坊语言：序号章 / 首字水印 / 进场 stagger）──

/** 卡片序号章：模板 id 三位补零（No.007） */
function serialOf(id: number): string {
    return `No.${String(id).padStart(3, '0')}`
}

/** 印张水印首字：模板名首字符（空名回落 #） */
function glyphOf(name: string): string {
    const ch = [...name.trim()][0]
    return ch ?? '#'
}

/** 进场 stagger：前 8 张依次延迟 60ms，其后不再累加 */
function riseDelay(index: number): string {
    return `${Math.min(index, 7) * 60}ms`
}
</script>

<template>
    <main class="pc-atelier relative min-h-screen text-zinc-200">
        <div class="relative z-10 mx-auto max-w-5xl px-6 pb-16 pt-14">
            <!-- 页头：金杠 kicker + 衬线大标题 + 动作组（数据源管理为次、两个新建为主） -->
            <header class="pc-rise">
                <p class="pc-kicker mb-4">Canvas Press · 证书批量生成</p>
                <div class="flex flex-wrap items-end justify-between gap-x-8 gap-y-5">
                    <div class="min-w-0">
                        <h1 class="pc-display mb-2 text-4xl leading-tight text-zinc-50">模板列表</h1>
                        <p class="max-w-xl text-sm leading-6 text-zinc-500">
                            选择一张印张进入编辑器，或从零新建；卡片直渲染与编辑器同语义，以服务端存储态为准
                        </p>
                    </div>
                    <div class="flex flex-none flex-wrap items-center gap-2.5">
                        <RouterLink
                            to="/datasources"
                            data-datasource-manage-link
                            class="inline-flex items-center gap-2 rounded-lg border border-[#223252] px-3.5 py-2 text-sm text-zinc-400 transition-colors hover:border-sky-700 hover:text-sky-300"
                            title="数据源管理：独立数据源实体的浏览/新建/编辑（模板经引用绑定，编辑共享实体影响所有引用模板）"
                        >
                            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
                                <ellipse cx="12" cy="5" rx="8" ry="3" />
                                <path d="M4 5v14c0 1.7 3.6 3 8 3s8-1.3 8-3V5" />
                                <path d="M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3" />
                            </svg>
                            数据源管理
                        </RouterLink>
                        <!-- 两个新建入口折叠为一个触发钮：点开菜单选方式；
                             data-* 锚点跟随各自菜单项 -->
                        <div class="relative flex-none" @keydown.escape="newMenuOpen = false">
                            <button
                                type="button"
                                data-new-template-toggle
                                class="flex items-center gap-2 rounded-lg bg-gradient-to-b from-sky-400 to-sky-500 px-3.5 py-2 text-sm font-semibold text-[#06202b] shadow-[0_0_0_1px_rgba(56,189,248,0.35),0_8px_24px_rgba(56,189,248,0.18)] transition hover:brightness-110 active:translate-y-px"
                                :aria-expanded="newMenuOpen"
                                aria-haspopup="menu"
                                title="新建模板：选择空白 A4 画布，或以图片为底图"
                                @click="newMenuOpen = !newMenuOpen"
                            >
                                ＋ 新建模板
                                <svg
                                    class="transition-transform"
                                    :class="newMenuOpen ? 'rotate-180' : ''"
                                    width="12"
                                    height="12"
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2.4"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                    aria-hidden="true"
                                >
                                    <path d="M6 9l6 6 6-6" />
                                </svg>
                            </button>
                            <!-- 点外收起：透明截断层垫在菜单之下 -->
                            <span
                                v-if="newMenuOpen"
                                class="fixed inset-0 z-10 cursor-default"
                                aria-hidden="true"
                                @click="newMenuOpen = false"
                            ></span>
                            <div
                                v-if="newMenuOpen"
                                role="menu"
                                aria-label="新建模板方式"
                                class="pc-menu absolute right-0 top-full z-20 mt-2 w-[300px]"
                            >
                                <button
                                    type="button"
                                    role="menuitem"
                                    data-new-blank-template
                                    class="pc-menu__item"
                                    title="新建空白模板：单帧空白 A4 竖版画布（794×1123），成功后进入编辑器"
                                    @click="pickNewBlank"
                                >
                                    <span class="pc-menu__icon" aria-hidden="true">
                                        <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
                                            <path d="M5 3h9l5 5v13H5z" />
                                            <path d="M14 3v5h5" />
                                            <path d="M12 11v6M9 14h6" />
                                        </svg>
                                    </span>
                                    <span class="min-w-0">
                                        <span class="pc-menu__title">新建空白模板</span>
                                        <span class="pc-menu__desc">单帧空白 A4 竖版画布（794×1123），进入编辑器从零排版</span>
                                    </span>
                                </button>
                                <button
                                    type="button"
                                    role="menuitem"
                                    data-new-image-template
                                    class="pc-menu__item"
                                    title="以图片新建模板：画布宽高取所选图片的像素宽高，图片作为「底图」图层铺满画布，成功后进入编辑器"
                                    @click="pickNewImage"
                                >
                                    <span class="pc-menu__icon" aria-hidden="true">
                                        <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
                                            <rect x="3" y="4" width="18" height="16" rx="2" />
                                            <circle cx="9" cy="10" r="1.6" />
                                            <path d="M21 16l-5-5-9 9" />
                                        </svg>
                                    </span>
                                    <span class="min-w-0">
                                        <span class="pc-menu__title">以图片新建</span>
                                        <span class="pc-menu__desc">画布宽高取所选图片像素宽高，图片铺满作「底图」</span>
                                    </span>
                                </button>
                            </div>
                        </div>
                    </div>
                </div>
                <div class="mt-8 h-px bg-gradient-to-r from-sky-400/50 via-sky-400/10 to-transparent" aria-hidden="true"></div>
            </header>

            <!-- 卡片操作失败态（渲染失败等非弹窗通道；稳定码在前） -->
            <div
                v-if="actionError"
                data-action-error
                class="pc-rise mt-6 flex items-start gap-2.5 rounded-xl border border-red-900/60 bg-red-950/40 px-4 py-3 text-sm leading-6 text-red-300"
            >
                <span class="mt-0.5 font-mono text-xs text-red-400/90" aria-hidden="true">✕</span>
                <span>{{ actionError }}</span>
            </div>

            <!-- 读取失败态（服务端未起等） -->
            <div
                v-if="errorText"
                class="pc-rise mt-6 flex items-start gap-2.5 rounded-xl border border-red-900/60 bg-red-950/40 px-4 py-3 text-sm leading-6 text-red-300"
            >
                <span class="mt-0.5 font-mono text-xs text-red-400/90" aria-hidden="true">✕</span>
                <span>模板列表读取失败：{{ errorText }}</span>
            </div>

            <!-- 空态（合法初始态：可从零新建；seed 未生效时也在此提示） -->
            <div
                v-else-if="templates && templates.length === 0"
                class="pc-rise mt-6 rounded-xl border border-dashed border-[#24345a] bg-[#0a1322]/60 px-6 py-16 text-center"
            >
                <svg class="mx-auto mb-5 opacity-70" width="44" height="54" viewBox="0 0 24 30" fill="none" stroke="rgba(125,211,252,0.55)" stroke-width="1.1" aria-hidden="true">
                    <path d="M3 1h12l6 6v22H3z" />
                    <path d="M15 1v6h6" />
                    <path d="M7 14h10M7 18h10M7 22h6" stroke-opacity="0.55" />
                </svg>
                <h2 class="pc-display mb-2 text-lg text-zinc-300">印架尚空</h2>
                <p class="mx-auto max-w-md text-sm leading-6 text-zinc-500">
                    暂无模板——点击右上角「＋ 新建模板」，选空白 A4 或以图片新建开始；若列表本应有 seed 模板，请确认服务端启动播种已执行。
                </p>
            </div>

            <!-- 目录卡片（印张构图）：预览位 = A4 纸张从抽屉探出 + 四角裁切标记 +
                 序号章；铭牌 = 名称 + mono 更新时间；底部操作条（渲染 / 删除） -->
            <ul v-else-if="templates" class="mt-8 grid gap-5 sm:grid-cols-2">
                <li
                    v-for="(t, i) in templates"
                    :key="t.id"
                    class="pc-card pc-rise overflow-hidden rounded-xl"
                    :style="{ animationDelay: riseDelay(i) }"
                >
                    <RouterLink :to="`/editor/${t.id}`" class="block outline-none focus-visible:ring-2 focus-visible:ring-sky-500/70" :aria-label="`打开模板：${t.name}`">
                        <div class="pc-sheet" aria-hidden="true">
                            <i class="pc-crop pc-crop--tl"></i>
                            <i class="pc-crop pc-crop--tr"></i>
                            <i class="pc-crop pc-crop--bl"></i>
                            <i class="pc-crop pc-crop--br"></i>
                            <div class="pc-paper">
                                <span class="pc-paper__glyph">{{ glyphOf(t.name) }}</span>
                                <span class="pc-paper__rule pc-paper__rule--a"></span>
                                <span class="pc-paper__rule pc-paper__rule--b"></span>
                                <span class="pc-paper__seal"></span>
                            </div>
                            <span class="pc-stamp">{{ serialOf(t.id) }}</span>
                        </div>
                    </RouterLink>
                    <div class="border-t border-[#1b2740] px-4 pb-3 pt-3.5">
                        <RouterLink :to="`/editor/${t.id}`" class="group block">
                            <div class="truncate text-[15px] font-medium text-zinc-100 transition-colors group-hover:text-sky-300">{{ t.name }}</div>
                            <div class="mt-0.5 font-mono text-[11px] text-zinc-500">
                                更新于 {{ formatUpdatedAt(t.updatedAt) }}
                            </div>
                        </RouterLink>
                        <div class="mt-3 flex items-center justify-end gap-2">
                            <button
                                type="button"
                                data-card-render
                                class="rounded-md border border-sky-800/70 px-2.5 py-1 text-xs text-sky-300 transition-colors hover:border-sky-500 hover:bg-sky-950/40 disabled:cursor-not-allowed disabled:opacity-50"
                                :disabled="renderingId !== null"
                                :title="renderingId === t.id ? '渲染中…（以服务端存储态为准）' : '渲染终图：以服务端存储态为准，成功后打开结果抽屉'"
                                @click="runCardRender(t)"
                            >
                                {{ renderingId === t.id ? '渲染中…' : '渲染' }}
                            </button>
                            <button
                                type="button"
                                data-card-delete
                                class="rounded-md border border-red-900/60 px-2.5 py-1 text-xs text-red-300 transition-colors hover:border-red-600 hover:bg-red-950/40"
                                title="删除模板：级联清渲染记录，不可恢复（已落盘的渲染图片保留）"
                                @click="openDelete(t)"
                            >
                                删除
                            </button>
                        </div>
                    </div>
                </li>
            </ul>

            <!-- 载入骨架（读取在途）：两张印张卡占位呼吸 -->
            <div v-else class="mt-8 grid gap-5 sm:grid-cols-2" aria-busy="true">
                <div v-for="n in 2" :key="n" class="pc-card overflow-hidden rounded-xl">
                    <div class="pc-sheet animate-pulse opacity-50"></div>
                    <div class="border-t border-[#1b2740] px-4 py-3.5">
                        <div class="h-3.5 w-1/3 rounded bg-zinc-800/80"></div>
                        <div class="mt-2 h-2.5 w-1/4 rounded bg-zinc-800/60"></div>
                    </div>
                </div>
            </div>

            <!-- 页脚铭线 -->
            <footer class="mt-16 border-t border-[#16233c] pt-5">
                <p class="font-mono text-[11px] tracking-[0.25em] text-zinc-600">CANVAS NEXT · 证书批量生成示例</p>
            </footer>

            <!-- 新建空白模板弹窗（spec §4.1，22 票）：名字草稿/错误/在途态本页持有 -->
            <NamePromptDialog
                v-model:name="newName"
                :open="newOpen"
                :saving="newCreating"
                :error="newError"
                title="新建空白模板"
                label="模板名"
                note="创建含单帧空白 A4 竖版画布（794×1123）的新模板，成功后进入编辑器；数据源与流链可在编辑器内绑定。取消不建。"
                confirm-text="创建"
                confirm-busy-text="创建中…"
                @confirm="runCreateBlank"
                @cancel="closeNewTemplate"
            />

            <!-- 以图片新建弹窗（spec §4.1，24 票）：名字/已选底图/错误/在途态本页持有；
                 确认 = 先上传后建模板（画布宽高 = 图片像素宽高） -->
            <ImageTemplateDialog
                v-model:name="imageName"
                :open="imageOpen"
                :saving="imageCreating"
                :error="imageError"
                :image-name="pendingImageName"
                :image-info="pendingImageInfo"
                title="以图片新建模板"
                label="模板名"
                note="创建单帧模板：画布宽高取所选图片的像素宽高，图片作为「底图」图层铺满画布；确认后上传图片并创建模板，成功后进入编辑器。未选图不建，取消不建。"
                confirm-text="创建"
                confirm-busy-text="上传并创建中…"
                @select="onImageSelected"
                @confirm="runCreateImage"
                @cancel="closeImageTemplate"
            />

            <!-- 删除确认框（spec §2.4 #10）：错误留在框内可重试，开启聚焦取消钮 -->
            <ConfirmDialog
                :open="deleteOpen"
                :busy="deleting"
                :error="deleteError"
                danger
                title="删除模板"
                :note="`删除「${deleteTarget?.name ?? ''}」不可恢复：模板与其渲染记录一并删除（已落盘的渲染图片保留，直链仍可打开）。绑定的数据源不受影响。`"
                confirm-text="删除"
                confirm-busy-text="删除中…"
                @confirm="runDelete"
                @cancel="closeDelete"
            />

            <!-- 渲染结果抽屉（spec §4.6 同款；列表页无工具栏，top 贴顶） -->
            <RenderResultDrawer
                :open="renderDrawerOpen"
                :record="lastRender"
                :template-name="lastRenderName"
                top="24px"
                @close="renderDrawerOpen = false"
            />
        </div>
    </main>
</template>

<style scoped>
/* 新建入口折叠菜单：与弹窗同语言——金→青 hairline 顶线 + pop 进场，
   原点在触发钮方向（右上），展开像从钮后抽出 */
.pc-menu {
    overflow: hidden;
    border: 1px solid #1e2a40;
    border-radius: 12px;
    background: linear-gradient(180deg, #101a2e 0%, #0c1526 100%);
    box-shadow:
        inset 0 1px 0 rgba(148, 197, 255, 0.06),
        0 18px 44px rgba(2, 6, 23, 0.6);
    transform-origin: top right;
    animation: pc-menu-pop 0.24s cubic-bezier(0.2, 0.7, 0.3, 1) backwards;
}

.pc-menu::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 2px;
    background: linear-gradient(90deg, rgba(227, 195, 127, 0.85), rgba(56, 189, 248, 0.55) 45%, transparent);
}

@keyframes pc-menu-pop {
    from {
        opacity: 0;
        transform: translateY(-6px) scale(0.97);
    }
}

.pc-menu__item {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    width: 100%;
    padding: 12px 14px;
    border: 0;
    background: transparent;
    text-align: left;
    cursor: pointer;
    transition: background 0.18s;
}

.pc-menu__item + .pc-menu__item {
    border-top: 1px solid #16233c;
}

.pc-menu__item:hover,
.pc-menu__item:focus-visible {
    background: rgba(56, 189, 248, 0.07);
    outline: none;
}

.pc-menu__icon {
    display: grid;
    flex: none;
    place-items: center;
    width: 34px;
    height: 34px;
    border: 1px solid #223252;
    border-radius: 8px;
    background: #0a1322;
    color: #7dd3fc;
}

.pc-menu__title {
    display: block;
    font-size: 13px;
    font-weight: 600;
    line-height: 1.4;
    color: #e6edf7;
}

.pc-menu__desc {
    display: block;
    margin-top: 2px;
    font-size: 11px;
    line-height: 1.5;
    color: #7c8ca5;
}

@media (prefers-reduced-motion: reduce) {
    .pc-menu {
        animation: none;
    }
}

/* 印张预览位：蓝图细网格 + 顶部微辉，A4 纸张自底部探出（下缘被裁）——
   hover 时纸张再探出一点，像从抽屉里抽出印张 */
.pc-sheet {
    position: relative;
    aspect-ratio: 16 / 9;
    overflow: hidden;
    container-type: inline-size;
    background:
        radial-gradient(140% 100% at 50% 0%, rgba(56, 189, 248, 0.08), transparent 55%),
        repeating-linear-gradient(0deg, rgba(125, 211, 252, 0.04) 0 1px, transparent 1px 22px),
        repeating-linear-gradient(90deg, rgba(125, 211, 252, 0.04) 0 1px, transparent 1px 22px),
        #0a1220;
}

/* 四角裁切标记：印厂的 registration marks 词汇 */
.pc-crop {
    position: absolute;
    width: 12px;
    height: 12px;
}

.pc-crop--tl {
    top: 9px;
    left: 9px;
    border-top: 1px solid rgba(125, 211, 252, 0.45);
    border-left: 1px solid rgba(125, 211, 252, 0.45);
}

.pc-crop--tr {
    top: 9px;
    right: 9px;
    border-top: 1px solid rgba(125, 211, 252, 0.45);
    border-right: 1px solid rgba(125, 211, 252, 0.45);
}

.pc-crop--bl {
    bottom: 9px;
    left: 9px;
    border-bottom: 1px solid rgba(125, 211, 252, 0.45);
    border-left: 1px solid rgba(125, 211, 252, 0.45);
}

.pc-crop--br {
    bottom: 9px;
    right: 9px;
    border-bottom: 1px solid rgba(125, 211, 252, 0.45);
    border-right: 1px solid rgba(125, 211, 252, 0.45);
}

/* A4 印张：暖白纸面 + 首字水印 + 铭文细线 + 烫金钤印 */
.pc-paper {
    container-type: inline-size;
    position: absolute;
    bottom: -14%;
    left: 50%;
    width: 34%;
    aspect-ratio: 210 / 297;
    transform: translateX(-50%);
    border-radius: 2px;
    background: linear-gradient(180deg, #f2f4f9 0%, #dfe6f1 86%, #cbd6e6 100%);
    box-shadow:
        0 18px 40px rgba(2, 6, 23, 0.55),
        0 0 0 1px rgba(148, 197, 255, 0.16);
    transition: transform 0.35s cubic-bezier(0.2, 0.7, 0.3, 1);
}

.pc-card:hover .pc-paper {
    transform: translateX(-50%) translateY(-4%);
}

.pc-paper__glyph {
    position: absolute;
    top: 7%;
    left: 0;
    right: 0;
    font-family: var(--pc-font-display);
    font-weight: 900;
    font-size: 56px;
    font-size: 36cqw;
    line-height: 1;
    text-align: center;
    color: rgba(11, 18, 32, 0.82);
    user-select: none;
}

.pc-paper__rule {
    position: absolute;
    left: 18%;
    height: 2px;
    border-radius: 1px;
    background: rgba(11, 18, 32, 0.16);
}

.pc-paper__rule--a {
    top: 46%;
    right: 18%;
}

.pc-paper__rule--b {
    top: 54%;
    right: 30%;
}

.pc-paper__seal {
    position: absolute;
    top: 62%;
    left: 50%;
    width: 17cqw;
    aspect-ratio: 1;
    transform: translateX(-50%);
    border-radius: 50%;
    background: radial-gradient(circle at 35% 30%, #f0d9a5, #d9b26a 68%, #bd9752);
    box-shadow:
        0 0 0 2px rgba(189, 151, 82, 0.35),
        0 2px 6px rgba(2, 6, 23, 0.25);
    opacity: 0.92;
}

/* 序号章：金框 mono（印张编号） */
.pc-stamp {
    position: absolute;
    top: 10px;
    right: 12px;
    padding: 3px 8px;
    border: 1px solid rgba(227, 195, 127, 0.45);
    border-radius: 4px;
    background: rgba(7, 13, 24, 0.55);
    color: var(--pc-gold);
    font-family: var(--pc-font-mono);
    font-size: 10px;
    letter-spacing: 0.18em;
}
</style>
