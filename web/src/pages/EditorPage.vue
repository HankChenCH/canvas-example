<script setup lang="ts">
// 编辑器页（spec §4.2 / 15 票）：占位页换真编辑器——工具栏全量搬 playground +
// 语义替换（上传图片接 POST /api/assets、「保存」改 PUT 服务端语义、「打开」钮
// 去掉、预览导出文案标注「预览图」）。接线序照 playground onReady 收敛全套装配：
//   new EditorSession({scheduleFrame: createRafScheduler(), fitMargin: 48, uploadHandler})
//   → setDataSourceSchema(datasetSchema)（null 跳过）
//   → CanvasSurface @ready 内 Canvas2DBackend（基类，不搬 gridBackdrop）+ Materializer
//     + attachContentBackend + setOverlayPainter + materializer 双订阅
//     （doc 变更 → materialize；物化状态变更 → invalidate both）
//   → decodeGraph 第 0 帧 → openDocument → fitToSurface
// 保存面（spec §4.4，本票单帧口径，16 票升级文档级）：纯手动——按钮 + Ctrl/Cmd+S，
// 无防抖自动保存；保存 = 全量 PUT（name + 当前帧 encodeGraph 替换第 0 帧 + 未载入帧
// 原样透传 + flowChain 原样透传）；dirty 跟当前帧 ∪ 模板名；dirty 基线 = 载入时文档
// 快照；路由离开 onBeforeRouteLeave confirm + 页签关闭 beforeunload 双保险；保存
// 成功归 clean。
// 红线（01/07 票）：editor-vue 组件全 named 导入、不用 runtime template 字符串、
// editor.store 非响应式（动态读数走 subscribe + shallowRef，禁深度 reactive）。
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { onBeforeRouteLeave, useRoute } from 'vue-router'

import {
    Canvas2DBackend,
    Materializer,
    applyViewportTransform,
    drawResourceMarkers,
    exportPreviewPng,
} from '@hankchen/canvas-next-browser-renderer'
import type { ResourceState } from '@hankchen/canvas-next-browser-renderer'
import {
    ARM_CREATE_LAYER_TYPES,
    EditorSession,
    isLockedPath,
    isRootLayerPath,
    rootLayerOf,
} from '@hankchen/canvas-next-editor'
import type {
    EditorShortcutAction,
    LayerType,
    OverlayPainter,
    UploadFile,
} from '@hankchen/canvas-next-editor'
import {
    ADD_LAYER_MENU,
    ARM_LAYER_CREATE_HINT,
    DropdownMenu,
    HelpDialog,
    StatusBar,
    detectShortcutPlatform,
    formatLayerPath,
    shortcutActionLabel,
    uploadFileFromDom,
    useHistory,
    useSelection,
    useShortcuts,
    useTransientFeedback,
    type CanvasSurfaceReady,
    type DropdownMenuEntry,
} from '@hankchen/canvas-next-editor-vue'
import {
    AlignFloatBar,
    CanvasSurface,
    GuidesOverlay,
    LayerPanel,
    PropertyPanel,
    Ruler,
    createRafScheduler,
    drawCreateRubberBand,
    drawFindMatches,
    drawSelectionGizmo,
} from '@hankchen/canvas-next-editor-vue'
import { decodeGraph, encodeGraph } from '@hankchen/canvas-next'

import { api, ApiError, formatApiError, type TemplateRecord } from '../api'

// ---- 模板载入（spec §4.1）：GET /templates/{id}；404 template_not_found → 错误
// 提示 + 返回列表链接（按 code 判定，不按 HTTP status） ----

const route = useRoute()
const template = ref<TemplateRecord | null>(null)
const notFound = ref(false)
const errorText = ref<string | null>(null)

onMounted(async () => {
    try {
        const record = await api.getTemplate(String(route.params.id))
        if (record.canvases.length === 0) {
            errorText.value = '模板不含任何帧，无法编辑'
            return
        }
        // 会话建立即注入数据源 schema（spec §4.2 接线序；null/缺省跳过 = 无候选）。
        // 声明只进编辑器会话态，不进 graph、不动 wire；注入失败内核静默降级 +
        // console.warn 已内建，宿主不重复处理。
        if (record.datasetSchema != null) editor.setDataSourceSchema(record.datasetSchema)
        template.value = record
        templateName.value = record.name
    } catch (e) {
        if (e instanceof ApiError && e.code === 'template_not_found') {
            notFound.value = true
        } else {
            errorText.value = formatApiError(e)
        }
    }
})

