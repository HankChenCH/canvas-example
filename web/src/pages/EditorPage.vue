<script setup lang="ts">
// 编辑器页（spec §4.2 / 15 票装配，16 票升级多帧）：占位页换真编辑器——工具栏全量
// 搬 playground + 语义替换（上传图片接 POST /api/assets、「保存」改 PUT 服务端语义、
// 「打开」钮去掉、预览导出文案标注「预览图」）。接线序照 playground onReady 收敛全套装配：
//   new EditorSession({scheduleFrame: createRafScheduler(), fitMargin: 48, uploadHandler})
//   → setDataSourceSchema(datasetSchema)（null 跳过）
//   → CanvasSurface @ready 内 Canvas2DBackend（基类，不搬 gridBackdrop）+ Materializer
//     + attachContentBackend + setOverlayPainter + materializer 双订阅
//     （doc 变更 → materialize；物化状态变更 → invalidate both）
//   → decodeGraph 逐帧 → openDocument 第 0 帧 → fitToSurface
// 多帧（spec §4.2，16 票）：画布上方帧 tab 条；切帧即进宿主内存帧缓冲——切出前
// encodeGraph(editor.store.doc) 快照该帧、换帧 openDocument 重建会话文档（openDocument
// 不重置 schema，无需重复注入——01 票）；tab 双击重命名（改 canvases[i].name，进 dirty）；
// 不做增删帧；帧间切换不提示。帧缓冲纯逻辑在 src/editor/frames.ts（TDD 缝）。
// 保存面（spec §4.4，16 票文档级口径）：纯手动——按钮 + Ctrl/Cmd+S，无防抖自动保存；
// 保存 = 全量 PUT（name + 当前帧 ∪ 帧缓冲各帧 + flowChain 原样）；dirty 跟文档级——
// 当前帧 ∪ 帧缓冲任一帧 ∪ flowChain ∪ 模板名；dirty 基线 = 载入时各帧快照；路由离开
// onBeforeRouteLeave confirm + 页签关闭 beforeunload 双保险；保存成功归 clean、基线
// 同步本次发送各帧。
// 数据源抽屉（spec §4.3 双通道，17 票）：一个抽屉两条保存通道——数据源段
// （schema/data 文本域 +「保存数据源」→ PUT /templates/{id}/dataset，schema_invalid /
// dataset_schema_mismatch 段内回显，成功后 setDataSourceSchema 更新补全候选）与流链段
// （flowChain 文本域 +「保存流链」→ 文档级 PUT，canvases 一并整存，flow_chain_invalid
// 等编译码段内回显）；抽屉段内独立未保存标记（文本域 vs 载入基线）不混全局指示灯；
// dataset 不进全局 dirty 口径，flowChain 草稿进（spec §4.4）。
// 红线（01/07 票）：editor-vue 组件全 named 导入、不用 runtime template 字符串、
// editor.store 非响应式（动态读数走 subscribe + shallowRef，禁深度 reactive）。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
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
import { api, ApiError, formatApiError, type TemplateRecord } from '../api'
import {
    baselineFromSlots,
    buildSavePayload,
    canonicalFlowChain,
    decodeGraphJson,
    encodeGraphJson,
    isDocDirty,
    loadFrameSlots,
    type FrameBaseline,
    type FrameSlot,
} from '../editor/frames'
import {
    datasetDraftPayload,
    drawerBaselineFromRecord,
    isSegmentDirty,
    parseJsonDraft,
    type DrawerDraftBaseline,
} from '../editor/datasource'
import DataSourceDrawer from '../components/DataSourceDrawer.vue'

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
        // 会话建立即注入数据源 schema（spec §4.2 接线序；null/缺省 = 清除声明 =
        // 无候选，与会话初始态一致）。声明只进编辑器会话态，不进 graph、不动 wire；
        // 注入失败内核静默降级 + console.warn 已内建，宿主不重复处理。openDocument
        // 不重置 schema（01 票），切帧重建会话文档无需重复注入；数据源段保存成功后
        // 走同一入口更新（saveDataset）。
        editor.setDataSourceSchema(record.datasetSchema)
        // 帧缓冲载入（spec §4.2「decodeGraph 逐帧」）：逐帧解码即验 + canonical 化，
        // 基线与帧缓冲同形；任一帧解码失败 = 载入错误面（16 票）
        try {
            frameSlots.value = loadFrameSlots(record.canvases)
        } catch (e) {
            errorText.value = e instanceof Error ? e.message : String(e)
            return
        }
        savedBaseline = baselineFromSlots(record.name, frameSlots.value, record.flowChain)
        activeFrame.value = 0
        template.value = record
        templateName.value = record.name
        // 抽屉文本域初值 = 模板记录三字段的 pretty 文本（spec §4.3），基线同步打点
        drawerBaseline.value = drawerBaselineFromRecord(record)
        schemaText.value = drawerBaseline.value.schemaText
        dataText.value = drawerBaseline.value.dataText
        flowChainText.value = drawerBaseline.value.flowChainText
    } catch (e) {
        if (e instanceof ApiError && e.code === 'template_not_found') {
            notFound.value = true
        } else {
            errorText.value = formatApiError(e)
        }
    }
})

