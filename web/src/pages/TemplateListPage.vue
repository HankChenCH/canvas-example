<script setup lang="ts">
// 模板列表（spec §4.1 / 14 票）：纯目录页——卡片仅 name + updatedAt
// （GET /api/templates 摘要恰这三字段），无新建/删除/直渲染；首访重定向由
// router 守卫处理，本页只负责列表展示与卡片换模板入口。
import { onMounted, ref } from 'vue'

import { api, formatApiError, type TemplateSummary } from '../api'

const templates = ref<TemplateSummary[] | null>(null)
const errorText = ref<string | null>(null)

onMounted(async () => {
    try {
        templates.value = await api.listTemplates()
    } catch (e) {
        errorText.value = formatApiError(e)
    }
})

function formatUpdatedAt(iso: string): string {
    const t = new Date(iso)
    return Number.isNaN(t.getTime()) ? iso : t.toLocaleString('zh-CN', { hour12: false })
}
</script>

<template>
    <main class="min-h-screen bg-[#070d18] text-zinc-200">
        <div class="mx-auto max-w-3xl px-6 py-12">
            <h1 class="mb-1 text-2xl font-semibold text-zinc-100">模板列表</h1>
            <p class="mb-8 text-sm text-zinc-500">选择一个模板进入编辑器</p>

            <!-- 读取失败态（服务端未起等） -->
            <div
                v-if="errorText"
                class="rounded-lg border border-red-900/60 bg-red-950/40 px-4 py-3 text-sm text-red-300"
            >
                模板列表读取失败：{{ errorText }}
            </div>

            <!-- 空态（seed 未生效） -->
            <div
                v-else-if="templates && templates.length === 0"
                class="rounded-lg border border-zinc-800 bg-zinc-900/60 px-4 py-10 text-center text-sm text-zinc-500"
            >
                暂无模板——seed 未生效，请确认服务端启动播种已执行。
            </div>

            <!-- 目录卡片 -->
            <ul v-else-if="templates" class="grid gap-3 sm:grid-cols-2">
                <li v-for="t in templates" :key="t.id">
                    <RouterLink
                        :to="`/editor/${t.id}`"
                        class="block rounded-lg border border-zinc-800 bg-zinc-900/60 px-4 py-4 transition-colors hover:border-sky-700 hover:bg-zinc-900"
                    >
                        <div class="truncate font-medium text-zinc-100">{{ t.name }}</div>
                        <div class="mt-1 font-mono text-xs text-zinc-500">
                            更新于 {{ formatUpdatedAt(t.updatedAt) }}
                        </div>
                    </RouterLink>
                </li>
            </ul>
        </div>
    </main>
</template>