// ---- 会话（spec §4.2 接线序第 1 步）：fitMargin 48 与 playground 同款；上传注入
// 点接 POST /api/assets（响应 url 前导斜杠形态，原样写入 graph spec.src）。
// 字体清单：demo 不做字体上传，FONT_PICKER_KEY 不 provide（spec §4.2）；
// 文本度量不注入（票面构造参数口径），预览断行为参考、终图以服务端为准。 ----

/** 上传注入点：本机字节 → 服务端资源 URL（multipart 字段 file，spec §2.4 #8） */
async function uploadAssetToServer(file: UploadFile): Promise<string> {
    const { url } = await api.uploadAsset(file)
    return url
}

const editor = new EditorSession({
    scheduleFrame: createRafScheduler(),
    fitMargin: 48,
    uploadHandler: uploadAssetToServer,
})

const { canUndo, canRedo } = useHistory(editor)
// 快捷键注册表（撤销/重做/复制/粘贴/画拉建层/查找/标尺…）由内核统一接键盘；
// Ctrl/Cmd+S 保存是宿主职责，在下方 onKeydown 自理。
useShortcuts(editor)

// ---- 工具栏三下拉（全量搬 playground editor-top-toolbar 形态） ----

/** 键位后缀按宿主平台渲染（⌘] 或 Ctrl+]）：直查注册表，文案不另抄键位 */
const shortcutPlatform = detectShortcutPlatform()

/** 层型 → 武装快捷键动作（内核 ARM_CREATE_LAYER_TYPES 反查视图） */
const ARM_CREATE_ACTION_BY_TYPE = Object.fromEntries(
    Object.entries(ARM_CREATE_LAYER_TYPES).map(([action, type]) => [type, action]),
) as Record<LayerType, EditorShortcutAction>

/** 插入▾：面板＋ ADD_LAYER_MENU 同源四项，点击 = 武装画拉 + 状态栏瞬时提示 */
const insertItems = computed<DropdownMenuEntry[]>(() =>
    ADD_LAYER_MENU.map((entry) => ({
        label: entry.label,
        title: entry.title,
        shortcut: shortcutActionLabel(ARM_CREATE_ACTION_BY_TYPE[entry.type], shortcutPlatform),
        run: () => {
            editor.armLayerCreate(entry.type)
            useTransientFeedback().show(ARM_LAYER_CREATE_HINT)
        },
    })),
)

/** 选中路径（响应式桥）：排列▾ 灰态判定与动作入参共用 */
const selection = useSelection(editor)

/** 排列▾ 灰态：无选中或选中非根层 → 全组置灰 + title（内核空转守卫是正确性防线） */
const ARRANGE_GRAY_TITLE = '仅根图层可排列'
const canArrange = computed(() => selection.value !== null && isRootLayerPath(selection.value))

/** 排列▾/视图▾ 动态读数桥：editor.store 非响应式（禁深度 reactive 红线），按
 *  分支订阅同步 shallowRef——动态标签（锁定↔解锁、显示↔隐藏）与标尺勾选态
 *  不得滞留旧值 */
const docSnapshot = shallowRef(editor.store.doc)
const lockedPaths = shallowRef(editor.store.ui.lockedPaths)
const rulersVisible = shallowRef(editor.store.ui.rulersVisible)
const unsubscribeToolbarReads = editor.subscribe((change) => {
    if (change.scope === 'doc') {
        docSnapshot.value = editor.store.doc
        return
    }
    if (change.scope !== 'ui') return
    if (change.branch === 'lockedPaths') lockedPaths.value = editor.store.ui.lockedPaths
    if (change.branch === 'rulersVisible') rulersVisible.value = editor.store.ui.rulersVisible
})

/** 选中根层的可见/锁定态（非根/无选择按可见/未锁兜底） */
const selectedRootVisible = computed(() => {
    const path = selection.value
    const doc = docSnapshot.value
    if (!path || !doc || !isRootLayerPath(path)) return true
    return rootLayerOf(doc, path)?.visible ?? true
})
const selectedRootLocked = computed(() => {
    const path = selection.value
    return path !== null && isLockedPath(path, lockedPaths.value)
})

