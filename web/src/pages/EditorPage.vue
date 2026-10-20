<script setup lang="ts">
// 编辑器页（spec §4.2 / 15 票装配，16 票升级多帧）：占位页换真编辑器——工具栏全量
// 搬 playground + 语义替换（上传图片接 POST /api/assets、「保存」改 PUT 服务端语义、
// 「打开」钮去掉、预览导出文案标注「预览图」）。接线序照 playground onReady 收敛全套装配：
//   new EditorSession({scheduleFrame: createRafScheduler(), fitMargin: 48, uploadHandler})
//   → setDataSourceSchema(datasetSchema)（null 跳过）
//   → CanvasSurface @ready 内 Canvas2DBackend（基类，不搬 gridBackdrop）+ Materializer
//     + attachContentBackend + setOverlayPainter + materializer 双订阅
//     （doc 变更 → materialize；物化状态变更 → invalidate both）
//   → decodeGraph 逐页 → openDocument 第 0 页 → fitToSurface
//   → decodeGraph 逐页 → openDocument 第 0 页 → fitToSurface
// 页序列（模板页序列工票 03，spec 页形状契约 / ADR 0025；16 票多帧 / 26 票尾部增页 /
// 27 票删页的交互能力面沿用）：画布上方页 tab 条；切页即进宿主内存页槽序列——切出前
// encodeGraph(editor.store.doc) 快照该页、换页 openDocument 重建会话文档（openDocument
// 不重置 schema，无需重复注入——01 票）；tab 双击重命名（改 page.name，进 dirty；页 id
// 稳定不随改名/重排变——工程数据键与 tab key 都引用 id，无迁移记账）；页 tab 条尾部
// 「＋」钮尾部追加空白页（未命名、显示层渲染「第 N 页」缺省名，幅面随追加时活动页、
// 层空，追加即切并进文档级 dirty）；tab hover「✕」删页（flow 附着于页对象随页一并
// 移除，「流链随帧下标重写」随平行数组模型退役）；不做中插/重排/复制（数据结构本次
// 解锁，落位 UI 另立工单）；页间切换不提示。页槽位纯逻辑在 src/editor/pages.ts（TDD 缝）。
// 保存面（spec §4.4 + 页形状契约，16 票文档级口径沿用）：纯手动——按钮 + Ctrl/Cmd+S，
// 无防抖自动保存；保存 = 全量 PUT（name + 当前页 ∪ 页序列各页，页形状 id/name/graph/
// flow）；dirty 跟文档级——当前页 ∪ 页序列任一页（id/name/graph/flow）∪ 模板名；
// dirty 基线 = 载入时各页快照；路由离开 onBeforeRouteLeave confirm + 页签关闭
// beforeunload 双保险；保存成功归 clean、基线同步本次发送各页。
// 数据源抽屉（spec §4.3 两段式，23 票重构：数据源为独立实体、模板持引用；
// 29 票流链段迁出）：绑定段（数据源列表下拉 + 绑定/解绑 → PUT
// /templates/{id}/datasource，引用列整存替换）、内容段（编辑绑定的数据源实体
// ——name/schema/data + 「保存数据源」→ PUT /datasources/{id}，共享实体影响
// 所有引用模板；未绑时「创建并绑定」→ POST + 绑定；已绑另有「另存为新数据源」
// → 弹名字框 POST + 重绑）；抽屉段内独立未保存标记（文本域 vs 载入基线）不混
// 全局指示灯；数据源实体与绑定均不进全局 dirty 口径（spec §4.4）。
// 流链编辑器（29 票引入，模板页序列工票 03 改「页 flow 编辑」）：页 tab 条「流链」
// 钮开合 FlowChainDrawer——结构化页列表（每页一行：不在链/fixed/paged 三选 + quota +
// omitIfEmpty），flow 真值源 = 逐页 page.flow（PageSlot.flow，「条目下标 = 帧下标」
// 平行链草稿数组退役）；本地校验（双 paged/paged 链尾/模板表/rowsPath，
// src/editor/pageflow.ts 纯逻辑，TDD 缝）只做 UI 拦截面（保存禁用 + 行内报错），权威
// 仍在服务端保存预检；「保存流链」与顶栏保存是同一文档级 PUT 通道，flow_chain_invalid
// 等编译码在编辑器段内回显；页 flow 进全局 dirty 口径。
// 另存为（spec §4.5，23 票改单调用）：弹名字输入 → POST /templates（name + pages +
// dataSourceId 引用随行，副本与原模板引用同一数据源实体）
// → 成功后会话延续式重绑（dirty 基线重置为新模板）+ router.replace('/editor/<newId>')
// 不重走载入——App.vue 去 :key 后 editor→editor 参数级导航复用本页实例，路由 id
// 与会话脱钩的导航走防御性重载兜底；中途失败错误归因弹窗内回显，不重绑不导航。
// 渲染终图（spec §4.6，19 票 / tdp 批 B ④ draft 哨兵）：顶栏「渲染终图」→ POST
// /templates/{id}/render（body {version:"draft"}——编辑器是草稿世界，预览渲草稿列
// 现值，发布不被预览倒逼；缺省语义 = 最新已发布归列表页）；等待期按钮禁用，
// 30s deadline 服务端控制、前端不另设超时；失败错误体（稳定码在前）走状态栏
// 反馈段回显；成功自动打开结果抽屉（RenderResultDrawer，只显本次会话最近一次）。
// 与预览导出并存：导出预览图钮保留（文案含「预览图」），抽屉注明「预览仅断行
// 参考，以终图为准」——导出 vs 终图词汇直接做进 UI。
// 发布与版本（template-draft-publish 批 B ②③，spec §6 / 票 02-05）：顶栏「发布」
// 钮 + 轻量版本徽标（未发布/vN，数据 = 载入取一次 GET /versions，发布/回滚后刷
// 新，失败容忍徽标隐藏）——发布编排在 src/editor/publish.ts 纯逻辑（dirty 自动
// 先走保存流成功后 publish，一次动作；保存流不变：编辑器永远打开草稿、保存 =
// 现有 PUT、发布是保存后的显式固化，自动促存是编排便利非「发布即保存」合并）；
// 顶栏「版本」钮开合 VersionHistoryDrawer（版本史 = 发布史），行内回滚 →
// ConfirmDialog（spec 定死文案）→ POST revert 全量记录 → src/editor/revert.ts
// planRevertSession 成算 → 会话延续式替换（dirty 基线重置为新内容即归 clean、
// 工程数据零特殊处理——页 id 随发布快照整体定着、按稳定 id 对叁，对不上的悬空键
// 宽容读丢弃；spec §5），徽标/版本列表刷新 + docNote 反馈。
// 工程数据（project-data spec §6，工单 04/05；键切页 id 工票 03）：编辑器状态随模板
// 聚合持久化——宿主拼装快照 TemplateEditorStateSnapshot（schemaVersion + frames[页 id
// 切片] + prefs），切片自内核导入、宿主只拼装不解释。首载宽容恢复（null/悬空页键 =
// 全默认态起步）+ 切页缝内存映射（exportEditorState 写回、openDocument(canvas,
// frames[页 id]) 同型恢复；页 id 稳定，改名/重排零迁移记账）+ 写入三时点 flush（均
// 无条件幂等——无脏跟踪每次全量快照、无常驻定时器、失败静默 console.warn）：① 切页
// 缝顺带 PUT（fire-and-forget 小载荷写，工单 05）；
// ② 内容保存成功搭车 PUT（工单 04）；③ 路由离开静默 flush（不进 confirm、不拦
// 导航）+ 页签关闭 beforeunload 经 fetch keepalive 发出（工单 05）。状态不计 dirty
// （纯内容口径零改动）。另存为 POST 不带状态、内存映射随会话延续——replace 是
// editor→editor 参数级导航不触发离开钩子，副本首写落下一 flush 时点、原模板状态
// 不受扰动。
// 功能导游（feature-tour 03 票，spec §2.4）：首访轻导触发门——台面首挂（template
// null→record）后单次就绪判定（seen 资格 + 首尾锚点在 DOM + 无既有浮层/抽屉打开），
// 不过静默不弹不重试（spec「数据加载慢下会话再遇」）；顶栏常驻「功能导游」重播钮
// （data-tour-replay，无视 seen，先收既有浮层再开）；导游态快捷键门——window 捕获段
// 拦 keydown（内核/宿主监听全在冒泡段；Tab 与 popover 内 Enter/Space 放行给导游
// 自身 UI，Esc keydown 拦、keyup 放行归 driver）；卸载/路由离开守卫 destroy 句柄。
// 纯逻辑在 src/editor/tour.ts、driver 接线在 tourDriver.ts（01/02 票），本页只做
// 宿主状态编排。
// 红线（01/07 票）：editor-vue 组件全 named 导入、不用 runtime template 字符串、
// editor.store 非响应式（动态读数走 subscribe + shallowRef，禁深度 reactive）。
import { computed, nextTick, onBeforeUnmount, onMounted, provide, ref, shallowRef, watch } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'

import {
    Canvas2DBackend,
    Materializer,
    applyViewportTransform,
    drawResourceMarkers,
    exportPreviewPng,
} from '@hankchen/canvas-browser-renderer'
import type { ResourceState } from '@hankchen/canvas-browser-renderer'
import {
    ARM_CREATE_LAYER_TYPES,
    EditorSession,
    isLockedPath,
    isRootLayerPath,
    rootLayerOf,
} from '@hankchen/canvas-editor'
import type {
    CanvasEditorState,
    EditorShortcutAction,
    FontCatalogEntry,
    LayerType,
    OverlayPainter,
    UploadFile,
} from '@hankchen/canvas-editor'
import {
    ADD_LAYER_MENU,
    ARM_LAYER_CREATE_HINT,
    DropdownMenu,
    FONT_PICKER_KEY,
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
    type FontPickerContext,
} from '@hankchen/canvas-editor-vue'
// 模板表共享 rowsPath 表单（template-table-drag-create 工单 03）：插入▾ 模板表项
// 弹层挂载，确认 = armTemplateTableCreate 武装（layer-panel 域子路径出口）
import { TemplateTableForm } from '@hankchen/canvas-editor-vue/layer-panel'
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
} from '@hankchen/canvas-editor-vue'
import {
    api,
    ApiError,
    formatApiError,
    type DataSourceRecord,
    type DataSourceSummary,
    type RenderRecord,
    type TemplateEditorStateSnapshot,
    type TemplateRecord,
    type TemplateVersionSummary,
} from '../api'
import {
    buildEditorStateSnapshot,
    parseTemplateEditorStateSnapshot,
    pickPageStates,
} from '../editor/editorState'
import {
    baselineFromPages,
    blankPageSlot,
    buildSavePayload,
    canonicalPageFlow,
    decodeGraphJson,
    encodeGraphJson,
    isDocDirty,
    loadPageSlots,
    type PageBaseline,
    type PageSlot,
} from '../editor/pages'
import {
    pageTableInfo,
    validatePageFlows,
    type PageFlowRole,
    type PageFlowRow,
} from '../editor/pageflow'
import {
    dataSourceDraftPayload,
    isBindingChange,
    isSegmentDirty,
    sourceDraftBaseline,
} from '../editor/datasource'
import { runRenderFlow, sanitizeFileBase } from '../editor/render'
import { runPublishFlow, versionBadgeOf } from '../editor/publish'
import { planRevertSession } from '../editor/revert'
import { readSeenVersion, shouldShowTour, TOUR_STEPS, tourEndpointsReady } from '../editor/tour'
import { startFeatureTour, TOUR_POPOVER_CONTENT_SELECTOR, type TourDriverHandle } from '../editor/tourDriver'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import DataSourceDrawer from '../components/DataSourceDrawer.vue'
import FlowChainDrawer from '../components/FlowChainDrawer.vue'
import RenderResultDrawer from '../components/RenderResultDrawer.vue'
import VersionHistoryDrawer from '../components/VersionHistoryDrawer.vue'
import NamePromptDialog from '../components/NamePromptDialog.vue'

