<script setup lang="ts">
// 数据源新建/编辑页（25 票，spec §4.1 /datasources/new 与 /datasources/:id 行）：
// 数据源独立管理面的落地形态——name/schema/data 三输入（编辑器抽屉内容段同款
// 文本域与标签），保存 = POST /datasources（new 态，成功 router.replace 到
// /datasources/{id} 就地转编辑态，已建记录判同跳过重载——EditorPage 另存为同款
// 会话延续）或 PUT /datasources/{id}（编辑态；共享实体，注记引用模板数）。
// 校验边界照旧：draft-07 权威在服务端，前端只做「名字非空/文本能成 JSON」的机械
// 解析（复用 editor/datasource.ts 纯逻辑），schema_invalid /
// dataset_schema_mismatch / data_source_not_found 稳定码在页面回显。
// 脏内容离开守卫双保险（spec §4.4 同哲学）：onBeforeRouteLeave confirm +
// beforeunload；数据源实体不属模板文档 dirty 口径，本页守卫只看本页草稿。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'

import {
    api,
    ApiError,
    formatApiError,
    type DataSourceRecord,
} from '../api'
import {
    dataSourceDraftPayload,
    isSegmentDirty,
    sourceDraftBaseline,
} from '../editor/datasource'

const route = useRoute()
const router = useRouter()

// new 与 :id 同一条路由记录：id === 'new' 即新建态（router.ts 注：同记录让创建
// 后的 replace 成为参数变化，不触发下方离开守卫）
const isNew = computed(() => route.params.id === 'new')

const source = ref<DataSourceRecord | null>(null)
const notFound = ref(false)
const errorText = ref<string | null>(null)

// 三文本域草稿 + 载入基线（spec §4.4 段内独立未保存标记哲学；new 态基线 = 空三联）。
// 基线必须持 ref：保存/创建成功后三文本域被设回与当前相同的字符串（Vue ref 对
// Object.is 相等值跳过触发），只有基线 ref 的整体换新能驱动 contentDirty 重算归
// false——普通 let 基线不可追踪，未保存标记会永久卡在 true（EditorPage
// contentBaseline 同款模式）
const nameText = ref('')
const schemaText = ref('')
const dataText = ref('')
const baseline = ref(sourceDraftBaseline(null))

const contentDirty = computed(
    () =>
        isSegmentDirty(nameText.value, baseline.value.nameText) ||
        isSegmentDirty(schemaText.value, baseline.value.schemaText) ||
        isSegmentDirty(dataText.value, baseline.value.dataText),
)

const saving = ref(false)
const saveError = ref<string | null>(null)

// 共享影响面计数（来自 GET /datasources 摘要；取不到不打断编辑，注记降级无计数）
const templateCount = ref<number | null>(null)

/** 载入序号：异步载入期间路由再变（连续导航）时丢弃过期结果 */
let loadSeq = 0

function applyRecord(record: DataSourceRecord): void {
    source.value = record
    baseline.value = sourceDraftBaseline(record)
    nameText.value = baseline.value.nameText
    schemaText.value = baseline.value.schemaText
    dataText.value = baseline.value.dataText
    saveError.value = null
}

function resetNewDraft(): void {
    source.value = null
    notFound.value = false
    errorText.value = null
    saveError.value = null
    baseline.value = sourceDraftBaseline(null)
    nameText.value = ''
    schemaText.value = ''
    dataText.value = ''
    templateCount.value = null
}

async function refreshCount(): Promise<void> {
    try {
        const summaries = await api.listDataSources()
        templateCount.value = summaries.find((s) => s.id === source.value?.id)?.templateCount ?? null
    } catch {
        // 影响面计数失败只降级注记，不影响编辑主流程
    }
}

async function loadSource(id: string): Promise<void> {
    const seq = ++loadSeq
    try {
        const record = await api.getDataSource(id)
        if (seq !== loadSeq) return
        applyRecord(record)
        void refreshCount()
    } catch (e) {
        if (seq !== loadSeq) return
        if (e instanceof ApiError && e.code === 'data_source_not_found') {
            notFound.value = true
        } else {
            errorText.value = formatApiError(e)
        }
    }
}

onMounted(() => {
    if (isNew.value) resetNewDraft()
    else void loadSource(String(route.params.id))
})

// 路由防御性重载（App.vue 无 :key 的绑定保证承接，EditorPage 同款）：new → edit
// 的 replace 已先 applyRecord（判同跳过重载，会话延续）；其余 id 脱钩变化整段重载；
// id === 'new'（进新建态）由 isNew watcher 重置，此处跳过
watch(
    () => route.params.id,
    (id) => {
        if (route.name !== 'datasource-edit' || id === 'new') return
        if (source.value !== null && String(source.value.id) === String(id)) return
        notFound.value = false
        errorText.value = null
        saveError.value = null
        source.value = null
        void loadSource(String(id))
    },
)