/** 排列▾：z 序四件 + 锁定/显隐，动作与右键菜单同缝（与 playground 同款） */
const arrangeItems = computed<DropdownMenuEntry[]>(() => {
    const gray = !canArrange.value
    const title = gray ? ARRANGE_GRAY_TITLE : undefined
    const path = selection.value
    return [
        {
            label: '前移一层',
            shortcut: shortcutActionLabel('bringForward', shortcutPlatform),
            disabled: gray,
            title,
            run: () => editor.bringForward(),
        },
        {
            label: '后移一层',
            shortcut: shortcutActionLabel('sendBackward', shortcutPlatform),
            disabled: gray,
            title,
            run: () => editor.sendBackward(),
        },
        {
            label: '置顶',
            shortcut: shortcutActionLabel('bringToFront', shortcutPlatform),
            disabled: gray,
            title,
            run: () => editor.bringToFront(),
        },
        {
            label: '置底',
            shortcut: shortcutActionLabel('sendToBack', shortcutPlatform),
            disabled: gray,
            title,
            run: () => editor.sendToBack(),
        },
        'separator',
        {
            label: selectedRootLocked.value ? '解锁' : '锁定',
            shortcut: shortcutActionLabel('toggleLayerLock', shortcutPlatform),
            disabled: gray,
            title,
            run: () => {
                if (path) editor.toggleLayerLock(path)
            },
        },
        {
            label: selectedRootVisible.value ? '隐藏' : '显示',
            shortcut: shortcutActionLabel('toggleLayerVisibility', shortcutPlatform),
            disabled: gray,
            title,
            run: () => {
                if (path) editor.toggleLayerVisibility(path)
            },
        },
    ]
})

/** 视图▾：缩放三件键位镜像 + 标尺勾选 + 清空参考线（与 playground 同款） */
const viewItems = computed<DropdownMenuEntry[]>(() => [
    {
        label: '100%',
        shortcut: shortcutActionLabel('zoomReset', shortcutPlatform),
        run: () => editor.resetZoom(),
    },
    {
        label: '适应画布',
        shortcut: shortcutActionLabel('fitToSurface', shortcutPlatform),
        run: () => editor.fitToSurface(),
    },
    {
        label: '适应选区',
        shortcut: shortcutActionLabel('fitToSelection', shortcutPlatform),
        run: () => editor.fitToSelection(),
    },
    'separator',
    {
        label: rulersVisible.value ? '✓ 标尺' : '标尺',
        shortcut: shortcutActionLabel('toggleRulers', shortcutPlatform),
        title: '标尺显隐（⇧R 同效）',
        run: () => editor.toggleRulers(),
    },
    {
        label: '清空参考线',
        title: '清空全部参考线（会话级，不可撤销；重建成本低——从标尺拖出即可）',
        run: () => editor.clearGuides(),
    },
])

// 参考线层实例：承接 Ruler 拖出参考线四事件的转发目标
const guidesOverlayRef = ref<InstanceType<typeof GuidesOverlay> | null>(null)

const assetsNote = ref('资源物化中…')
/** 文档操作读数（保存/上传/导出的状态与语义标注；状态栏反馈段显示） */
const docNote = ref('')
/** 物化在途计数（状态栏「物化中」段） */
const pendingCount = ref(0)

// ---- 画布装配（spec §4.2 接线序第 3 步，照 playground onReady 收敛） ----

let materializer: Materializer | null = null
let contentBackend: Canvas2DBackend | null = null
let overlayCtx: CanvasRenderingContext2D | null = null
let unsubscribeAssets: (() => void) | null = null
let unsubscribeDoc: (() => void) | null = null

/** 覆盖层画笔：资源状态标识 + 查找命中轮廓 + 选区 gizmo + 画拉橡皮筋（playground
 *  组合序原样——组合项与工具栏「查找」「＋插入」的视觉语义一一对应；gizmo 组合
 *  的封装形态 createGizmoOverlayPainter 只画选区，此处取 playground 全组合）。
 *  与内容层同一呈现变换（场景坐标，经共享的 applyViewportTransform 施加）。 */
const overlayPainter: OverlayPainter = (args) => {
    const ctx = overlayCtx
    if (!ctx) return
    ctx.setTransform(1, 0, 0, 1, 0, 0)
    ctx.clearRect(0, 0, ctx.canvas.width, ctx.canvas.height)
    if (!args.doc) return
    applyViewportTransform(ctx, { dpr: args.dpr, zoom: args.viewport.zoom, x: args.viewport.x, y: args.viewport.y })
    drawResourceMarkers(ctx, args.doc, (materializer?.state ?? {}) as ResourceState)
    drawFindMatches(ctx, editor, args)
    drawSelectionGizmo(ctx, editor, args)
    drawCreateRubberBand(ctx, editor, args)
}

function assetsStatus(state: ResourceState, pending: number): string {
    const failed = Object.values(state).filter((e) => e.status === 'failed')
    if (pending > 0) return `资源物化中（在途 ${pending}）…`
    if (failed.length > 0) return `部分资源物化失败（占位 + 红叉标识）：${failed.length} 项`
    return '资源就绪，已渲染'
}