// ---- 模板载入（spec §4.1）：GET /templates/{id}；404 template_not_found → 错误
// 提示 + 返回列表链接（按 code 判定，不按 HTTP status）。onMounted 首载与路由 id
// 变化的防御性重载共用（18 票：App.vue 去 :key 后 editor→editor 参数级导航复用
// 本页实例——另存为 replace 前已重绑 template.id，watcher 判同不重载即会话延续；
// 其余脱钩导航走整段重载，补齐 :key 时代的会话-id 绑定保证）。 ----

const route = useRoute()
const router = useRouter()
const template = ref<TemplateRecord | null>(null)
const notFound = ref(false)
const errorText = ref<string | null>(null)

/** 载入序号：异步载入期间路由再变（连续导航）时丢弃过期结果 */
let loadSeq = 0

async function loadTemplate(id: string): Promise<void> {
    const seq = ++loadSeq
    try {
        const record = await api.getTemplate(id)
        if (seq !== loadSeq) return
        if (record.pages.length === 0) {
            errorText.value = '模板不含任何页，无法编辑'
            return
        }
        // 会话建立即注入数据源 schema（spec §4.2 接线序）：模板持引用——按
        // dataSourceId 取数据源实体注入其 schema；未绑 = null = 清除声明 = 无候选，
        // 与会话初始态一致。声明只进编辑器会话态，不进 graph、不动 wire；注入失败
        // 内核静默降级 + console.warn 已内建，宿主不重复处理。openDocument 不重置
        // schema（01 票），切页重建会话文档无需重复注入；数据源保存/绑定变更后走
        // 同一入口 applyBoundSource 更新。
        if (record.dataSourceId === null) {
            applyBoundSource(null)
        } else {
            const source = await api.getDataSource(record.dataSourceId)
            if (seq !== loadSeq) return
            applyBoundSource(source)
        }
        // 页槽序列载入（spec 页形状契约「decodeGraph 逐页」）：逐页解码即验 + canonical
        // 化，基线与页槽同形；任一页解码失败 = 载入错误面（16 票口径沿用）；flow 随页
        // 宽容归一（flowFromWire），流链段基线 = 各页 flow canonical 串
        try {
            pageSlots.value = loadPageSlots(record.pages)
        } catch (e) {
            errorText.value = e instanceof Error ? e.message : String(e)
            return
        }
        flowChainBaselineFlows.value = pageSlots.value.map((slot) => canonicalPageFlow(slot.flow))
        // 工程数据快照恢复编排（工单 04，spec §6 首载序「GET editorState → 逐页
        // decode → openDocument(页0, 切片) → applyEditorPrefs 一次性」的宿主半边）：
        // 宽容解析（editorState null/形状不合法 = 全默认态起步，零报错）→ 悬空页键
        // （删页/回滚遗留）剔除后种子进内存映射（切页往返 / 后续 flush 载荷所需）→
        // prefs 一次性应用。prefs 应用先于页 0 的 openDocument（spec §6 字面序相反）
        // ——openDocument reset 保留标尺等偏好（内核自持，工单 01 已验 reset 语义），
        // 先后等价；快照无 prefs 则不应用（内核自持，不重置）。页 0 切片实际入会话
        // 在 openPageDocument 的 openDocument(canvas, frames[页 id])（onReady 或
        // 重载路径收口）。
        const restored = parseTemplateEditorStateSnapshot(record.editorState)
        pageStates.value = pickPageStates(restored.frames, pageSlots.value.map((slot) => slot.id))
        if (restored.prefs) editor.applyEditorPrefs(restored.prefs)
        savedBaseline = baselineFromPages(record.name, pageSlots.value)
        activePage.value = 0
        template.value = record
        templateName.value = record.name
        // 版本徽标数据（批 B ②）：载入取一次 GET /versions，fire-and-forget——
        // 拉取失败徽标隐藏（null = 未知），不阻塞编辑器会话建立
        void refreshVersions()
        // 重载路径（画布已就绪，onReady 不会再来）：换文档重开当前页；首载路径由
        // onReady 收口（CanvasSurface 在 template 置位后才挂载）
        if (contentBackend !== null) openPageDocument()
    } catch (e) {
        if (seq !== loadSeq) return
        if (e instanceof ApiError && e.code === 'template_not_found') {
            notFound.value = true
        } else {
            errorText.value = formatApiError(e)
        }
    }
}

onMounted(() => {
    void loadTemplate(String(route.params.id))
})

// 路由 id 防御性重载（18 票，App.vue 去 :key 的绑定保证承接）：另存为 replace 已
// 先重绑 template.id → 判同不重载（会话延续）；其余 id 脱钩的参数变化重置页面态
// 后整段重载。离开编辑器路由（返回列表）不触发。
watch(
    () => route.params.id,
    (id) => {
        if (route.name !== 'editor') return
        if (template.value !== null && String(template.value.id) === String(id)) return
        notFound.value = false
        errorText.value = null
        template.value = null
        void loadTemplate(String(id))
    },
)

// ---- 页槽序列（模板页序列工票 03；spec §4.2 16 票 / 26 票增页 / 27 票删页交互面
// 沿用）：宿主内存页槽序列 = record.pages 的宿主镜像，graphJson 以 canonical encode
// 字符串持有（dirty 比较/保存载荷直接消费字符串）；activePage 当前页下标（页身份由
// slot.id 承载）；增页 = 尾部追加（见 appendBlankPage），删页 = 从页序列 splice（flow
// 随页一并移除，见 deletePage），不做中插/重排/复制。 ----

const pageSlots = ref<PageSlot[]>([])
const activePage = ref(0)

// ---- 工程数据内存映射（工单 04；键切页 id 工票 03，spec §6）：会话内存页状态切片
// 映射，键 = 页 id（稳定不随改名/重排变，spec 页形状契约）。切页缝 exportEditorState
// 写回/恢复维护（切页正确性所需、零额外成本），载入时自快照种子（pickPageStates 悬空
// 键剔除），flush 时与活动页现取切片拼装成完整快照。不进 dirty 口径（纯内容口径零改
// 动）、不驱动渲染——纯切页/flush 正确性记账。 ----

const pageStates = ref<Record<string, CanvasEditorState>>({})

/** 当前会话的完整工程数据快照（flush 载荷）：活动页切片现取（exportEditorState 的
 *  ui 现值，映射写回值可能滞后到切出时刻），其余页取映射（切页缝写回值，未访问页
 *  = 种子值或无切片）；prefs 现读。组装与拷贝语义在 editorState.ts 纯逻辑单测。 */
function currentEditorStateSnapshot(): TemplateEditorStateSnapshot {
    return buildEditorStateSnapshot({
        pageIds: pageSlots.value.map((slot) => slot.id),
        slices: pageSlots.value.map((slot, i) =>
            i === activePage.value ? editor.exportEditorState() : (pageStates.value[slot.id] ?? null),
        ),
        prefs: editor.getEditorPrefs(),
    })
}

/** 切出页状态切片写回映射（切页缝）：ui 现值 exportEditorState，键 = 页 id */
function stashActivePageState(): void {
    const leavingId = pageSlots.value[activePage.value]?.id
    if (leavingId) pageStates.value[leavingId] = editor.exportEditorState()
}

/** 切出缝 = graph 快照入槽 + 状态切片写回映射 + 工程数据 flush（spec §4.2 切页
 *  快照 + 工单 04 状态写回 + 工单 05 时点一，同一时刻、同一页）：switchToPage /
 *  appendBlankPage / deletePage 共用；保存缝（runDocumentSave/runSaveAs）只取
 *  graph 快照——状态现取（currentEditorStateSnapshot 的活动页现读），不走映射。
 *  缝尾顺带 flush（时点一）：载荷含切出页现场切片（活动页现读 = 刚写回映射的
 *  同值）+ 映射其余页 + prefs，fire-and-forget 不等结果、不阻塞切页。deletePage
 *  的 flush 载荷瞬时含将删页键（stash 先于 splice/键剔除）——悬空键由恢复端宽容
 *  读丢弃 + 下次 flush 覆盖自愈（记档）。 */
function stashActivePage(): void {
    snapshotActivePage()
    stashActivePageState()
    flushEditorState()
}

/** 切出前快照当前页（spec §4.2：encodeGraph(editor.store.doc) 快照该页进序列） */
function snapshotActivePage(): void {
    const doc = editor.store.doc
    if (!doc) return
    const index = activePage.value
    const slot = pageSlots.value[index]
    if (!slot) return
    pageSlots.value[index] = { ...slot, graphJson: encodeGraphJson(doc) }
}

/** 切页（spec §4.2）：快照切出页 → decodeGraph 重建会话文档（ui 选择/撤销历史等
 *  会话态随 openDocument 重置）。视口保留不 refit；schema 不重置无需重复注入；
 *  materialize 与 dirty 重算由 doc 订阅顺带驱动；页间切换不提示。
 *  工程数据（工单 04）：切出页状态切片写回映射 → 切入页切片随 openDocument(canvas,
 *  frames[页 id]) 同型恢复（reset → apply → prune 原子管线；无切片 = 全默认态），
 *  页间往返参考线/锁定随映射回来。 */
function switchToPage(index: number): void {
    if (index === activePage.value || !pageSlots.value[index]) return
    stashActivePage()
    const doc = decodeGraphJson(pageSlots.value[index]!.graphJson)
    activePage.value = index
    editor.openDocument(doc, pageStates.value[pageSlots.value[index]!.id])
}

/** 尾部追加空白页（spec §4.2 增页，26 票口径沿用）：追加位置恒为尾部与活动页位置
 *  无关——flow 附着于页对象，页序列追加零连带改写（平行链草稿同步随模型退役）。
 *  时序复用 16 票切页缝：先快照当前页入槽（防丢未保存编辑）+ 状态切片写回映射
 *  （工单 04 切页缝同款）→ push 新槽位（未命名、显示层渲染「第 N 页」，幅面 = 追加
 *  时活动页画布宽高、层空、flow null 链外页起步）→ activePage 指向新页 →
 *  openDocument 空白文档（新页无切片 = 全默认态；schema 不重置无需注入；与活动页
 *  同幅，切页惯例视口保留不 refit）。dirty 重算与 materialize 由 openDocument
 *  的 doc 通知顺带驱动（store 契约：openDocument 必发 doc 通知，switchToPage 同款），
 *  槽位数超基线在 isDocDirty 长度收紧处成立。 */