watch(isNew, (v) => {
    if (v) resetNewDraft()
})

/** 保存：载荷组装失败（名字空/JSON 坏）本地拒绝不打服务端；new 态创建后 replace
 *  进编辑态；编辑态整存替换共享实体 */
async function runSave(): Promise<void> {
    if (saving.value) return
    const draft = dataSourceDraftPayload(nameText.value, schemaText.value, dataText.value)
    if (!draft.ok) {
        saveError.value = draft.message
        return
    }
    saving.value = true
    saveError.value = null
    try {
        if (isNew.value) {
            const created = await api.createDataSource(draft.payload)
            applyRecord(created)
            templateCount.value = 0
            await router.replace(`/datasources/${created.id}`)
        } else if (source.value !== null) {
            const saved = await api.updateDataSource(source.value.id, draft.payload)
            applyRecord(saved)
        }
    } catch (e) {
        saveError.value = formatApiError(e)
    } finally {
        saving.value = false
    }
}

/** 关闭守卫双保险之一：路由离开 confirm（spec §4.4 同哲学）——同步即时比较 */
onBeforeRouteLeave(() => {
    if (!contentDirty.value) return true
    return window.confirm('当前数据源有未保存的变更，离开将丢失。确定离开吗？')
})

/** 双保险之二：页签关闭 beforeunload（文案由浏览器定） */
function onBeforeUnload(event: BeforeUnloadEvent): void {
    if (!contentDirty.value) return
    event.preventDefault()
    event.returnValue = ''
}
window.addEventListener('beforeunload', onBeforeUnload)

onBeforeUnmount(() => {
    window.removeEventListener('beforeunload', onBeforeUnload)
})
</script>