function onReady({ contentCanvas, overlayCanvas }: CanvasSurfaceReady) {
    const contentCtx = contentCanvas.getContext('2d')
    overlayCtx = overlayCanvas.getContext('2d')
    if (!contentCtx || !overlayCtx) return

    const backend = new Canvas2DBackend(contentCtx)
    contentBackend = backend
    // 本页资源全部同源（/assets、/renders 走 vite proxy / nginx），无需 imageProxy
    materializer = new Materializer(backend)

    editor.attachContentBackend(backend)
    editor.setOverlayPainter(overlayPainter)

    // materializer 双订阅之一：物化状态变更 → 双层重绘（占位/红叉标识画在覆盖层，
    // 只脏内容层会留残影）
    unsubscribeAssets = materializer.subscribe(() => {
        assetsNote.value = assetsStatus(materializer!.state, materializer!.pendingCount)
        pendingCount.value = materializer!.pendingCount
        editor.invalidate('both')
    })

    // 双订阅之二：文档变更 = 内核「文档已变更」信号——驱动物化补调度 + dirty 防抖
    unsubscribeDoc = editor.subscribe((change) => {
        if (change.scope !== 'doc') return
        const doc = editor.store.doc
        if (doc) materializer?.materialize(doc)
        syncDirty()
    })

    // 载入第 0 帧（本票单帧口径，16 票升级多帧）：decodeGraph → openDocument → fit
    const record = template.value
    const first = record?.canvases[0]
    if (!record || !first) return
    const doc = decodeGraph(first.graph)
    editor.openDocument(doc)
    // dirty 基线 = 载入时文档快照（spec §4.4）：canonical encode JSON + 模板名
    savedBaseline = { name: templateName.value, frame0: JSON.stringify(encodeGraph(doc)) }
    computeDirty()
    materializer.materialize(doc)
    editor.fitToSurface() // 初始进入：整页 fit 语义
}

// ---- 保存面（spec §4.4，单帧口径）：纯手动 + dirty 双守卫 ----

/** 模板名（顶栏可编辑输入框，内容进 dirty 口径） */
const templateName = ref('')

/** 未保存点（playground 防抖同款）：encode+stringify 对大文档是 MB 级字符串重建，
 *  doc 订阅侧只在文档停变 500ms 后算一次；守卫（路由离开/页签关闭）走同步即时
 *  比较——防抖窗口内的最近编辑也要计入。 */
const isDirty = ref(false)
const DIRTY_DEBOUNCE_MS = 500
let dirtyTimer: ReturnType<typeof setTimeout> | null = null
/** dirty 基线 = 载入（或保存成功）时的文档快照：模板名 + 第 0 帧 canonical encode */
let savedBaseline: { name: string; frame0: string } | null = null

function computeDirty(): void {
    const doc = editor.store.doc
    const frameJson = doc ? JSON.stringify(encodeGraph(doc)) : null
    isDirty.value =
        savedBaseline !== null &&
        frameJson !== null &&
        (frameJson !== savedBaseline.frame0 || templateName.value !== savedBaseline.name)
}

function syncDirty(): void {
    if (dirtyTimer !== null) clearTimeout(dirtyTimer)
    dirtyTimer = setTimeout(() => {
        dirtyTimer = null
        computeDirty()
    }, DIRTY_DEBOUNCE_MS)
}

watch(templateName, () => syncDirty())

const saving = ref(false)

/** 保存按钮共享 title（顶栏/工具栏两处同文案，单点维护） */
const SAVE_TITLE = '保存到服务端（Ctrl/Cmd+S；全量 PUT：name + 当前帧 + flowChain）'

/** 保存 = 全量 PUT（spec §2.4 #5 整存替换语义）：name + 当前帧 encodeGraph 替换
 *  第 0 帧 + 未载入帧原样透传 + flowChain 原样透传。未载入帧必须透传——flowChain
 *  仍引用帧下标（seed 续页 frame:1），只发单帧会被预检 flow_chain_invalid 打回；
 *  16 票帧缓冲落地后由「当前帧 ∪ 帧缓冲」接管。 */
async function saveTemplate(): Promise<void> {
    const record = template.value
    const doc = editor.store.doc
    if (!record || !doc || saving.value) return
    const wire = encodeGraph(doc)
    const sentName = templateName.value
    saving.value = true
    docNote.value = '保存中…'
    try {
        const updated = await api.updateTemplate(record.id, {
            name: sentName,
            canvases: record.canvases.map((c, i) => (i === 0 ? { name: c.name, graph: wire } : c)),
            flowChain: record.flowChain,
        })
        template.value = updated
        // 基线 = 本次发送的字节（canonical encode），保存期间的继续编辑保持 dirty
        savedBaseline = { name: updated.name, frame0: JSON.stringify(wire) }
        computeDirty()
        docNote.value = '已保存到服务端'
    } catch (e) {
        docNote.value = `保存失败：${formatApiError(e)}`
    } finally {
        saving.value = false
    }
}