function appendBlankPage(): void {
    const doc = editor.store.doc
    if (!template.value || !doc) return
    stashActivePage()
    const index = pageSlots.value.length
    pageSlots.value.push(blankPageSlot(doc.width, doc.height))
    activePage.value = index
    editor.openDocument(decodeGraphJson(pageSlots.value[index]!.graphJson))
}

// ---- 删页（spec §4.2 删页，27 票口径沿用；页模型工票 03）：flow 附着于页对象，
// 删除即从页序列 splice——被删页与其 flow、工程数据键一并移除，无第二份数据要同步
// （「流链随帧下标重写」随平行数组模型退役）。中插/重排/复制仍不做（UI 另立工单）。 ----

/** 删页确认文案（title 单点，确认框同口径）：「下标前移/流链重写」措辞随模型退役
 *  改写——flow 与工程数据键都引用页 id，随页一并移除，其余页不受影响 */
const DELETE_PAGE_TITLE = '删除该页：该页的行分配策略（flow）与工程数据（参考线/锁定）随页一并移除；该页内容删除后不可恢复'

/** 删页入口：唯一页不可删（pages 空是非法态）；confirm 前置即防误删（页级操作
 *  不进撤销历史，宿主态同 16/26 票边界） */
function confirmDeletePage(index: number): void {
    if (pageSlots.value.length <= 1) return
    const label = pageSlots.value[index]?.name || `第 ${index + 1} 页`
    if (!window.confirm(`删除「${label}」？${DELETE_PAGE_TITLE.replace('删除该页：', '')}`)) return
    deletePage(index)
}

/** 删页执行：切出缝 stash（graph 快照 + 状态写回——删活动页之前的页时活动页会被
 *  openDocument 重建，现场切片必须先进映射，否则随映射恢复的是上次切页的旧值）→
 *  splice 删槽位 → 被删页切片按 id 无条件出映射（id 页内唯一，无「同名幸存」歧义，
 *  帧名键时代的保守清理补丁撤销）→ 活动页重定向按页身份处理（删活动页之前：同一页
 *  id 保持活动，仅下标前移一位；删活动页：取同位即后继页、末位越界缩一；命中活动页
 *  变化才 openDocument 重建会话文档，切片随映射恢复同切页，schema 不重置、视口保留
 *  不 refit）→ 显式 syncDirty（删活动页之后的页不产生文档事件，槽位数变化需手动
 *  触发重算）。 */
function deletePage(index: number): void {
    if (!template.value || pageSlots.value.length <= 1) return
    if (renamingPage.value !== null) cancelPageRename()
    stashActivePage()
    const deletedId = pageSlots.value[index]?.id
    pageSlots.value.splice(index, 1)
    if (deletedId) {
        const next = { ...pageStates.value }
        delete next[deletedId]
        pageStates.value = next
    }
    const wasActive = index === activePage.value
    const beforeActive = index < activePage.value
    if (beforeActive) {
        activePage.value -= 1
    } else if (wasActive) {
        activePage.value = Math.min(index, pageSlots.value.length - 1)
    }
    if (wasActive || beforeActive) {
        editor.openDocument(
            decodeGraphJson(pageSlots.value[activePage.value]!.graphJson),
            pageStates.value[pageSlots.value[activePage.value]!.id],
        )
    }
    syncDirty()
}

// ---- tab 双击重命名（spec §4.2 + 页形状契约）：改 page.name（可选字段，进 dirty
// ——name 随保存载荷上行）；页 id 不动，工程数据键零迁移记账（帧名键时代的改名迁移
// 补丁撤销）。空名拒绝提交。 ----

const renamingPage = ref<number | null>(null)
const renamingValue = ref('')
const renameInputEl = ref<HTMLInputElement | null>(null)

function setRenameInputEl(el: unknown): void {
    renameInputEl.value = el instanceof HTMLInputElement ? el : null
}

watch(renamingPage, async (index) => {
    if (index === null) return
    await nextTick()
    renameInputEl.value?.focus()
    renameInputEl.value?.select()
})

function beginPageRename(index: number): void {
    renamingPage.value = index
    renamingValue.value = pageSlots.value[index]?.name ?? ''
}

function commitPageRename(): void {
    const index = renamingPage.value
    renamingPage.value = null
    if (index === null) return
    const next = renamingValue.value.trim()
    const slot = pageSlots.value[index]
    if (!slot || !next || next === slot.name) return
    pageSlots.value[index] = { ...slot, name: next }
    syncDirty()
}

function cancelPageRename(): void {
    renamingPage.value = null
}

// ---- 数据源抽屉（spec §4.3 两段式，23 票：数据源为独立实体、模板持引用）。
// 段内独立未保存标记 = 文本域 vs 载入基线（spec §4.4，不混全局 saveState 指示灯）
// ——数据源实体与绑定都不在全局 dirty 口径（独立通道即时落库），抽屉开着改全局
// 仍 clean。各段保存成功后基线同步本次发送文本。 ----

const drawerOpen = ref(false)

// ---- 流链编辑器（29 票引入；工票 03 改「页 flow 编辑」）：页 tab 条「流链」钮
// 开合 FlowChainDrawer。flow 真值源 = 逐页 page.flow（PageSlot.flow，附着于页对象，
// 平行链草稿数组退役）；段内基线 = 载入（或保存成功）时的各页 flow canonical 串，
// 独立未保存标记与全局指示灯分离——flow 本身在全局 dirty 口径内（spec §4.4，经
// syncDirty 计入）。本地校验只是 UI 拦截面（保存禁用 + 行内报错），权威仍在服务端
// 保存预检。 ----

const flowChainDrawerOpen = ref(false)
const flowChainBaselineFlows = shallowRef<string[]>([])
const flowChainSegmentDirty = computed(
    () =>
        pageSlots.value.length !== flowChainBaselineFlows.value.length ||
        pageSlots.value.some((slot, i) => canonicalPageFlow(slot.flow) !== flowChainBaselineFlows.value[i]),
)
// 页摘要（编辑器行标 + rowsPath 提示/表校验输入）：graphJson 取页槽现值
// （含未保存编辑，提示与校验跟着现场走）；行序 = 页序、行 key = 页 id
const flowChainPages = computed<PageFlowRow[]>(() =>
    pageSlots.value.map((slot, i) => ({
        id: slot.id,
        label: slot.name || `第 ${i + 1} 页`,
        table: pageTableInfo(slot.graphJson),
        flow: slot.flow,
    })),
)
const flowChainLocalErrors = computed(() =>
    validatePageFlows(
        pageSlots.value.map((slot) => slot.flow),
        flowChainPages.value,
    ),
)
// 链上页 id 集合（tab 角标）——链上页数徽标直用 .size
const onChainPages = computed(() => {
    const set = new Set<string>()
    for (const slot of pageSlots.value) {
        if (slot.flow !== null) set.add(slot.id)
    }
    return set
})

// 绑定段：数据源列表摘要（抽屉打开时刷新）+ 选中项草稿。列表错误不影响其余段。
const sources = ref<DataSourceSummary[]>([])
const selectedBindId = ref<number | null>(null)
const bindSaving = ref(false)
const bindError = ref<string | null>(null)

// 内容段：编辑的是「绑定的数据源实体」；未绑 = 创建并绑定的起笔态（三文本域空）。
const boundSource = ref<DataSourceRecord | null>(null)
const nameText = ref('')
const schemaText = ref('')
const dataText = ref('')
const contentBaseline = ref({ nameText: '', schemaText: '', dataText: '' })
const contentDirty = computed(
    () =>
        isSegmentDirty(nameText.value, contentBaseline.value.nameText) ||
        isSegmentDirty(schemaText.value, contentBaseline.value.schemaText) ||
        isSegmentDirty(dataText.value, contentBaseline.value.dataText),
)
const contentSaving = ref(false)
const contentError = ref<string | null>(null)
// 绑定 schema 的内核拒绝明细（死端态整改 2b，applyBoundSource 注入时快照）
const schemaRejected = ref<string | null>(null)

/** 应用绑定态（spec §4.3 载入/保存/绑定同一入口）：实体进内容段基线与文本域、
 *  schema 注入编辑器会话（null = 清除候选；注入失败内核静默降级已内建） */
function applyBoundSource(source: DataSourceRecord | null): void {
    boundSource.value = source
    contentBaseline.value = sourceDraftBaseline(source)
    nameText.value = contentBaseline.value.nameText
    schemaText.value = contentBaseline.value.schemaText
    dataText.value = contentBaseline.value.dataText
    selectedBindId.value = source?.id ?? null
    contentError.value = null
    editor.setDataSourceSchema(source === null ? null : source.schema)
    // 声明被拒快照（死端态整改 2b）：注入同步生效，直读会话态即可——开放映射/
    // 保留键等被静默降级为无候选，抽屉回显原因，否则用户「绑了数据源」却无候选
    const rejection = editor.store.ui.dataSourceSchemaRejection
    schemaRejected.value =
        rejection === null ? null : `当前 schema 被编辑器拒绝（${rejection.reason}）：${rejection.detail}——表达式字段引用将无候选，修正后保存数据源即恢复`
}

/** 抽屉打开：数据源列表刷新（共享影响面 templateCount 与新建实体即时可见） */
watch(drawerOpen, async (open) => {
    if (!open) return
    try {
        sources.value = await api.listDataSources()
    } catch (e) {
        bindError.value = formatApiError(e)
    }
})

// 段内错误随再编辑清空（陈旧错误误导）；页 flow 变更计入全局 dirty 口径
// （flow 真值源在页槽，三事件 handler 末尾直接清错误 + syncDirty 收口——见 setPageFlow*）
const flowChainError = ref<string | null>(null)
watch([nameText, schemaText, dataText], () => {
    contentError.value = null
})
watch(selectedBindId, () => {
    bindError.value = null
})

// ---- 会话（spec §4.2 接线序第 1 步）：fitMargin 48 与 playground 同款；上传注入
// 点接 POST /api/assets（响应 url 前导斜杠形态，原样写入 graph spec.src；扩展名
// 白名单不校验内容类型，字体文件同样可传）。文本度量不注入（票面构造参数口径），
// 预览断行为参考、终图以服务端为准。 ----

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

// 字体清单（网络字体方案）：GET /api/fonts 下发 {label, ref} 白名单（seed/
// fonts.json，中文开源字体直链）→ 内核 FontCatalog addCustom 补录（同引用去重）
// → FONT_PICKER_KEY 注入缝让文字图层字体字段渲染下拉。清单 ≠ 物化：预览
// FontFace 与服务端渲染（远程物化进 .cache/）各自按 ref 拉取。拉取失败不阻塞
// 编辑器——下拉退化为「内置默认 + 本机上传」（手输文本框只属未接缝宿主的
// 兼容形态，本页恒接缝，上传通道不受清单影响）
api.listFonts()
    .then((fonts) => {
        for (const font of fonts) editor.fontCatalog.addCustom(font)
    })
    .catch(() => {})

/** 清单响应式桥：上传追加的字体经 catalog 订阅实时进下拉（playground 同款） */
const fontCatalogEntries = ref<readonly FontCatalogEntry[]>(editor.fontCatalog.entries)
const unsubscribeFontCatalog = editor.fontCatalog.subscribe((entries) => {
    fontCatalogEntries.value = entries
})
provide(FONT_PICKER_KEY, {
    entries: fontCatalogEntries,
    canUpload: editor.canUpload,
    uploadFont: (file) => editor.uploadFont(file),
} satisfies FontPickerContext)

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

