<script setup lang="ts">
// 编辑器占位页（14 票）：只取模板名显示，证明路由与读端点接通；完整装配
// （工具栏/画布/抽屉/dirty）在 15 票。404 template_not_found → 错误提示 +
// 返回列表链接（spec §4.1 编辑器页行为）。
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

import { api, ApiError, formatApiError, type TemplateRecord } from '../api'

const route = useRoute()
const template = ref<TemplateRecord | null>(null)
const notFound = ref(false)
const errorText = ref<string | null>(null)

onMounted(async () => {
    try {
        template.value = await api.getTemplate(String(route.params.id))
    } catch (e) {
        // 按 code 判定语义（spec §2.1），不按 HTTP status
        if (e instanceof ApiError && e.code === 'template_not_found') {
            notFound.value = true
        } else {
            errorText.value = formatApiError(e)
        }
    }
})
</script>

<template>
    <main class="min-h-screen bg-[#070d18] text-zinc-200">
        <div class="mx-auto max-w-3xl px-6 py-12">
            <RouterLink to="/" class="text-sm text-sky-400 hover:text-sky-300"
                >← 返回列表</RouterLink
            >

            <!-- 模板不存在 -->
            <div
                v-if="notFound"
                class="mt-8 rounded-lg border border-red-900/60 bg-red-950/40 px-4 py-3 text-sm text-red-300"
            >
                模板不存在（template_not_found）。
                <RouterLink to="/" class="text-sky-400 hover:text-sky-300">返回列表</RouterLink>
            </div>

            <!-- 读取失败态 -->
            <div
                v-else-if="errorText"
                class="mt-8 rounded-lg border border-red-900/60 bg-red-950/40 px-4 py-3 text-sm text-red-300"
            >
                模板读取失败：{{ errorText }}
            </div>

            <!-- 占位正文 -->
            <div v-else-if="template" class="mt-8">
                <h1 class="text-2xl font-semibold text-zinc-100">{{ template.name }}</h1>
                <p class="mt-2 text-sm text-zinc-500">
                    编辑器装配在后续工单交付，本页为占位（帧数
                    {{ template.canvases.length }}）。
                </p>
            </div>
        </div>
    </main>
</template>