/** 输入法合成中的按键不触发保存 */
function isImeComposing(event: KeyboardEvent): boolean {
    return event.isComposing || event.keyCode === 229
}

/** 键盘：仅 Ctrl/Cmd+S（保存是宿主职责，不入内核注册表）。文本编辑中键盘路由
 *  进 textarea，保存不抢；其余快捷键已由 useShortcuts 统一处理。 */
function onKeydown(event: KeyboardEvent): void {
    if (editor.store.ui.editing !== null) return
    const mod = event.ctrlKey || event.metaKey
    if (mod && !event.shiftKey && event.key.toLowerCase() === 's') {
        if (isImeComposing(event)) return
        event.preventDefault()
        void saveTemplate()
    }
}
window.addEventListener('keydown', onKeydown)

/** 关闭守卫双保险之一：路由离开 confirm（spec §4.4）——同步即时比较 */
onBeforeRouteLeave(() => {
    computeDirty()
    if (!isDirty.value) return true
    return window.confirm('当前有未保存的变更，离开将丢失。确定离开吗？')
})

/** 双保险之二：页签关闭 beforeunload（文案由浏览器定） */
function onBeforeUnload(event: BeforeUnloadEvent): void {
    computeDirty()
    if (!isDirty.value) return
    event.preventDefault()
    event.returnValue = ''
}
window.addEventListener('beforeunload', onBeforeUnload)

// ---- 上传图片（spec §4.2 语义替换）：本机选图 → POST /api/assets → 前导斜杠
// url 新建图片图层 ----

const imageInput = ref<HTMLInputElement | null>(null)
function onUploadImageClick(): void {
    imageInput.value?.click()
}

async function onImageFile(event: Event): Promise<void> {
    const inputEl = event.target as HTMLInputElement
    const file = inputEl.files?.[0]
    inputEl.value = ''
    if (!file) return
    try {
        const path = await editor.uploadImageAsLayer(await uploadFileFromDom(file))
        docNote.value = path
            ? `已上传并新建图片图层：${formatLayerPath(path)} · ${file.name}`
            : '上传完成但文档未打开'
    } catch (error) {
        docNote.value = `上传失败：${formatApiError(error)}`
    }
}

// ---- 预览导出（spec §4.6，ADR 0004）：浏览器 PNG 是预览图，非终图 ----

const exporting = ref(false)

function downloadBlob(blob: Blob, filename: string): void {
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = filename
    anchor.click()
    URL.revokeObjectURL(url)
}