<template>
    <main class="pc-atelier relative min-h-screen text-zinc-200">
        <div class="relative z-10 mx-auto max-w-3xl px-6 pb-16 pt-14">
            <RouterLink
                to="/datasources"
                class="pc-rise mb-8 inline-flex items-center gap-1.5 rounded-lg border border-[#223252] px-3 py-1.5 font-mono text-xs text-zinc-400 transition-colors hover:border-sky-700 hover:text-sky-300"
            >
                ← 返回数据源列表
            </RouterLink>

            <!-- 页头：金杠 kicker + 衬线大标题 + 实体注记 -->
            <header class="pc-rise" style="animation-delay: 60ms">
                <p class="pc-kicker mb-4">Data Source · {{ isNew ? 'NEW' : 'EDIT' }}</p>
                <h1 class="pc-display mb-2 text-3xl leading-tight text-zinc-50">
                    {{ isNew ? '新建数据源' : '编辑数据源' }}
                </h1>
                <p class="text-sm leading-6 text-zinc-500" data-source-meta>
                    <template v-if="isNew">
                        创建为独立数据源实体；创建后可在模板编辑器的数据源抽屉绑定使用。
                    </template>
                    <template v-else-if="source">
                        ID {{ source.id }}<template v-if="templateCount !== null"> · 当前 {{ templateCount }} 个模板引用</template>。保存写入实体本身——所有引用它的模板同享（渲染始终读取已保存版本）。
                    </template>
                </p>
                <div class="mt-7 h-px bg-gradient-to-r from-sky-400/50 via-sky-400/10 to-transparent" aria-hidden="true"></div>
            </header>

            <!-- 404（data_source_not_found：id 不存在或非整数，spec §2.4 通用寻址） -->
            <div
                v-if="notFound"
                data-datasource-not-found
                class="pc-rise mt-6 flex items-start gap-2.5 rounded-xl border border-red-900/60 bg-red-950/40 px-4 py-3 text-sm leading-6 text-red-300"
            >
                <span class="mt-0.5 font-mono text-xs text-red-400/90" aria-hidden="true">✕</span>
                <span>
                    数据源不存在或已被删除（data_source_not_found）。
                    <RouterLink to="/datasources" class="text-sky-400 hover:text-sky-300">返回数据源列表</RouterLink>
                </span>
            </div>

            <!-- 读取失败态（服务端未起等） -->
            <div
                v-else-if="errorText"
                class="pc-rise mt-6 flex items-start gap-2.5 rounded-xl border border-red-900/60 bg-red-950/40 px-4 py-3 text-sm leading-6 text-red-300"
            >
                <span class="mt-0.5 font-mono text-xs text-red-400/90" aria-hidden="true">✕</span>
                <span>数据源读取失败：{{ errorText }}</span>
            </div>

            <!-- 表单（new 态或载入完成后）：编号分段的三输入（01 名称 / 02 SCHEMA /
                 03 DATA），draft-07 权威校验在服务端、前端只做机械解析 -->
            <div v-else-if="isNew || source !== null" class="pc-card pc-rise mt-6 overflow-hidden rounded-xl" style="animation-delay: 120ms">
                <div class="px-6 py-7 sm:px-8">
                    <section>
                        <div class="mb-3 flex items-baseline gap-3">
                            <span class="font-mono text-[11px] tracking-[0.2em] text-[#e3c37f]/85" aria-hidden="true">01</span>
                            <label class="text-sm font-semibold text-zinc-200" for="ds-name">数据源名</label>
                        </div>
                        <input
                            id="ds-name"
                            v-model="nameText"
                            data-source-name-input
                            class="pc-field pc-field--code"
                            spellcheck="false"
                        />
                        <p class="mt-2 text-xs leading-5 text-zinc-600">独立实体的显示名；模板按引用绑定，改名不影响既有绑定关系。</p>
                    </section>

                    <div class="my-7 h-px bg-[#16233c]" aria-hidden="true"></div>

                    <section>
                        <div class="mb-3 flex items-center gap-3">
                            <span class="font-mono text-[11px] tracking-[0.2em] text-[#e3c37f]/85" aria-hidden="true">02</span>
                            <label class="text-sm font-semibold text-zinc-200" for="ds-schema">schema（draft-07 JSON）</label>
                            <span class="rounded border border-[#223252] px-1.5 py-0.5 font-mono text-[10px] tracking-wider text-zinc-500">JSON</span>
                        </div>
                        <textarea
                            id="ds-schema"
                            v-model="schemaText"
                            data-schema-input
                            class="pc-field pc-field--code"
                            rows="7"
                            spellcheck="false"
                        ></textarea>
                        <p class="mt-2 text-xs leading-5 text-zinc-600">draft-07 权威校验在服务端；前端只做「文本能否解析为 JSON」的机械检查。</p>
                    </section>

                    <div class="my-7 h-px bg-[#16233c]" aria-hidden="true"></div>

                    <section>
                        <div class="mb-3 flex items-center gap-3">
                            <span class="font-mono text-[11px] tracking-[0.2em] text-[#e3c37f]/85" aria-hidden="true">03</span>
                            <label class="text-sm font-semibold text-zinc-200" for="ds-data">data（JSON）</label>
                            <span class="rounded border border-[#223252] px-1.5 py-0.5 font-mono text-[10px] tracking-wider text-zinc-500">JSON</span>
                        </div>
                        <textarea
                            id="ds-data"
                            v-model="dataText"
                            data-dataset-input
                            class="pc-field pc-field--code"
                            rows="10"
                            spellcheck="false"
                        ></textarea>
                        <p class="mt-2 text-xs leading-5 text-zinc-600">渲染时注入画布的数据集；保存写入实体本身，渲染始终读取已保存版本。</p>
                    </section>

                    <p v-if="saveError" data-dataset-error class="mt-5 flex items-start gap-2.5 rounded-lg border border-red-900/60 bg-red-950/40 px-3.5 py-2.5 text-sm leading-6 text-red-300">
                        <span class="mt-0.5 font-mono text-xs text-red-400/90" aria-hidden="true">✕</span>
                        <span>{{ saveError }}</span>
                    </p>

                    <div class="mt-7 flex items-center justify-end gap-3 border-t border-[#16233c] pt-5">
                        <span v-if="contentDirty" data-dataset-unsaved class="animate-pulse font-mono text-xs text-amber-400">● 未保存</span>
                        <button
                            type="button"
                            data-save-datasource
                            class="rounded-lg bg-gradient-to-b from-sky-400 to-sky-500 px-4 py-2 text-sm font-semibold text-[#06202b] shadow-[0_0_0_1px_rgba(56,189,248,0.35),0_8px_24px_rgba(56,189,248,0.18)] transition hover:brightness-110 active:translate-y-px disabled:cursor-not-allowed disabled:opacity-50"
                            :disabled="saving"
                            @click="runSave"
                        >
                            {{ saving ? (isNew ? '创建中…' : '保存中…') : isNew ? '创建数据源' : '保存数据源' }}
                        </button>
                    </div>
                </div>
            </div>

            <!-- 页脚铭线 -->
            <footer class="mt-16 border-t border-[#16233c] pt-5">
                <p class="font-mono text-[11px] tracking-[0.25em] text-zinc-600">CANVAS NEXT · 证书批量生成示例</p>
            </footer>
        </div>
    </main>
</template>

<style scoped>
/* 三输入的统一字段面：深底 + 青辉 focus（编辑器令牌同族的青色） */
.pc-field {
    box-sizing: border-box;
    display: block;
    width: 100%;
    padding: 10px 12px;
    border: 1px solid #223252;
    border-radius: 10px;
    background: #0a1322;
    color: #e6edf7;
    resize: vertical;
    transition:
        border-color 0.2s,
        box-shadow 0.2s;
}

.pc-field:focus {
    border-color: rgba(56, 189, 248, 0.65);
    box-shadow: 0 0 0 3px rgba(56, 189, 248, 0.14);
    outline: none;
}

.pc-field--code {
    font-family: var(--pc-font-mono);
    font-size: 12px;
    line-height: 1.7;
}
</style>