/** 插入▾：面板＋ ADD_LAYER_MENU 同源五项（文案与 title 单点维护）。四图层项点击 =
 *  武装画拉 + 状态栏瞬时提示；模板表项（template-table-drag-create 工单 03 同序收入）
 *  keepsOpen 交换弹层挂共享 TemplateTableForm——确认 = armTemplateTableCreate 武装 +
 *  同句提示 + 收弹层；取消零副作用收弹层 */
const insertItems = computed<DropdownMenuEntry[]>(() =>
    insertTemplateFormOpen.value
        ? []
        : ADD_LAYER_MENU.map((entry) =>
              entry.kind === 'layer'
                  ? {
                        label: entry.label,
                        title: entry.title,
                        shortcut: shortcutActionLabel(ARM_CREATE_ACTION_BY_TYPE[entry.type], shortcutPlatform),
                        run: () => {
                            editor.armLayerCreate(entry.type)
                            useTransientFeedback().show(ARM_LAYER_CREATE_HINT)
                        },
                    }
                  : {
                        label: entry.label,
                        title: entry.title,
                        dataAttrs: { 'data-add-layer': 'template-table' },
                        keepsOpen: true,
                        run: () => {
                            insertTemplateFormOpen.value = true
                        },
                    },
          ),
)

/** 插入▾ 模板表弹层开合态（面板＋ templateFormOpen 同款）：随菜单收清（closed） */
const insertTemplateFormOpen = ref(false)

/** 弹层表单确认：同一内核武装缝 + 同句提示 + 收弹层；落库归画拉 endCreate（与面板链路同缝） */
function onInsertTemplateConfirm(rowsPath: string, closeMenu: () => void): void {
    editor.armTemplateTableCreate(rowsPath)
    useTransientFeedback().show(ARM_LAYER_CREATE_HINT)
    closeMenu()
}

/** 弹层表单取消：收弹层不武装、零副作用（草稿/错误态随组件实例收挂即弃） */
function onInsertTemplateCancel(closeMenu: () => void): void {
    insertTemplateFormOpen.value = false
    closeMenu()
}

/** 插入▾ 随收清理（底座 closed 事件）：表单开合态不跨开合残留 */
function onInsertMenuClosed(): void {
    insertTemplateFormOpen.value = false
}

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

    // 载入当前页（spec §4.2 接线序收口）：页槽 graphJson → decodeGraph →
    // openDocument → fit；首载（第 0 页）与另存为/防御性重载后的换文档共用
    openPageDocument()
}

/** 打开当前页文档（spec §4.2）：页槽 graphJson → decodeGraph → openDocument →
 *  计一次 dirty 让守卫立即可用 → 物化 → 整页 fit（初始进入与重载同语义）。
 *  工程数据（工单 04；键切页 id 工票 03）：切片自内存映射按页 id 取，随
 *  openDocument(canvas, frames[页 id]) 原子应用（载入种子 = 首载恢复编排的页 0
 *  恢复；无切片 = 全默认态）。 */