function previewFileName(): string {
    const base = templateName.value.replace(/[/\\:*?"<>|]/g, '_').trim()
    return `${base || 'template'}-preview.png`
}

async function exportPreview(): Promise<void> {
    const doc = editor.store.doc
    // 导出消费预览视图：模板态表格以一行预览行出图，与画布所见一致
    const view = editor.previewCanvas
    if (!doc || !view || !contentBackend || !materializer || exporting.value) return
    exporting.value = true
    docNote.value = '导出预览：等待全部资源物化…'
    try {
        materializer.materialize(doc) // 兜底补调度（面板改 URL 后的增量引用）
        const state = await materializer.whenSettled()
        const failed = Object.values(state).filter((e) => e.status === 'failed').length
        // 文本布局策略与编辑会话同一注入值：导出与画布的断行/盒高不分叉
        const result = await exportPreviewPng(view, contentBackend, { textPolicies: editor.textPolicies })
        downloadBlob(result.blob, previewFileName())
        docNote.value =
            failed > 0
                ? `已导出预览 PNG（预览图，非终图；${failed} 项资源失败以占位出图）· ${result.width}×${result.height}`
                : `已导出预览 PNG（预览图，非终图）· ${result.width}×${result.height}`
    } catch (error) {
        docNote.value = `导出失败：${formatApiError(error)}`
    } finally {
        exporting.value = false
    }
}

onBeforeUnmount(() => {
    window.removeEventListener('keydown', onKeydown)
    window.removeEventListener('beforeunload', onBeforeUnload)
    if (dirtyTimer !== null) clearTimeout(dirtyTimer)
    unsubscribeAssets?.()
    unsubscribeDoc?.()
    unsubscribeToolbarReads()
    editor.dispose()
})
</script>

<template>
    <!-- 载入中 / 404 / 读取失败（spec §4.1：404 template_not_found → 错误提示 +
         返回列表链接；编辑器台面在模板载入成功后才挂载） -->
    <main v-if="!template" class="min-h-screen bg-[#070d18] px-6 py-12 text-zinc-200">
        <RouterLink to="/" class="text-sm text-sky-400 hover:text-sky-300">← 返回列表</RouterLink>
        <div
            v-if="notFound"
            class="mt-8 rounded-lg border border-red-900/60 bg-red-950/40 px-4 py-3 text-sm text-red-300"
        >
            模板不存在（template_not_found）。
            <RouterLink to="/" class="text-sky-400 hover:text-sky-300">返回列表</RouterLink>
        </div>
        <div
            v-else-if="errorText"
            class="mt-8 rounded-lg border border-red-900/60 bg-red-950/40 px-4 py-3 text-sm text-red-300"
        >
            模板读取失败：{{ errorText }}
        </div>
        <p v-else class="mt-8 text-sm text-zinc-500">模板载入中…</p>
    </main>

    <!-- 编辑器台面：全视口暗色工作台壳（playground 同款四行网格：顶栏 / 工具栏 /
         工作台 / 状态栏），页面零滚动 -->
    <main v-else class="stage">
        <!-- 顶栏（spec §4.1）：← 返回列表｜模板名可编辑（内容进 dirty 口径）｜
             保存态｜保存 / 导出预览图（另存为 18 票、渲染终图 19 票） -->
        <header class="topbar" aria-label="编辑器顶栏">
            <div class="topbar-doc">
                <RouterLink to="/" class="back-link" data-back-link>← 返回列表</RouterLink>
                <input
                    v-model="templateName"
                    class="name-input"
                    data-template-name
                    aria-label="模板名"
                    :readonly="saving"
                    spellcheck="false"
                />
                <span class="save-state" data-save-state :class="{ 'is-dirty': isDirty }">
                    {{ isDirty ? '● 未保存' : '○ 已保存' }}
                </span>
            </div>
            <div class="topbar-actions">
                <button
                    type="button"
                    class="primary"
                    data-save-template
                    :title="SAVE_TITLE"
                    :disabled="saving"
                    @click="saveTemplate"
                >
                    {{ saving ? '保存中…' : '保存' }}
                </button>
                <!-- editor.store.doc 是非响应式读数，按钮可用态不做文档门（处理器自守卫），
                     导出中状态走响应式 exporting -->
                <button
                    type="button"
                    class="ghost"
                    data-export-preview
                    title="导出浏览器预览 PNG（预览图，非终图；先等待全量物化）"
                    :disabled="exporting"
                    @click="exportPreview"
                >
                    {{ exporting ? '导出中…' : '导出预览图' }}
                </button>
            </div>
        </header>

        <!-- 工具栏（全量搬 playground 分组形态 + 语义替换）：保存（PUT 服务端）｜
             上传图片（POST /api/assets）｜撤销/重做｜＋插入▾｜查找｜排列▾｜视图▾。
             playground 的「打开」钮去掉（换模板走返回列表）、桌面网格开关不搬
             （后端用基类，无 gridBackdrop）。 -->
        <section class="toolbar" aria-label="编辑器工具栏">
            <button
                type="button"
                data-save
                :title="SAVE_TITLE"
                :disabled="saving"
                @click="saveTemplate"
            >
                保存
            </button>
            <button
                type="button"
                data-upload-image
                title="本机选图 → 上传到服务端（POST /api/assets）→ 新建图片图层"
                :disabled="!editor.canUpload"
                @click="onUploadImageClick"
            >
                上传图片
            </button>
            <span class="toolbar-divider" aria-hidden="true"></span>
            <button
                type="button"
                data-undo
                title="撤销（Ctrl/Cmd+Z）"
                :disabled="!canUndo"
                @click="editor.undo()"
            >
                撤销
            </button>
            <button
                type="button"
                data-redo
                title="重做（Ctrl/Cmd+Shift+Z 或 Ctrl+Y）"
                :disabled="!canRedo"
                @click="editor.redo()"
            >
                重做
            </button>
            <span class="toolbar-divider" aria-hidden="true"></span>
            <DropdownMenu
                class="tb-insert"
                data-insert-menu
                label="＋插入"
                :items="insertItems"
                title="插入图层（四类层型统一画拉：点击后在画布拖拽定落位与尺寸，Esc 取消）"
            />
            <span class="toolbar-divider" aria-hidden="true"></span>
            <button
                type="button"
                data-find
                title="查找替换（Ctrl/Cmd+F 同效；查找条开在画布顶部）"
                @click="editor.beginFind()"
            >
                查找
            </button>
            <span class="toolbar-divider" aria-hidden="true"></span>
            <DropdownMenu class="tb-arrange" data-arrange-menu label="排列" :items="arrangeItems" title="排列：z 序与锁定/显隐" />
            <span class="toolbar-divider" aria-hidden="true"></span>
            <DropdownMenu class="tb-view" data-view-menu label="视图" :items="viewItems" title="视图：缩放键位镜像 / 标尺 / 参考线" />
            <!-- 隐藏文件入口：本机选图上传 -->
            <input ref="imageInput" type="file" accept="image/*" class="hidden" @change="onImageFile" />
        </section>

        <section class="workbench" aria-label="画布与面板">
            <LayerPanel :editor="editor" />
            <!-- canvas-area：画布覆盖物（标尺/参考线/对齐浮条）的宿主级定位上下文 -->
            <div class="canvas-area">
                <CanvasSurface class="surface" :editor="editor" @ready="onReady" />
                <!-- 参考线/吸附线层：root 自带 absolute inset:0 + pointer-events:none，
                     与 .surface 同矩形直挂、浮于画布之上；挂点次序在标尺之前（拖回
                     删除的落点判定要求标尺条盖在参考线命中条之上） -->
                <GuidesOverlay ref="guidesOverlayRef" :editor="editor" />
                <!-- 标尺：画布容器顶+左贴边宿主挂载；壳 pointer-events:none 不拦画布
                     事件；内联箭头保证每次触发都取当前 ref -->
                <div class="ruler-shell">
                    <Ruler
                        :editor="editor"
                        @guide-drag-start="(g) => guidesOverlayRef?.beginGuideDrag(g)"
                        @guide-drag-move="(g) => guidesOverlayRef?.moveGuideDrag(g)"
                        @guide-drag-end="(g) => guidesOverlayRef?.endGuideDrag(g)"
                        @guide-drag-cancel="() => guidesOverlayRef?.cancelGuideDrag()"
                    />
                </div>
                <!-- 对齐浮条：画布容器顶部居中宿主级挂载，浮于 CanvasSurface 之上 -->
                <AlignFloatBar class="align-float" :editor="editor" />
            </div>
            <PropertyPanel :editor="editor" />
        </section>

        <!-- 状态栏：组件内直读缩放/选中路径/坐标尺寸/schema 声明态，资源/保存/反馈
             三段由宿主注入 -->
        <StatusBar
            class="statusbar"
            :editor="editor"
            :pending-count="pendingCount"
            :resource-note="assetsNote"
            :save-state="isDirty ? 'dirty' : 'clean'"
            :feedback="docNote"
        />

        <!-- 快捷键帮助面板：⌘/ 与状态栏「快捷键」入口随组件与桥自带，宿主零键位代码 -->
        <HelpDialog />
    </main>
</template>

<style scoped>
/* 全视口暗色工作台壳（playground 同款令牌）：100vh 四行网格——顶栏 48 / 工具栏
   44 / 工作台 1fr / 状态栏 auto（≈30），页面零滚动；小视口降级：工作台行压缩 +
   面板自内滚 + 工具栏横向内滚。editor-vue 面板/状态栏令牌零改动。 */
.stage {
    --shell-bg: #070d18;
    --shell-panel: #0b1220;
    --shell-panel-92: rgb(11 18 32 / 0.92);
    --shell-line: #1e2a40;
    --shell-line-strong: #2a3a58;
    --shell-hover: #16223a;
    --shell-fg: #e6edf7;
    --shell-fg-2: #c3cddd;
    --shell-fg-3: #aab6c8;
    --shell-muted: #7c8ca5;
    --shell-accent: #38bdf8;
    --shell-on-accent: #06202b;
    --shell-insert: #7dd3fc;
    --shell-font-mono: 'SF Mono', Menlo, Consolas, monospace;
    --shell-desktop: radial-gradient(1100px 600px at 50% 40%, #101b30 0%, #0a1120 70%);

    display: grid;
    grid-template-rows: 48px 44px minmax(0, 1fr) auto;
    height: 100vh;
    margin: 0;
    overflow: hidden;
    background: var(--shell-bg);
    font-family: system-ui, sans-serif;
    color: var(--shell-fg);
}

/* 顶栏（spec §4.1）：48px 单行——返回链接｜模板名输入框｜保存态｜保存/导出钮 */
.topbar {
    display: flex;
    align-items: center;
    gap: 14px;
    height: 48px;
    padding: 0 16px;
    background: var(--shell-panel);
    border-bottom: 1px solid var(--shell-line);
}

.topbar-doc {
    display: flex;
    flex: 1;
    align-items: center;
    gap: 12px;
    min-width: 0;
}

.back-link {
    flex: none;
    font-size: 13px;
    color: var(--shell-fg-3);
    text-decoration: none;
    white-space: nowrap;
}

.back-link:hover {
    color: var(--shell-accent);
}

/* 模板名输入框：静态时无边框似标题，悬停/聚焦现边框提示可编辑（内容进 dirty） */
.name-input {
    flex: 0 1 auto;
    min-width: 0;
    max-width: 360px;
    padding: 4px 8px;
    border: 1px solid transparent;
    border-radius: 6px;
    background: transparent;
    font-size: 14px;
    font-weight: 600;
    color: var(--shell-fg);
}

.name-input:hover {
    border-color: var(--shell-line);
}

.name-input:focus {
    border-color: var(--shell-line-strong);
    background: var(--shell-hover);
    outline: none;
}

.save-state {
    flex: none;
    font-size: 12px;
    white-space: nowrap;
    color: var(--shell-muted);
}

.save-state.is-dirty {
    color: #f59e0b;
}

.topbar-actions {
    display: flex;
    flex: none;
    align-items: center;
    gap: 8px;
}

.topbar .ghost {
    padding: 5px 12px;
    border: 1px solid var(--shell-line);
    border-radius: 8px;
    background: transparent;
    font-size: 13px;
    color: var(--shell-fg-3);
    cursor: pointer;
}

.topbar .ghost:hover:not(:disabled) {
    border-color: var(--shell-line-strong);
    color: var(--shell-fg);
}

/* 保存主按钮：保存中禁用 */
.topbar .primary {
    padding: 6px 14px;
    border: none;
    border-radius: 8px;
    background: var(--shell-accent);
    font-size: 13px;
    font-weight: 600;
    color: var(--shell-on-accent);
    cursor: pointer;
}

.topbar .primary:hover:not(:disabled) {
    filter: brightness(1.12);
}

.topbar .primary:disabled,
.topbar .ghost:disabled {
    opacity: 0.55;
    cursor: not-allowed;
}

.hidden {
    display: none;
}

/* 工具栏：44px 全宽边条，ghost 钮 + 分隔线；小视口横向内滚降级 */
.toolbar {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    padding: 0 16px;
    overflow-x: auto;
    overflow-y: hidden;
    background: var(--shell-panel);
    border-bottom: 1px solid var(--shell-line);
    scrollbar-width: none;
}

.toolbar::-webkit-scrollbar {
    display: none;
}

.toolbar button {
    flex: none;
    padding: 5px 9px;
    border: none;
    border-radius: 6px;
    background: transparent;
    font-size: 13px;
    white-space: nowrap;
    color: var(--shell-fg-2);
    cursor: pointer;
}

.toolbar button:hover:not(:disabled) {
    background: var(--shell-hover);
}

.toolbar button:disabled {
    opacity: 0.55;
    color: var(--shell-muted);
    cursor: not-allowed;
}

/* 插入组亮色引导最高频动作（playground 同款）：触发钮经 :deep 着色 */
.toolbar :deep(.tb-insert .cn-dropdown__trigger) {
    color: var(--shell-insert);
}

.toolbar-divider {
    flex: none;
    width: 1px;
    height: 18px;
    margin: 0 2px;
    background: var(--shell-line);
}

/* 工作台：1fr 行垂直撑满；min-height 0 允许小视口压缩，页面不滚 */
.workbench {
    display: flex;
    gap: 10px;
    min-width: 0;
    min-height: 0;
    padding: 0 10px;
}

/* 状态栏：全宽底边条，宿主只做几何——组件自带暗色令牌零改动 */
.statusbar {
    width: 100%;
    border-radius: 0;
    border-top: 1px solid var(--shell-line);
}

/* 画布容器 = 覆盖物（标尺/参考线/对齐浮条）的定位上下文 */
.canvas-area {
    position: relative;
    flex: 1;
    min-width: 0;
}

.workbench .surface {
    border: 1px solid var(--shell-line);
    border-radius: 12px;
    background: var(--shell-desktop); /* 画布外的「桌面」暗底 */
    overflow: hidden;
}

.surface {
    width: 100%;
    height: 100%;
    border-radius: 11px;
}

/* 标尺壳：画布容器顶+左贴边挂载——壳 pointer-events:none 不拦画布事件（条自收），
   条外缘 = 内容区缘（手势坐标换算前提）；无 z-index：流内次序即 surface 之上、
   浮条（z10）之下 */
.ruler-shell {
    position: absolute;
    inset: 0;
    overflow: hidden;
    border-radius: 12px;
    pointer-events: none;
}

/* 对齐浮条：宿主只出定位——画布容器顶部居中，浮于画布之上 */
.align-float {
    position: absolute;
    top: 14px;
    left: 50%;
    z-index: 10;
    transform: translateX(-50%);
}
</style>