// ---- 多帧帧缓冲（spec §4.2，16 票）：宿主内存帧缓冲 = canvases 的宿主镜像，
// graphJson 以 canonical encode 字符串持有（dirty 比较/保存载荷直接消费字符串）；
// activeFrame 当前帧下标；不做增删帧。 ----

const frameSlots = ref<FrameSlot[]>([])
const activeFrame = ref(0)

/** 切出前快照当前帧（spec §4.2：encodeGraph(editor.store.doc) 快照该帧进缓冲） */
function snapshotActiveFrame(): void {
    const doc = editor.store.doc
    if (!doc) return
    const index = activeFrame.value
    const slot = frameSlots.value[index]
    if (!slot) return
    frameSlots.value[index] = { ...slot, graphJson: encodeGraphJson(doc) }
}

/** 切帧（spec §4.2）：快照切出帧 → decodeGraph 重建会话文档（ui 选择/撤销历史等
 *  会话态随 openDocument 重置）。视口保留不 refit；schema 不重置无需重复注入；
 *  materialize 与 dirty 重算由 doc 订阅顺带驱动；帧间切换不提示。 */
function switchToFrame(index: number): void {
    if (index === activeFrame.value || !frameSlots.value[index]) return
    snapshotActiveFrame()
    const doc = decodeGraphJson(frameSlots.value[index]!.graphJson)
    activeFrame.value = index
    editor.openDocument(doc)
}

// ---- tab 双击重命名（spec §4.2）：改 canvases[i].name（帧缓冲 name 即它）进 dirty；
// 与 graph 无关，活动帧重命名不动文档。空名拒绝提交。 ----

const renamingFrame = ref<number | null>(null)
const renamingValue = ref('')
const renameInputEl = ref<HTMLInputElement | null>(null)

function setRenameInputEl(el: unknown): void {
    renameInputEl.value = el instanceof HTMLInputElement ? el : null
}

watch(renamingFrame, async (index) => {
    if (index === null) return
    await nextTick()
    renameInputEl.value?.focus()
    renameInputEl.value?.select()
})

function beginFrameRename(index: number): void {
    renamingFrame.value = index
    renamingValue.value = frameSlots.value[index]?.name ?? ''
}

function commitFrameRename(): void {
    const index = renamingFrame.value
    renamingFrame.value = null
    if (index === null) return
    const next = renamingValue.value.trim()
    const slot = frameSlots.value[index]
    if (!slot || !next || next === slot.name) return
    frameSlots.value[index] = { ...slot, name: next }
    syncDirty()
}

function cancelFrameRename(): void {
    renamingFrame.value = null
}

// ---- 数据源抽屉（spec §4.3 双通道，17 票）：三文本域草稿 + 段基线。段内独立
// 未保存标记 = 文本域 vs 载入基线（spec §4.4，不混全局 saveState 指示灯）——
// dataset 不在全局 dirty 口径，抽屉开着改数据源全局仍 clean；flowChain 草稿在
// 全局口径内，文本变更经 syncDirty 计入。各段保存成功后基线同步本次发送文本。 ----