function openPageDocument(): void {
    if (!template.value || pageSlots.value.length === 0) return
    const doc = decodeGraphJson(pageSlots.value[activePage.value]!.graphJson)
    editor.openDocument(doc, pageStates.value[pageSlots.value[activePage.value]!.id])
    // dirty 基线已在载入时打快照（各页 canonical 串）；此处即算一次让守卫立即可用
    computeDirty()
    materializer?.materialize(doc)
    editor.fitToSurface()
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
/** dirty 基线 = 载入（或保存成功）时的文档级快照（spec §4.4 文档级口径，页模型）：
 *  模板名 + 各页载荷字节（id/name/graph/flow canonical 串） */
let savedBaseline: PageBaseline | null = null

function computeDirty(): void {
    if (savedBaseline === null || template.value === null) {
        isDirty.value = false
        return
    }
    // flow 现值随页槽参与比较（spec §4.4 口径含 flow；页模型下 flow 附着于页对象，
    // 无独立草稿分支）
    const doc = editor.store.doc
    isDirty.value = isDocDirty({
        baseline: savedBaseline,
        slots: pageSlots.value,
        activeIndex: activePage.value,
        activeGraphJson: doc ? encodeGraphJson(doc) : null,
        templateName: templateName.value,
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
const SAVE_TITLE = '保存到服务端（Ctrl/Cmd+S；全量 PUT：name + 文档各页）'

/** 工程数据搭车写（工单 04，spec §6 写入时点二）：内容保存成功的同一用户动作缝随发
 *  PUT editor-state（载荷 = schemaVersion + frames + prefs 整存替换）。fire-and-forget：
 *  不等结果、不重试、不阻塞保存完成与错误面；失败静默 console.warn（spec §6 失败面
 *  ——无 toast、无状态栏标记、无重试；中断最多丢最近状态增量，恢复端 prune 兜底）。
 *  状态不计 dirty（纯内容口径零改动），保存态指示灯不受影响。快照由保存缝预取
 *  （与发送内容同一时刻），不走 flushEditorState 现取。 */
function putEditorState(id: number, snapshot: TemplateEditorStateSnapshot): void {
    api.updateTemplateEditorState(id, snapshot).catch((e) => {
        console.warn('[editor-state] 搭车保存失败（静默降级）', e)
    })
}

/** 工程数据 flush（工单 05，spec §6 写入时点一/三之一共用）：当前会话全量快照
 *  无条件 PUT——状态不设脏跟踪，flush 幂等，每次全量（小载荷，切页/离开频率下
 *  可接受）。fire-and-forget：不等结果、不阻塞动作；失败静默 console.warn。
 *  template 未绑（载入中/载入失败）跳过。 */
function flushEditorState(): void {
    const record = template.value
    if (!record) return
    api.updateTemplateEditorState(record.id, currentEditorStateSnapshot()).catch((e) => {
        console.warn('[editor-state] 状态 flush 失败（静默降级）', e)
    })
}

/** 页签关闭 flush（工单 05，spec §6 写入时点三之二）：同快照经 keepalive 通道
 *  发出（unload 期存活；sendBeacon 只能 POST，PUT 写分道语义保不住）。同样无条件
 *  幂等；载荷自足（currentEditorStateSnapshot 同步组装、JSON 同步串化）。 */
function flushEditorStateKeepalive(): void {
    const record = template.value
    if (!record) return
    api.updateTemplateEditorStateKeepalive(record.id, currentEditorStateSnapshot())
}

/** 保存 = 全量 PUT（spec 页形状契约 + §2.4 #5 整存替换语义，§4.4 文档级口径）：当前页
 *  快照并入页序列（与切出同一条缝）→ 载荷 = name + 页序列各页（id/name/graph/flow）。
 *  flow 以各页现值随载荷整存（工票 03，附着于页对象）——「保存流链」与顶栏保存是
 *  同一条文档级通道，区别只在错误回显面（编辑器段内 vs 状态栏）；本地校验不过不发货
 *  （顶栏入口就地提示，编辑器入口保存钮已禁用）。基线 = 本次发送字节，保存期间的
 *  继续编辑保持 dirty。工程数据（工单 04）：快照随同一保存缝组装，内容 PUT 成功后
 *  搭车写。返回 true = 保存成功（发布编排的促存链需要感知成败；常规入口忽略返回值）。 */
async function runDocumentSave(origin: 'topbar' | 'flowchain'): Promise<boolean> {
    const record = template.value
    if (!record || !editor.store.doc || saving.value) return false
    if (flowChainLocalErrors.value.length > 0) {
        if (origin === 'topbar') docNote.value = '保存失败：流链未通过本地校验（打开页条「流链」编辑器修复后再保存）'
        return false
    }
    snapshotActivePage()
    const sentName = templateName.value
    const sentSlots = pageSlots.value.map((slot) => ({ ...slot }))
    const editorState = currentEditorStateSnapshot()
    const payload = buildSavePayload({ slots: sentSlots, templateName: sentName })
    saving.value = true
    docNote.value = '保存中…'
    try {
        const updated = await api.updateTemplate(record.id, payload)
        template.value = updated
        // 基线 = 本次发送字节（各页 id/name/graph/flow canonical 串），保存期间的
        // 继续编辑保持 dirty
        savedBaseline = baselineFromPages(sentName, sentSlots)
        // flow 已随本次文档级 PUT 落库：段基线同步各页 flow canonical 串，段内标记归灭
        flowChainBaselineFlows.value = sentSlots.map((slot) => canonicalPageFlow(slot.flow))
        flowChainError.value = null
        computeDirty()
        docNote.value = '已保存到服务端'
        // 工程数据搭车写（工单 04）：fire-and-forget，失败静默，不影响保存结果面
        putEditorState(record.id, editorState)
        return true
    } catch (e) {
        const text = formatApiError(e)
        docNote.value = `保存失败：${text}`
        // flow_chain_invalid 等编译码在流链编辑器段内回显（spec §4.2，29 票）
        if (origin === 'flowchain') flowChainError.value = text
        return false
    } finally {
        saving.value = false
    }
}

/** 顶栏保存钮 / Ctrl/Cmd+S 入口（错误回显走状态栏反馈段） */
function saveTemplate(): void {
    void runDocumentSave('topbar')
}

/** 流链编辑器「保存流链」入口：同一文档级通道，错误段内回显（spec §4.2，29 票） */
function saveFlowChain(): void {
    void runDocumentSave('flowchain')
}

// ---- 发布与版本徽标（template-draft-publish 批 B ②，spec §6）：顶栏「发布」
//      按钮 + 轻量徽标（「未发布」/「vN」）。徽标数据 = 载入时取一次 GET /versions
//      （发布/回滚后刷新）；拉取失败不阻塞编辑器——徽标隐藏（null = 未知）而不
//      假显「未发布」；版本抽屉对 null 显「版本列表不可用」（评审修复 F1：
//      未知 ≠ 未发布，null 语义由 versionListState 纯逻辑钉死）。发布编排在
//      src/editor/publish.ts 纯逻辑（TDD 缝）：dirty 自动先走保存流成功后
//      publish，一次动作；保存流不变声明 = 编辑器永远打开草稿、保存 = 现有 PUT、
//      发布是保存后的显式固化，自动促存是编排便利而非「发布即保存」语义合并。
//      反馈复用内联模式（按钮等待态 + docNote），无 toast。 ----

const versions = ref<TemplateVersionSummary[] | null>(null)
const versionBadgeLabel = computed(() => (versions.value === null ? null : versionBadgeOf(versions.value)))

/** 版本列表刷新：失败容忍（保持现值不置 null——发布/回滚主流程已成功时旧值比
 *  「未知」更接近真相；载入首拉失败则保持 null = 徽标隐藏、抽屉显「版本列表
 *  不可用」） */
async function refreshVersions(): Promise<void> {
    const record = template.value
    if (!record) return
    try {
        versions.value = await api.listTemplateVersions(record.id)
    } catch {
        // 徽标刷新失败静默（spec §6 内联反馈模式，不阻塞主流程）
    }
}

const publishing = ref(false)

/** 发布入口（票 05 编排）：先同步即时比较 dirty（防抖窗口内的最近编辑也要计入，
 *  路由守卫同款口径），dirty 走 runPublishFlow 自动促存再发；成功后刷新徽标与
 *  版本列表，docNote 反馈「已发布 vN」/「内容与 vN 一致，跳过重复发布」。 */
async function runPublish(): Promise<void> {
    const record = template.value
    if (!record || publishing.value || saving.value) return
    publishing.value = true
    try {
        computeDirty()
        const outcome = await runPublishFlow({
            dirty: isDirty.value,
            save: () => runDocumentSave('topbar'),
            publish: () => api.publishTemplate(record.id),
        })
        if (outcome.ok) {
            docNote.value = outcome.note
            await refreshVersions()
        } else if (outcome.note !== null) {
            docNote.value = outcome.note
        }
        // note null = 保存失败已自带错误面（docNote「保存失败：…」），不覆盖肇因
    } finally {
        publishing.value = false
    }
}

// ---- 版本历史抽屉与回滚（template-draft-publish 批 B ③，spec §6 / 票 04）：顶栏
//      「版本」钮开合 VersionHistoryDrawer（复用抽屉模式，宿主持有全部状态）；行内
//      回滚 → ConfirmDialog（spec 定死文案）→ POST revert 拿全量记录 → planRevertSession
//      成算 → 会话替换（见 runRevert 详注）。 ----

const versionDrawerOpen = ref(false)

const revertOpen = ref(false)
const revertTarget = ref<TemplateVersionSummary | null>(null)
const revertError = ref<string | null>(null)
const reverting = ref(false)

/** 确认框文案（spec §6 定死句式，N = 目标版本号） */
const revertNote = computed(() =>
    revertTarget.value === null
        ? '将用历史版本的内容覆盖当前草稿；未保存的修改将丢失'
        : `将用版本 ${revertTarget.value.version} 的内容覆盖当前草稿；未保存的修改将丢失`,
)

/** 行内回滚入口：弹确认框（服务端零守卫，确认交互在前端——票 04） */
function openRevert(v: TemplateVersionSummary): void {
    revertTarget.value = v
    revertError.value = null
    revertOpen.value = true
}

function closeRevert(): void {
    revertOpen.value = false
    revertError.value = null
    revertTarget.value = null
}

/** 回滚执行（spec §5 / 票 04，批 B ③；页模型工票 03）：POST revert 200 回全量模板
 *  记录（与 GET /templates/{id} 同形，无需二次 GET），拿记录做**会话替换**——会话
 *  延续式（另存为先例，不整页重载、内容后端已就绪走 openPageDocument 重开当前页）：
 *  - template/模板名/页槽序列/活动页归 0 整体切换；savedBaseline = plan.baseline
 *    （**dirty 基线重置为新内容**——回滚后立即归 clean，与 loadTemplate/另存为
 *    同口径；继续编辑才再 dirty，满意后发布成新版本，版本线线性）；
 *  - flow 随页槽载入（宽容归一），段基线同步各页 flow canonical 串；
 *  - **工程数据零特殊处理**（spec §5）：pageStates 不动——页 id 随发布快照整体定着，
 *    与现会话页按稳定 id 对叁（id 同 = 参考线/锁定跟对页，spec US9 语义）；对不上
 *    的悬空键由宽容读丢弃（pickPageStates 精神），flush 载荷按现役页 id 组装自愈；
 *  - 数据源绑定：revert 不碰绑定列（dataSourceId 理论不变），仍按全量记录如实
 *    重取/重注 schema（applyBoundSource 同入口，与 loadTemplate 同口径）；
 *  - 徽标/版本列表刷新 + docNote 反馈「已回滚到 vN」。
 *  成算先行：planRevertSession 任一页解码失败抛错时会话零变更，错误留在确认框内
 *  可重试/取消；API 失败同样框内回显。 */
async function runRevert(): Promise<void> {
    const record = template.value
    const target = revertTarget.value
    if (!record || !target || reverting.value) return
    reverting.value = true
    try {
        const updated = await api.revertTemplate(record.id, target.version)
        // 成算先行（全有或全无）：planRevertSession 任一页解码失败、数据源实体重取
        // 失败都发生在任何会话变更之前——失败时原会话零变更，错误留在确认框内
        const plan = planRevertSession(updated)
        const source = updated.dataSourceId === null ? null : await api.getDataSource(updated.dataSourceId)
        // ---- 会话替换（同步赋值段，以上均成功后才执行）----
        template.value = updated
        templateName.value = updated.name
        pageSlots.value = plan.slots
        activePage.value = 0
        savedBaseline = plan.baseline
        flowChainBaselineFlows.value = plan.slots.map((slot) => canonicalPageFlow(slot.flow))
        flowChainError.value = null
        // 数据源绑定按全量记录如实重注 schema（applyBoundSource 同入口）
        applyBoundSource(source)
        revertOpen.value = false
        openPageDocument()
        await refreshVersions()
        revertTarget.value = null
        docNote.value = `已回滚到 v${target.version}`
    } catch (e) {
        revertError.value = formatApiError(e)
    } finally {
        reverting.value = false
    }
}

// ---- 版本历史行内渲染（渲染历史对比 / 版本级预览）：行内「渲染」→
// POST render {version:N} 渲染该版本快照（不触草稿列、不产生版本行——发版前先
// 看历史版本效果，或发布后比对历史）；成功刷新渲染历史 + 打开结果抽屉
// （activeRecordId 指向本次）。在途态行内显示（与回滚钮同款 revertingVersion
// 模式），错误走状态栏反馈段。 ----

const versionRendering = ref(false)
const versionRenderingTarget = ref<number | null>(null)

async function runVersionRender(v: TemplateVersionSummary): Promise<void> {
    const record = template.value
    if (!record || versionRendering.value) return
    versionRendering.value = true
    versionRenderingTarget.value = v.version
    docNote.value = `渲染 v${v.version} 中…`
    try {
        const result = await api.renderTemplate(record.id, { version: v.version })
        await refreshRenderHistory(result)
        renderDrawerOpen.value = true
        docNote.value = `渲染完成，共 ${result.images.length} 页（v${v.version} 版本快照）`
    } catch (e) {
        docNote.value = `渲染失败：${formatApiError(e)}`
    } finally {
        versionRendering.value = false
        versionRenderingTarget.value = null
    }
}

// ---- 流链编辑器三事件（29 票；工票 03 页 flow 编辑）：页槽不可变更新（slot 整体
// 替换触发响应），flow 直接写回该页（off = 无 flow 键 = flow null；quota 交编辑器
// 钳制后的非负整数，null = 不携带）；末尾清段内错误 + dirty 防抖收口 ----

/** flow 段编辑共用收尾：段内错误清空（陈旧错误误导）+ dirty 重算（防抖） */
function afterFlowEdit(): void {
    flowChainError.value = null
    syncDirty()
}

function setPageFlowRole(index: number, role: PageFlowRole): void {
    const slot = pageSlots.value[index]
    if (!slot) return
    const flow =
        role === 'off'
            ? null
            : {
                  mode: role,
                  // quota 仅 fixed 有意义（paged 携带归 null，wire 不输出）；
                  // off→fixed 起步 null（不携带 = 容量型）
                  quota: role === 'fixed' ? (slot.flow?.quota ?? null) : null,
                  omitIfEmpty: slot.flow?.omitIfEmpty ?? false,
              }
    pageSlots.value[index] = { ...slot, flow }
    afterFlowEdit()
}

function setPageFlowQuota(index: number, quota: number | null): void {
    const slot = pageSlots.value[index]
    if (!slot || slot.flow === null || slot.flow.mode !== 'fixed') return
    pageSlots.value[index] = { ...slot, flow: { ...slot.flow, quota } }
    afterFlowEdit()
}

function setPageFlowOmitIfEmpty(index: number, value: boolean): void {
    const slot = pageSlots.value[index]
    if (!slot || slot.flow === null) return
    pageSlots.value[index] = { ...slot, flow: { ...slot.flow, omitIfEmpty: value } }
    afterFlowEdit()
}

// ---- 另存为（spec §4.5，23 票改单调用：副本引用同一数据源实体，不再复制）：
// 弹名字输入 → POST /templates（name + pages + dataSourceId 引用
// 随行）。成功 = 会话延续式重绑：template / dirty 基线 / 抽屉基线整体切到副本后
// router.replace——App.vue 去 :key，本页实例承接参数级导航，不重走载入；「页槽序列
// 重挂」以基线重绑承载（页槽字节 = 发送字节 = 服务端存储字节，重建是恒等操作）；
// dirty 基线 = 本次发送字节（与保存同缝），另存后不改任何东西 saveState 即 clean。
// 绑定是存储态引用，不进 dirty 口径——副本与原模板从落库起即引用同一数据源实体，
// 对实体的后续编辑两模板同享。中途失败回显弹窗内不重绑不导航，重试为全新调用。 ----

const saveAsOpen = ref(false)
const saveAsName = ref('')
const saveAsError = ref<string | null>(null)
const saveAsSaving = ref(false)

function openSaveAs(): void {
    saveAsName.value = templateName.value
    saveAsError.value = null
    saveAsOpen.value = true
}

function closeSaveAs(): void {
    saveAsOpen.value = false
    saveAsError.value = null
}

// 错误随再编辑清空（陈旧错误误导）
watch(saveAsName, () => {
    saveAsError.value = null
})

async function runSaveAs(): Promise<void> {
    const record = template.value
    if (!record || !editor.store.doc || saveAsSaving.value) return
    const name = saveAsName.value.trim()
    if (!name) {
        saveAsError.value = '副本名不能为空'
        return
    }
    if (flowChainLocalErrors.value.length > 0) {
        saveAsError.value = '流链未通过本地校验（在流链编辑器内修复后再另存）'
        return
    }
    saveAsSaving.value = true
    docNote.value = '另存为中…'
    let savedAsId: number | null = null
    try {
        // 与保存同一条缝：当前页快照并入页序列，载荷只消费已快照槽位；
        // dataSourceId = 存储态引用原样随行（单调用，副本即刻可渲染）
        snapshotActivePage()
        const sentName = name
        const sentSlots = pageSlots.value.map((slot) => ({ ...slot }))
        const newRecord = await api.createTemplate({
            ...buildSavePayload({ slots: sentSlots, templateName: sentName }),
            dataSourceId: record.dataSourceId,
        })
        // ---- 成功：会话延续式重绑（spec §4.5）----
        template.value = newRecord
        // dirty 基线 = 本次发送字节（各页 id/name/graph/flow canonical 串），与保存
        // 同缝；另存期间的继续编辑保持 dirty
        savedBaseline = baselineFromPages(sentName, sentSlots)
        templateName.value = sentName
        // 流链编辑器基线：flow 已随 POST 落库 → 段基线同步各页 flow canonical 串
        // （段内标记归灭）；数据源实体未动（引用随行），内容段基线与文本域保持
        flowChainBaselineFlows.value = sentSlots.map((slot) => canonicalPageFlow(slot.flow))
        flowChainError.value = null
        computeDirty()
        saveAsOpen.value = false
        docNote.value = `已另存为「${sentName}」，副本引用同一数据源（即刻可渲染）`
        savedAsId = newRecord.id
    } catch (e) {
        saveAsError.value = formatApiError(e)
        docNote.value = `另存为失败：${formatApiError(e)}`
    } finally {
        saveAsSaving.value = false
    }
    // 路由 replace 在主 try 之外：副本已建、状态已重绑后，导航异常不得反咬
    // 「另存为失败」；失败时路由停在原 id，会话仍指向副本（防御 watcher 以
    // template.id 为准，不受影响）
    if (savedAsId !== null) {
        await router.replace(`/editor/${savedAsId}`).catch(() => {})
    }
}

// ---- 数据源抽屉动作（spec §2.4 数据源段 / §4.3，23 票）：绑定、内容保存、
// 另存为新数据源三条独立通道。校验权威在服务端——schema_invalid /
// dataset_schema_mismatch / data_source_not_found 等错误码原样段内回显，前端只做
// 「能否成 JSON/名字非空」的机械解析（解析失败本地拒绝、不打服务端）；成功后经
// applyBoundSource 同一入口更新基线与补全候选。数据源实体与绑定均不随文档级
// PUT、不进全局 dirty 口径（spec §4.4）。 ----

/** 绑定段动作：按选中项与当前绑定的差值发绑定通道（PUT …/datasource 引用列
 *  整存替换）；绑上后取回实体进内容段（applyBoundSource），解绑回落空态 */
async function runBind(): Promise<void> {
    const record = template.value
    if (!record || bindSaving.value || !isBindingChange(record.dataSourceId, selectedBindId.value)) return
    bindSaving.value = true
    try {
        const updated = await api.bindTemplateDataSource(record.id, selectedBindId.value)
        template.value = updated
        const nextId = updated.dataSourceId
        applyBoundSource(nextId === null ? null : await api.getDataSource(nextId))
        await refreshSources()
        docNote.value = nextId === null ? '已解绑数据源' : `已绑定数据源「${boundSource.value?.name ?? ''}」`
    } catch (e) {
        bindError.value = formatApiError(e)
    } finally {
        bindSaving.value = false
    }
}

/** 内容段列表刷新（templateCount 共享影响面即时可见） */
async function refreshSources(): Promise<void> {
    try {
        sources.value = await api.listDataSources()
    } catch {
        // 列表刷新失败不打断主流程（绑定段错误面已由相应动作负责）
    }
}

/** 内容段保存：已绑 = PUT 实体（共享，影响所有引用模板）；未绑 = 创建并绑定。
 *  载荷组装失败（名字空/JSON 坏）本地拒绝段内回显，不打服务端 */
async function saveContent(): Promise<void> {
    const record = template.value
    if (!record || contentSaving.value) return
    const draft = dataSourceDraftPayload(nameText.value, schemaText.value, dataText.value)
    if (!draft.ok) {
        contentError.value = draft.message
        return
    }
    contentSaving.value = true
    try {
        if (boundSource.value === null) {
            const created = await api.createDataSource(draft.payload)
            const updated = await api.bindTemplateDataSource(record.id, created.id)
            template.value = updated
            applyBoundSource(created)
            await refreshSources()
            docNote.value = `已创建数据源「${created.name}」并绑定`
        } else {
            const saved = await api.updateDataSource(boundSource.value.id, draft.payload)
            applyBoundSource(saved)
            await refreshSources()
            docNote.value = '数据源已保存（共享实体，所有引用模板同享；渲染读取已保存的数据集）'
        }
    } catch (e) {
        contentError.value = formatApiError(e)
    } finally {
        contentSaving.value = false
    }
}

// 另存为新数据源弹窗态（仅已绑可用）：名字草稿/错误/在途态宿主持有
const saveAsNewOpen = ref(false)
const saveAsNewName = ref('')
const saveAsNewError = ref<string | null>(null)
const saveAsNewSaving = ref(false)

function openSaveAsNew(): void {
    if (boundSource.value === null) return
    saveAsNewName.value = `${boundSource.value.name} 副本`
    saveAsNewError.value = null
    saveAsNewOpen.value = true
}

function closeSaveAsNew(): void {
    saveAsNewOpen.value = false
    saveAsNewError.value = null
}

watch(saveAsNewName, () => {
    saveAsNewError.value = null
})

/** 另存为新数据源：以当前内容段草稿 POST 新实体 → 绑定到本模板（copy-on-write，
 *  不影响现共享实体与其他引用模板）；失败弹窗内回显，现绑定不动 */
async function runSaveAsNew(): Promise<void> {
    const record = template.value
    if (!record || saveAsNewSaving.value) return
    const name = saveAsNewName.value.trim()
    if (!name) {
        saveAsNewError.value = '数据源名不能为空'
        return
    }
    const draft = dataSourceDraftPayload(name, schemaText.value, dataText.value)
    if (!draft.ok) {
        saveAsNewError.value = draft.message
        return
    }
    saveAsNewSaving.value = true
    try {
        const created = await api.createDataSource(draft.payload)
        const updated = await api.bindTemplateDataSource(record.id, created.id)
        template.value = updated
        applyBoundSource(created)
        await refreshSources()
        saveAsNewOpen.value = false
        docNote.value = `已另存为新数据源「${created.name}」并绑定`
    } catch (e) {
        saveAsNewError.value = formatApiError(e)
    } finally {
        saveAsNewSaving.value = false
    }
}

// ---- 渲染草稿（spec §4.6，19 票 / template-draft-publish 批 B ④ draft 哨兵，票 03
// 契约修订 / 渲染历史对比升级）：顶栏触发「改了草稿先渲一下看效果」——
// runRenderFlow 促存链（publish.ts runPublishFlow 同款先例）：dirty 自动先走保存
// 流成功后渲草稿列现值，渲的即所见（否则结果对应旧草稿列，用户以为渲的是所改
// 内容——废版成因之一）；clean 直接渲（不强制无谓 PUT）。保存写草稿列不产生版本
// 行，发布仍是显式动作（促存是编排便利，非「渲染即保存」语义合并）。等待期按钮
// 禁用，30s deadline 服务端控制，前端不另设超时；失败错误体（稳定码在前）走状态
// 栏反馈段回显；成功刷新渲染历史并自动打开结果抽屉（RenderResultDrawer 记录切换
// 条支持与历史渲染对比，activeRecordId 指向本次）。与预览导出并存：导出预览图钮
// 保留（文案含「预览图」），抽屉注明「预览仅断行参考，以终图为准」——导出 vs 终图
// 词汇直接做进 UI。按版本渲染（发版轨）：版本历史抽屉行内「渲染」+ 列表页卡片，
// 见 runVersionRender。 ----

const rendering = ref(false)
const renderDrawerOpen = ref(false)
const renderRecords = ref<RenderRecord[]>([])
const activeRenderId = ref<number | null>(null)

/** 渲染历史刷新（渲染历史对比）：失败容忍——退化为仅本次记录（结果抽屉不空），
 *  activeRecordId 恒指向本次记录（用户要看的就是刚渲的这版） */
async function refreshRenderHistory(fallback: RenderRecord): Promise<void> {
    try {
        renderRecords.value = await api.listRenders(fallback.templateId)
    } catch {
        renderRecords.value = [fallback]
    }
    activeRenderId.value = fallback.id
}

/** 记录切换条回写（状态归宿主） */
function onRenderSelect(id: number): void {
    activeRenderId.value = id
}

async function runRender(): Promise<void> {
    const record = template.value
    if (!record || rendering.value) return
    rendering.value = true
    docNote.value = '渲染草稿中…（dirty 自动先保存，渲的即所见；按已发布版本渲染走列表页）'
    try {
        computeDirty()
        const outcome = await runRenderFlow({
            dirty: isDirty.value,
            save: () => runDocumentSave('topbar'),
            render: () => api.renderTemplate(record.id, { version: 'draft' }),
        })
        if (outcome.ok) {
            await refreshRenderHistory(outcome.record)
            renderDrawerOpen.value = true
            docNote.value = `渲染完成，共 ${outcome.record.images.length} 页（草稿${outcome.saved ? '，已先保存未存改动' : ''}）`
        } else if (outcome.note !== null) {
            docNote.value = outcome.note
        }
        // note null = 保存失败已自带错误面（docNote「保存失败：…」），不覆盖肇因
    } finally {
        rendering.value = false
    }
}

// ---- 功能导游宿主编排（feature-tour 03 票，spec §2.4）：触发门 + 顶栏常驻重播
//      入口 + 导游态快捷键门 + 生命周期收尾。宿主只做状态编排——seen 读写/资格/
//      锚点判定在 tour.ts、driver 接线与销毁语义在 tourDriver.ts（01/02 票）。 ----

/** 活动导游句柄：非 null 且 isActive() = 导游进行中。startFeatureTour 的
 *  onFinish/onSkip 与宿主收尾路径都置回 null——02 票销毁语义里宿主主动 destroy 走
 *  discard 路径不回调宿主，清线自理。 */
let tourHandle: TourDriverHandle | null = null

const clearTourHandle = (): void => {
    tourHandle = null
}

function isTourActive(): boolean {
    return tourHandle?.isActive() ?? false
}

/** 既有浮层/抽屉开合 ref 清单（单源）：anyOverlayOpen / closeAllOverlays 共用——
 *  新增浮层只登记此处（spec §4「新增全屏浮层」升级走查项的机械半边）。内核瞬时层
 *  （下拉/帮助面板）不在列——触发时点上无键盘焦点竞争，导游中又被快捷键门拦在
 *  门外。 */
const overlayOpenRefs = [
    drawerOpen,
    flowChainDrawerOpen,
    versionDrawerOpen,
    renderDrawerOpen,
    saveAsOpen,
    saveAsNewOpen,
    revertOpen,
]

/** 既有浮层/抽屉是否开着（触发门条件三） */
function anyOverlayOpen(): boolean {
    return overlayOpenRefs.some((state) => state.value)
}

/** 收起全部既有浮层/抽屉（重播前奏）：导游与浮层互斥的宿主半边（spec §3 走查项
 *  「导游与抽屉/对话框互斥」）——只收开合态，段内草稿/错误态原样保留。 */
function closeAllOverlays(): void {
    for (const state of overlayOpenRefs) state.value = false
}

/** 启动导游（触发门与重播共用出口）：单实例守卫（活动中不重启——driver 新实例
 *  顶替旧实例走 replaced 销毁，语义浑浊）→ startFeatureTour；finish/skip 回调只
 *  清句柄（落 seen 已由 02 票接线在 onDestroyed 单点自理）。 */
function startTour(): void {
    if (isTourActive()) return
    tourHandle = startFeatureTour({ onFinish: clearTourHandle, onSkip: clearTourHandle })
}

/** 宿主侧收尾（卸载/路由离开守卫共用）：destroy 句柄 + 清线。02 票销毁语义：
 *  discard 路径不回调宿主但同样落 seen——弹过即看过，中途路由离开不重播。 */
function teardownTour(): void {
    tourHandle?.destroy()
    tourHandle = null
}

// 触发门（spec §2.4）：台面首挂（template null→record，首载与防御性重载同形）后
// nextTick 单次判定——资格（seen < TOUR_VERSION）+ 轻导首尾锚点在 DOM + 无既有
// 浮层/抽屉打开；判定不过静默不弹、不重试轰炸（spec「数据加载慢下会话再遇」）。
// record→record 替换（保存/绑定/回滚/另存为回写 template）不触发——编辑中途不弹。
watch(template, async (record, prev) => {
    if (record === null || prev !== null) return
    await nextTick()
    if (!shouldShowTour(readSeenVersion()) || !tourEndpointsReady(TOUR_STEPS, document) || anyOverlayOpen()) {
        return
    }
    startTour()
})

/** 重播入口（顶栏 data-tour-replay 钮，spec §2.4 常驻）：无视 seen 直接重播；
 *  有既有浮层先收起再开。导游中按钮不可达（遮罩收点击），守卫只防路径外触发。 */
function replayTour(): void {
    if (isTourActive()) return
    closeAllOverlays()
    startTour()
}

/** 导游态快捷键门（spec §2.3「导游进行中抑制宿主快捷键」，Esc 归导游）：导游激活
 *  期间在 window 捕获段拦 keydown——内核 useShortcuts / CanvasSurface 的 Esc /
 *  帮助面板与宿主保存快捷键全在 window 冒泡段，捕获先于冒泡、与注册序无关；
 *  preventDefault 防焦点残留在输入框时导游中继续打字。放行面（导游自身 UI 的
 *  键盘操作，非宿主快捷键）：① Tab 无条件放行——driver 焦点圈（popover ⇄ 高亮
 *  元素循环）在 keydown 收 Tab；内核注册表同键绑循环选层为已知共存副作用（纯
 *  选中态、零数据变更，导游结束即无），换取焦点圈可用。② Enter/Space 仅在
 *  popover 内放行（TOUR_POPOVER_CONTENT_SELECTOR 判定）——焦点按钮激活的合成
 *  click 前提；内核注册表不绑这两键，放行无宿主副作用。popover 外拦下：防焦点
 *  落在高亮宿主元素时按键激活宿主 UI（disableActiveInteraction 的键盘半边）。
 *  其余 keydown 全拦：Esc 的 keydown 也拦（CanvasSurface 的 Esc 语义不越权），
 *  driver 的 Esc/方向键收在 keyup（keyup 无门，天然放行）。句柄销毁即门失效
 *  （isActive 读数）。 */
function onTourKeydownGate(event: KeyboardEvent): void {
    if (!isTourActive()) return
    if (event.key === 'Tab') return
    const target = event.target
    if (
        (event.key === 'Enter' || event.key === ' ') &&
        target instanceof Element &&
        target.closest(TOUR_POPOVER_CONTENT_SELECTOR) !== null
    ) {
        return
    }
    event.preventDefault()
    event.stopPropagation()
}
window.addEventListener('keydown', onTourKeydownGate, true)

/** 输入法合成中的按键不触发保存 */
function isImeComposing(event: KeyboardEvent): boolean {
    return event.isComposing || event.keyCode === 229
}

/** 键盘：仅 Ctrl/Cmd+S（保存是宿主职责，不入内核注册表）。文本编辑中键盘路由
 *  进 textarea，保存不抢；其余快捷键已由 useShortcuts 统一处理。 */
function onKeydown(event: KeyboardEvent): void {
    if (isTourActive()) return // 导游态门（03 票）：宿主快捷键 early-return，Esc 归导游（捕获段门已拦，此为语义兜底）
    if (saveAsOpen.value || saveAsNewOpen.value || revertOpen.value) return // 名字/确认弹窗模态持有键盘（Enter/Esc 由弹窗自理）
    if (editor.store.ui.editing !== null) return
    const mod = event.ctrlKey || event.metaKey
    if (mod && !event.shiftKey && event.key.toLowerCase() === 's') {
        if (isImeComposing(event)) return
        event.preventDefault()
        void saveTemplate()
    }
}
window.addEventListener('keydown', onKeydown)

/** 关闭守卫双保险之一：路由离开 confirm（spec §4.4）——同步即时比较。工程数据
 *  静默 flush（工单 05 时点三之一）随钩子无条件先发：不进 confirm 文案、不拦
 *  导航（flush 后照常放行/询问）；内容 dirty 的 confirm 语义原样（状态不计 dirty，
 *  不因状态弹窗）。editor→editor 参数级导航（另存为 replace）不走本钩子（vue-router
 *  同路由参数变化触发 update 不触发 leave）——副本首写落下一真实 flush 时点。 */
onBeforeRouteLeave(() => {
    flushEditorState()
    computeDirty()
    if (isDirty.value && !window.confirm('当前有未保存的变更，离开将丢失。确定离开吗？')) return false
    // 导航成立才 destroy 导游句柄（03 票「路由离开守卫中 destroy」）——confirm 取消
    // = 留在编辑器，进行中的导游不惊扰；discard 路径自身落 seen（弹过即看过）
    teardownTour()
    return true
})

/** 双保险之二：页签关闭 beforeunload（文案由浏览器定）。工程数据 flush（工单 05
 *  时点三之二）随钩子无条件先发（keepalive PUT，unload 期存活）；内容 dirty 的
 *  浏览器确认行为原样（状态不计 dirty，不因状态弹）。 */
function onBeforeUnload(event: BeforeUnloadEvent): void {
    flushEditorStateKeepalive()
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
    // 非法字符折叠与终图下载建议名同口径（render.ts sanitizeFileBase 单源）
    const base = sanitizeFileBase(templateName.value)
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
    window.removeEventListener('keydown', onTourKeydownGate, true)
    teardownTour() // 卸载中 destroy 导游句柄（03 票），防 overlay 残留
    window.removeEventListener('beforeunload', onBeforeUnload)
    if (dirtyTimer !== null) clearTimeout(dirtyTimer)
    unsubscribeAssets?.()
    unsubscribeDoc?.()
    unsubscribeToolbarReads()
    unsubscribeFontCatalog()
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
             保存态｜保存 / 发布 + 版本徽标（tdp 批 B ②）/ 版本（批 B ③）/ 另存为
             （18 票）/ 渲染终图（19 票，draft 哨兵）/ 导出预览图（预览与终图并存）/
             功能导游重播钮（feature-tour 03 票，常驻入口） -->
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
                    title="数据源抽屉：绑定独立数据源实体、编辑其 schema/data（各自独立保存通道）"
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
                <!-- 发布（template-draft-publish 批 B ②，spec §6）：把草稿固化为新
                     版本——dirty 时自动先保存再发布（一次动作）；草稿与最新版本字节
                     相等服务端幂等跳过（duplicate:true 也算成功）。徽标 = 版本史
                     推导（未发布/vN），发布/回滚后刷新 -->
                <button
                    type="button"
                    class="ghost"
                    data-publish-template
                    title="发布：把当前草稿固化为新发布版本（dirty 时自动先保存再发布，一次动作；内容与最新版本一致则跳过重复发布）"
                    :disabled="publishing || saving"
                    @click="runPublish"
                >
                    {{ publishing ? '发布中…' : '发布' }}
                </button>
                <span v-if="versionBadgeLabel" class="version-badge" data-version-badge title="版本徽标：最新已发布版本（「未发布」= 空版本史；发布/回滚后自动刷新）">
                    {{ versionBadgeLabel }}
                </span>
                <!-- 版本历史（template-draft-publish 批 B ③，spec §6）：开合抽屉——
                     版本号 + 发布时间 + 行内回滚（ConfirmDialog 二次确认） -->
                <button
                    type="button"
                    class="ghost"
                    data-open-versions
                    title="版本历史：已发布版本列表与回滚入口"
                    @click="versionDrawerOpen = !versionDrawerOpen"
                >
                    版本
                </button>
                <button
                    type="button"
                    class="ghost"
                    data-save-as-template
                    title="另存为：以当前内容创建副本模板（各页 + 流链 + 数据源随行），成功后跳转到副本继续编辑"
                    :disabled="saveAsSaving"
                    @click="openSaveAs"
                >
                    另存为
                </button>
                <!-- 渲染草稿（spec §4.6，19 票 / 批 B ④ draft 哨兵 / 渲染历史对比）：
                     dirty 自动先保存再渲（渲的即所见），{version:"draft"} 渲草稿列现值；
                     等待期禁用（30s deadline 服务端控制，前端不另设超时） -->
                <button
                    type="button"
                    class="ghost"
                    data-render-final
                    title="渲染草稿：先保存未存改动，再渲染当前草稿看效果（发布不被预览倒逼；按已发布版本渲染走列表页或版本历史行内「渲染」）；服务端 30s 上限，完成自动打开结果抽屉（可与历史渲染对比）"
                    :disabled="rendering"
                    @click="runRender"
                >
                    {{ rendering ? '渲染中…' : '渲染草稿' }}
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
                <!-- 功能导游常驻重播入口（feature-tour 03 票，spec §2.4）：独立 ? 图标
                     钮，点击无视 seen 直接重播；有既有浮层先收起再开导游（replayTour） -->
                <button
                    type="button"
                    class="ghost tour-replay"
                    data-tour-replay
                    title="功能导游：重看编辑器引导（首访轻导，约 1 分钟）"
                    aria-label="功能导游"
                    @click="replayTour"
                >
                    ?
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
                title="插入图层（四类层型统一画拉：点击后在画布拖拽定落位与尺寸，Esc 取消；模板表经表单填数据行路径后画拉）"
                @closed="onInsertMenuClosed"
            >
                <template #body="{ closeMenu }">
                    <TemplateTableForm
                        v-if="insertTemplateFormOpen"
                        :editor="editor"
                        @confirm="(rowsPath) => onInsertTemplateConfirm(rowsPath, closeMenu)"
                        @cancel="() => onInsertTemplateCancel(closeMenu)"
                    />
                </template>
            </DropdownMenu>
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
            <!-- canvas-area：页 tab 条（顶）+ canvas-holder（画布覆盖物定位上下文） -->
            <div class="canvas-area">
                <!-- 页 tab 条（spec 页形状契约；16 票 / 26 票增页 / 27 票删页 / 29 票流链
                     入口沿用）：画布上方；单击切页（页间切换不提示），双击重命名（改
                     page.name，进 dirty）；tab key = 页 id（工票 03——改名/删页/重排后
                     行身份稳定，数组下标 key 退役）；尾部「＋」钮追加空白页并切为活动页
                     （普通 button 非 tab 角色——它不是页签）；tab hover「✕」删页
                     （confirm 前置、flow 随页移除，唯一页不出现）；链上页 tab 带角标；
                     最右「流链」钮开合流链编辑器；不做中插/重排/复制（UI 另立工单） -->
                <div class="page-tabs" role="tablist" aria-label="文档页">
                    <div
                        v-for="(slot, i) in pageSlots"
                        :key="slot.id"
                        class="page-tab"
                        :class="{ 'is-active': i === activePage, 'is-on-chain': onChainPages.has(slot.id) }"
                        role="tab"
                        :aria-selected="i === activePage"
                        :title="`第 ${i + 1} 页：${slot.name || '（未命名）'}${onChainPages.has(slot.id) ? '（流链）' : ''}`"
                        :data-page-tab="i"
                        @click="switchToPage(i)"
                        @dblclick="beginPageRename(i)"
                    >
                        <input
                            v-if="renamingPage === i"
                            :ref="setRenameInputEl"
                            v-model="renamingValue"
                            class="page-tab-input"
                            data-page-rename-input
                            aria-label="页名"
                            spellcheck="false"
                            @click.stop
                            @dblclick.stop
                            @keydown.enter.prevent="commitPageRename"
                            @keydown.esc.prevent="cancelPageRename"
                            @blur="commitPageRename"
                        />
                        <template v-else>
                            <span class="page-tab-label" :data-page-tab-label="i">
                                {{ slot.name || `第 ${i + 1} 页` }}
                            </span>
                            <!-- 删页（27 票）：hover 现身（visibility 不挤占布局）、唯一页
                                 不出现；@click.stop 不触发切页 -->
                            <button
                                v-if="pageSlots.length > 1"
                                type="button"
                                class="page-tab-delete"
                                data-page-delete
                                :aria-label="`删除第 ${i + 1} 页`"
                                :title="DELETE_PAGE_TITLE"
                                @click.stop="confirmDeletePage(i)"
                                @dblclick.stop
                            >
                                ✕
                            </button>
                        </template>
                    </div>
                    <button
                        type="button"
                        class="page-add"
                        data-page-add
                        aria-label="新增页"
                        title="新增页"
                        @click="appendBlankPage"
                    >
                        ＋
                    </button>
                    <!-- 流链入口（29 票，工票 03 页 flow 编辑）：flow 附着于页对象，
                         入口贴页序列；徽标 = 链上页数（空链不显示） -->
                    <button
                        type="button"
                        class="page-chain"
                        data-open-flowchain
                        aria-label="流链"
                        title="流链（行流分配）：声明各页如何消费数据行"
                        @click="flowChainDrawerOpen = !flowChainDrawerOpen"
                    >
                        流链<span v-if="onChainPages.size > 0" class="page-chain-badge" data-flowchain-badge>{{ onChainPages.size }}</span>
                    </button>
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

        <!-- 数据源抽屉（spec §4.3 两段式，23 票重构 / 29 票流链段迁出）：绑定/内容
             两段，文本域草稿与段内错误/标记状态全由宿主持有，组件纯呈现；段内独立
             标记与全局保存态指示灯分离 -->
        <DataSourceDrawer
            v-model:selected-bind-id="selectedBindId"
            v-model:name-text="nameText"
            v-model:schema-text="schemaText"
            v-model:data-text="dataText"
            :open="drawerOpen"
            :bound-id="template?.dataSourceId ?? null"
            :bound-name="boundSource?.name ?? null"
            :sources="sources"
            :bind-saving="bindSaving"
            :bind-error="bindError"
            :content-dirty="contentDirty"
            :content-saving="contentSaving"
            :content-error="contentError"
            :schema-rejected="schemaRejected"
            @close="drawerOpen = false"
            @bind="runBind"
            @save-content="saveContent"
            @save-as-new="openSaveAsNew"
        />

        <!-- 流链编辑器（29 票引入，工票 03 改页 flow 编辑）：页 tab 条「流链」钮开合；
             结构化页列表纯呈现，页序列（flow 真值源）/本地校验/服务端错误/独立未保存
             标记全由宿主持有；保存流链走文档级 PUT 同一通道（各页一并整存） -->
        <FlowChainDrawer
            :open="flowChainDrawerOpen"
            :rows="flowChainPages"
            :errors="flowChainLocalErrors"
            :error="flowChainError"
            :dirty="flowChainSegmentDirty"
            :saving="saving"
            @close="flowChainDrawerOpen = false"
            @save="saveFlowChain"
            @set-role="setPageFlowRole"
            @set-quota="setPageFlowQuota"
            @set-omit-if-empty="setPageFlowOmitIfEmpty"
        />

        <!-- 另存为弹窗（spec §4.5 单调用随行引用，18 票组件、23 票文案修订）：
             名字草稿与错误/在途态全由宿主持有，组件纯呈现；确认 → 单调用 POST +
             会话延续式重绑 + router.replace -->
        <NamePromptDialog
            v-model:name="saveAsName"
            :open="saveAsOpen"
            :saving="saveAsSaving"
            :error="saveAsError"
            title="另存为副本"
            label="副本名"
            note="以当前内容（各页 + 流链 + 数据源引用）创建副本模板，副本与原模板引用同一数据源实体；成功后跳转到副本继续编辑，原模板保持不变。"
            confirm-text="另存为"
            confirm-busy-text="另存中…"
            @confirm="runSaveAs"
            @cancel="closeSaveAs"
        />

        <!-- 另存为新数据源弹窗（23 票 copy-on-write）：以内容段草稿 POST 新数据源
             实体并重绑本模板，不影响现共享实体与其他引用模板 -->
        <NamePromptDialog
            v-model:name="saveAsNewName"
            :open="saveAsNewOpen"
            :saving="saveAsNewSaving"
            :error="saveAsNewError"
            title="另存为新数据源"
            label="数据源名"
            note="以当前草稿内容创建新的数据源实体并绑定到本模板；现有数据源与其他引用它的模板不受影响。"
            confirm-text="创建并绑定"
            confirm-busy-text="创建中…"
            @confirm="runSaveAsNew"
            @cancel="closeSaveAsNew"
        />

        <!-- 版本历史抽屉（template-draft-publish 批 B ③，spec §6）：版本列表纯呈现
             （null = 未知态显「版本列表不可用」，评审修复 F1——未知 ≠ 未发布），
             回滚确认/在途/错误全由宿主持有；行内回滚 → ConfirmDialog → 会话替换；
             行内渲染 → runVersionRender（版本快照渲染，进渲染历史对比） -->
        <VersionHistoryDrawer
            :open="versionDrawerOpen"
            :versions="versions"
            :reverting="reverting"
            :reverting-version="revertTarget?.version ?? null"
            :rendering="versionRendering"
            :rendering-version="versionRenderingTarget"
            @close="versionDrawerOpen = false"
            @revert="openRevert"
            @render="runVersionRender"
        />

        <!-- 回滚确认框（spec §6 定死文案；服务端零守卫，确认交互在前端——票 04）：
             错误留在框内可重试/取消，会话替换只在确认后发生 -->
        <ConfirmDialog
            :open="revertOpen"
            :busy="reverting"
            :error="revertError"
            danger
            title="回滚到历史版本"
            :note="revertNote"
            confirm-text="回滚"
            confirm-busy-text="回滚中…"
            @confirm="runRevert"
            @cancel="closeRevert"
        />

        <!-- 渲染结果抽屉（spec §4.6，19 票 + 渲染历史对比）：渲染历史记录/当前记录/
             开合/模板名（下载建议名）由宿主持有，缩略图网格直用 images[].url；成功
             自动打开、关闭钮回落；切换条在历史记录间对比查看 -->
        <RenderResultDrawer
            :open="renderDrawerOpen"
            :records="renderRecords"
            :active-record-id="activeRenderId"
            :template-name="templateName"
            @select="onRenderSelect"
            @close="renderDrawerOpen = false"
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

/* 功能导游重播钮（feature-tour 03 票，spec §2.4）：顶栏独立 ? 图标钮——正方形
   紧凑形态区别于文字 ghost 钮；常驻语义（无 seen 门、无 v-if，锁「常驻 + 点击
   重播」） */
.topbar .tour-replay {
    width: 28px;
    padding: 5px 0;
    font-weight: 600;
    text-align: center;
}

/* 版本徽标（template-draft-publish 批 B ②）：轻量 pill 同行随发布钮，「未发布」
   与「vN」同款中性形态不闪色，语义靠文本；拉取失败不渲染（v-if） */
.version-badge {
    padding: 3px 9px;
    border: 1px solid var(--shell-line-strong);
    border-radius: 999px;
    background: var(--shell-panel);
    color: var(--shell-fg-2);
    font-size: 12px;
    line-height: 1.4;
    white-space: nowrap;
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

/* 画布容器 = 页 tab 条（顶）+ canvas-holder（覆盖物定位上下文）的纵向排布 */
.canvas-area {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-width: 0;
}

/* 页 tab 条（spec 页形状契约；16 票多帧引入）：画布上方窄条；活动 tab 面板色高亮；
   多页横向内滚降级（同工具栏姿势） */
.page-tabs {
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

.page-tabs::-webkit-scrollbar {
    display: none;
}

.page-tab {
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

.page-tab:hover {
    background: var(--shell-hover);
    color: var(--shell-fg-2);
}

.page-tab.is-active {
    border-color: var(--shell-line);
    background: var(--shell-panel);
    color: var(--shell-fg);
}

.page-tab-label {
    overflow: hidden;
    text-overflow: ellipsis;
}

/* 页 tab 删除钮（27 票删页）：hover 现身（visibility 不挤占布局、tab 宽不抖动），
   唯一页由 v-if 收起；红系 hover 提示不可恢复 */
.page-tab-delete {
    flex: none;
    box-sizing: border-box;
    width: 16px;
    height: 16px;
    margin-left: 4px;
    padding: 0;
    border: none;
    border-radius: 4px;
    background: transparent;
    font-size: 10px;
    line-height: 1;
    color: var(--shell-fg-3);
    cursor: pointer;
    visibility: hidden;
}

.page-tab:hover .page-tab-delete,
.page-tab-delete:focus-visible {
    visibility: visible;
}

.page-tab-delete:hover {
    background: var(--shell-hover);
    color: #f87171;
}

.page-tab-input {
    width: 96px;
    padding: 1px 4px;
    border: 1px solid var(--shell-line-strong);
    border-radius: 4px;
    background: var(--shell-bg);
    font-size: 12px;
    color: var(--shell-fg);
    outline: none;
}

/* 页尾「＋」钮（26 票增页）：tab 条尾部普通按钮（非 tab 角色），虚线框示意追加位 */
.page-add {
    flex: none;
    box-sizing: border-box;
    width: 26px;
    height: 26px;
    margin-left: 4px;
    padding: 0;
    border: 1px dashed var(--shell-line-strong);
    border-radius: 6px;
    background: transparent;
    font-size: 14px;
    line-height: 1;
    color: var(--shell-fg-3);
    cursor: pointer;
}

.page-add:hover {
    border-color: var(--shell-accent);
    color: var(--shell-accent);
}

.page-add:focus-visible {
    border-color: var(--shell-accent);
    outline: 1px solid var(--shell-accent);
}

/* 链上页角标（29 票）：页名左侧小圆点，呼应流链编辑器的行角色 */
.page-tab.is-on-chain .page-tab-label::before {
    content: '';
    display: inline-block;
    width: 5px;
    height: 5px;
    margin-right: 5px;
    border-radius: 50%;
    background: var(--shell-accent);
    vertical-align: 2px;
}

/* 页尾「流链」钮（29 票流链编辑器入口）：与「＋」同排的实边框钮，徽标显链上页数 */
.page-chain {
    flex: none;
    box-sizing: border-box;
    height: 26px;
    margin-left: 6px;
    padding: 0 10px;
    border: 1px solid var(--shell-line-strong);
    border-radius: 6px;
    background: transparent;
    font-size: 12px;
    line-height: 1;
    color: var(--shell-fg-3);
    cursor: pointer;
}

.page-chain:hover {
    border-color: var(--shell-accent);
    color: var(--shell-accent);
}

.page-chain:focus-visible {
    border-color: var(--shell-accent);
    outline: 1px solid var(--shell-accent);
}

.page-chain-badge {
    display: inline-block;
    min-width: 14px;
    margin-left: 5px;
    padding: 1px 4px;
    border-radius: 7px;
    background: var(--shell-accent);
    color: var(--shell-on-accent);
    font-size: 10px;
    font-weight: 600;
    line-height: 1.2;
    text-align: center;
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