const drawerOpen = ref(false)
// 未绑数据源/空链的文本域初值（spec §3.1：空链与 null 同义，抽屉内以 null 字面表达）
const EMPTY_DRAFT_TEXT = 'null'
const schemaText = ref(EMPTY_DRAFT_TEXT)
const dataText = ref(EMPTY_DRAFT_TEXT)
const flowChainText = ref(EMPTY_DRAFT_TEXT)
const drawerBaseline = ref<DrawerDraftBaseline>({
    schemaText: EMPTY_DRAFT_TEXT,
    dataText: EMPTY_DRAFT_TEXT,
    flowChainText: EMPTY_DRAFT_TEXT,
})

const datasetSegmentDirty = computed(
    () =>
        isSegmentDirty(schemaText.value, drawerBaseline.value.schemaText) ||
        isSegmentDirty(dataText.value, drawerBaseline.value.dataText),
)
const flowChainSegmentDirty = computed(() => isSegmentDirty(flowChainText.value, drawerBaseline.value.flowChainText))

// 段内错误随再编辑清空（陈旧错误误导）；流链草稿文本同时计入全局 dirty 口径
const datasetError = ref<string | null>(null)
const flowChainError = ref<string | null>(null)
watch([schemaText, dataText], () => {
    datasetError.value = null
})
watch(flowChainText, () => {
    flowChainError.value = null
    syncDirty()
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

    // 载入第 0 帧（spec §4.2）：帧缓冲 graphJson → decodeGraph → openDocument → fit
    if (!template.value || frameSlots.value.length === 0) return
    const doc = decodeGraphJson(frameSlots.value[0]!.graphJson)
    editor.openDocument(doc)
    // dirty 基线已在载入时打快照（各帧 canonical 串）；此处即算一次让守卫立即可用
    computeDirty()
    materializer.materialize(doc)
    editor.fitToSurface() // 初始进入：整页 fit 语义
}

// ---- 保存面（spec §4.4，16 票文档级口径）：纯手动 + dirty 双守卫 ----

/** 模板名（顶栏可编辑输入框，内容进 dirty 口径） */
const templateName = ref('')

/** 未保存点（playground 防抖同款）：encode+stringify 对大文档是 MB 级字符串重建，
 *  doc 订阅侧只在文档停变 500ms 后算一次；守卫（路由离开/页签关闭）走同步即时
 *  比较——防抖窗口内的最近编辑也要计入。 */
const isDirty = ref(false)
const DIRTY_DEBOUNCE_MS = 500
let dirtyTimer: ReturnType<typeof setTimeout> | null = null
/** dirty 基线 = 载入（或保存成功）时的文档级快照（spec §4.4 文档级口径）：
 *  模板名 + 各帧名 + 各帧 canonical 串 + flowChain canonical 串 */
let savedBaseline: FrameBaseline | null = null

function computeDirty(): void {
    if (savedBaseline === null || template.value === null) {
        isDirty.value = false
        return
    }
    // flowChain 现值 = 抽屉流链草稿的解析值（spec §4.4 口径含 flowChain，17 票）；
    // 半成品文本（解析失败）视同已改动占住 dirty，直到修复或还原
    const flowDraft = parseJsonDraft(flowChainText.value)
    if (!flowDraft.ok) {
        isDirty.value = true
        return
    }
    const doc = editor.store.doc
    isDirty.value = isDocDirty({
        baseline: savedBaseline,
        slots: frameSlots.value,
        activeIndex: activeFrame.value,
        activeGraphJson: doc ? encodeGraphJson(doc) : null,
        templateName: templateName.value,
        flowChain: flowDraft.value,
    })
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
const SAVE_TITLE = '保存到服务端（Ctrl/Cmd+S；全量 PUT：name + 文档各帧 + flowChain）'

/** 保存 = 全量 PUT（spec §2.4 #5 整存替换语义，spec §4.4 文档级口径）：当前帧
 *  快照并入帧缓冲（与切出同一条缝）→ 载荷 = name + 帧缓冲各帧 + flowChain。
 *  flowChain 以抽屉流链草稿的解析值为准随载荷整存（17 票）——「保存流链」与
 *  顶栏保存是同一条文档级通道，区别只在错误回显面（段内 vs 状态栏），解析失败
 *  则任何文档级保存都无载荷可发，就地回显流链段并终止。基线 = 本次发送字节，
 *  保存期间的继续编辑保持 dirty。 */
async function runDocumentSave(origin: 'topbar' | 'flowchain'): Promise<void> {
    const record = template.value
    if (!record || !editor.store.doc || saving.value) return
    const flowDraft = parseJsonDraft(flowChainText.value)
    if (!flowDraft.ok) {
        flowChainError.value = `flowChain JSON 解析失败：${flowDraft.message}`
        if (origin === 'topbar') docNote.value = '保存失败：流链 JSON 解析失败（在数据源抽屉内修复或还原后再保存）'
        return
    }
    snapshotActiveFrame()
    const sentName = templateName.value
    const sentSlots = frameSlots.value.map((slot) => ({ ...slot }))
    const sentFlowChainText = flowChainText.value
    const sentFlowChainCanonical = canonicalFlowChain(flowDraft.value)
    const payload = buildSavePayload({
        slots: sentSlots,
        templateName: sentName,
        flowChain: flowDraft.value,
    })
    saving.value = true
    docNote.value = '保存中…'
    try {
        const updated = await api.updateTemplate(record.id, payload)
        template.value = updated
        // 基线 = 本次发送字节（帧名 ∪ 各帧 canonical 串 ∪ flowChain），保存期间的
        // 继续编辑保持 dirty
        savedBaseline = {
            name: sentName,
            names: sentSlots.map((slot) => slot.name),
            frames: sentSlots.map((slot) => slot.graphJson),
            flowChain: sentFlowChainCanonical,
        }
        // 流链草稿已随本次文档级 PUT 落库：段基线同步发送文本，段内标记归灭
        drawerBaseline.value = { ...drawerBaseline.value, flowChainText: sentFlowChainText }
        flowChainError.value = null
        computeDirty()
        docNote.value = '已保存到服务端'
    } catch (e) {
        const text = formatApiError(e)
        docNote.value = `保存失败：${text}`
        // flow_chain_invalid 等编译码在流链段内回显（spec §4.3）
        if (origin === 'flowchain') flowChainError.value = text
    } finally {
        saving.value = false
    }
}

/** 顶栏保存钮 / Ctrl/Cmd+S 入口（错误回显走状态栏反馈段） */
function saveTemplate(): void {
    void runDocumentSave('topbar')
}

/** 抽屉流链段「保存流链」入口：同一文档级通道，错误段内回显（spec §4.3） */
function saveFlowChain(): void {
    void runDocumentSave('flowchain')
}

// ---- 数据源段保存通道（spec §2.4 #6 / §4.3）：显式「保存数据源」→
// PUT /templates/{id}/dataset。校验权威在服务端——schema_invalid /
// dataset_schema_mismatch 等错误码原样段内回显，前端只做「能否成 JSON」的机械
// 解析（解析失败本地拒绝、不打服务端）；成功后 editor.setDataSourceSchema(schema)
// 更新补全候选（null = 清除；注入失败内核静默降级 + console.warn 已内建）。
// dataset 不随文档级 PUT、不进全局 dirty 口径（spec §4.4）。 ----

const datasetSaving = ref(false)

async function saveDataset(): Promise<void> {
    const record = template.value
    if (!record || datasetSaving.value) return
    const draft = datasetDraftPayload(schemaText.value, dataText.value)
    if (!draft.ok) {
        datasetError.value = draft.message
        return
    }
    datasetSaving.value = true
    try {
        const updated = await api.putDataset(record.id, draft.payload)
        template.value = updated
        drawerBaseline.value = {
            ...drawerBaseline.value,
            schemaText: schemaText.value,
            dataText: dataText.value,
        }
        // 补全候选按新 schema 更新（null = 清除；注入失败内核静默降级 + console.warn
        // 已内建）——与载入路径同一入口，口径一致
        editor.setDataSourceSchema(draft.payload.schema)
        docNote.value = '数据源已保存（渲染读取已保存的数据集）'
    } catch (e) {
        datasetError.value = formatApiError(e)
    } finally {
        datasetSaving.value = false
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
                    class="ghost"
                    data-open-datasource
                    title="数据源抽屉：schema/data 数据集与流链（两条独立保存通道）"
                    @click="drawerOpen = !drawerOpen"
                >
                    数据源
                </button>
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
            <!-- canvas-area：帧 tab 条（顶）+ canvas-holder（画布覆盖物定位上下文） -->
            <div class="canvas-area">
                <!-- 帧 tab 条（spec §4.2 多帧，16 票）：画布上方；单击切帧（帧间切换
                     不提示），双击重命名（改 canvases[i].name，进 dirty）；不做增删帧 -->
                <div class="frame-tabs" role="tablist" aria-label="文档帧">
                    <div
                        v-for="(slot, i) in frameSlots"
                        :key="i"
                        class="frame-tab"
                        :class="{ 'is-active': i === activeFrame }"
                        role="tab"
                        :aria-selected="i === activeFrame"
                        :title="`帧 ${i + 1}：${slot.name || '（未命名）'}`"
                        :data-frame-tab="i"
                        @click="switchToFrame(i)"
                        @dblclick="beginFrameRename(i)"
                    >
                        <input
                            v-if="renamingFrame === i"
                            :ref="setRenameInputEl"
                            v-model="renamingValue"
                            class="frame-tab-input"
                            data-frame-rename-input
                            aria-label="帧名"
                            spellcheck="false"
                            @click.stop
                            @dblclick.stop
                            @keydown.enter.prevent="commitFrameRename"
                            @keydown.esc.prevent="cancelFrameRename"
                            @blur="commitFrameRename"
                        />
                        <span v-else class="frame-tab-label" :data-frame-tab-label="i">
                            {{ slot.name || `帧 ${i + 1}` }}
                        </span>
                    </div>
                </div>
                <div class="canvas-holder">
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

        <!-- 数据源抽屉（spec §4.3 双通道，17 票）：文本域草稿与段内错误/标记状态
             全由宿主持有，组件纯呈现；段内独立标记与全局保存态指示灯分离 -->
        <DataSourceDrawer
            v-model:schema-text="schemaText"
            v-model:data-text="dataText"
            v-model:flow-chain-text="flowChainText"
            :open="drawerOpen"
            :dataset-dirty="datasetSegmentDirty"
            :flow-chain-dirty="flowChainSegmentDirty"
            :dataset-saving="datasetSaving"
            :flow-chain-saving="saving"
            :dataset-error="datasetError"
            :flow-chain-error="flowChainError"
            @close="drawerOpen = false"
            @save-dataset="saveDataset"
            @save-flowchain="saveFlowChain"
        />
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

/* 画布容器 = 帧 tab 条（顶）+ canvas-holder（覆盖物定位上下文）的纵向排布 */
.canvas-area {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-width: 0;
}

/* 帧 tab 条（spec §4.2 多帧，16 票）：画布上方窄条；活动 tab 面板色高亮；
   多帧横向内滚降级（同工具栏姿势） */
.frame-tabs {
    display: flex;
    flex: none;
    align-items: flex-end;
    gap: 2px;
    height: 32px;
    padding: 0 10px;
    overflow-x: auto;
    overflow-y: hidden;
    scrollbar-width: none;
}

.frame-tabs::-webkit-scrollbar {
    display: none;
}

.frame-tab {
    display: flex;
    flex: none;
    align-items: center;
    box-sizing: border-box;
    max-width: 200px;
    height: 26px;
    padding: 0 12px;
    border: 1px solid transparent;
    border-bottom: none;
    border-radius: 8px 8px 0 0;
    font-size: 12px;
    white-space: nowrap;
    color: var(--shell-fg-3);
    cursor: pointer;
    user-select: none;
}

.frame-tab:hover {
    background: var(--shell-hover);
    color: var(--shell-fg-2);
}

.frame-tab.is-active {
    border-color: var(--shell-line);
    background: var(--shell-panel);
    color: var(--shell-fg);
}

.frame-tab-label {
    overflow: hidden;
    text-overflow: ellipsis;
}

.frame-tab-input {
    width: 96px;
    padding: 1px 4px;
    border: 1px solid var(--shell-line-strong);
    border-radius: 4px;
    background: var(--shell-bg);
    font-size: 12px;
    color: var(--shell-fg);
    outline: none;
}

/* canvas-holder：画布与覆盖物（标尺/参考线/对齐浮条）的定位上下文 */
.canvas-holder {
    position: relative;
    flex: 1;
    min-height: 0;
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
