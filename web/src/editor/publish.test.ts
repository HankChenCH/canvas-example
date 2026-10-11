// 发布编排纯逻辑测试（template-draft-publish 批 B ② TDD 缝，spec §6 / 票 05）：
// 版本徽标推导（空数组 = 未发布）与发布编排决策矩阵——dirty 自动促存再发（一次
// 动作）、保存失败中止（不吞保存错误面）、duplicate 视为发布成功。HTTP 客户端经
// 调用方注入（newtemplate.ts 同款，测试免 mock fetch）。
import { describe, expect, it } from 'vitest'

import { ApiError, type PublishResult, type TemplateVersionSummary } from '../api'

import { latestVersionOf, runPublishFlow, versionBadgeOf, versionListState } from './publish'

const version = (n: number): TemplateVersionSummary => ({ version: n, createdAt: `2026-10-1${n}` })

describe('versionBadgeOf / latestVersionOf（版本徽标推导，spec §3「空数组 = 未发布」）', () => {
    it('空数组 → 「未发布」（发布态是推导态非存储态）', () => {
        expect(latestVersionOf([])).toBeNull()
        expect(versionBadgeOf([])).toBe('未发布')
    })

    it('有版本 → 最新版徽标 vN；取 MAX 容错乱序（服务端降序，不依赖传输序）', () => {
        expect(latestVersionOf([version(3), version(1), version(2)])).toBe(3)
        expect(versionBadgeOf([version(1), version(3), version(2)])).toBe('v3')
    })

    it('单版本即该版本', () => {
        expect(versionBadgeOf([version(1)])).toBe('v1')
    })
})

describe('versionListState（版本抽屉列表态：未知 ≠ 未发布，评审修复 F1）', () => {
    it('null → unknown：列表未加载或拉取失败，不得冒充「尚未发布过版本」', () => {
        expect(versionListState(null)).toBe('unknown')
    })

    it('空数组 → unpublished（未发布推导态，spec §3）；非空 → ready', () => {
        expect(versionListState([])).toBe('unpublished')
        expect(versionListState([version(1)])).toBe('ready')
    })
})

describe('runPublishFlow（发布编排，票 05：dirty 自动促存再发，一次动作）', () => {
    it('clean：跳过保存直接 publish（保存流不变声明——发布不强制无谓 PUT）', async () => {
        const calls: string[] = []
        const result = await runPublishFlow({
            dirty: false,
            save: async () => {
                calls.push('save')
                return true
            },
            publish: async () => {
                calls.push('publish')
                return { version: 3, createdAt: 't' }
            },
        })
        expect(calls).toEqual(['publish'])
        expect(result).toEqual({ ok: true, version: 3, duplicate: false, note: '已发布 v3', saved: false })
    })

    it('dirty：先保存成功后 publish（顺序断言——「把我现在看到的定下来」）', async () => {
        const calls: string[] = []
        const result = await runPublishFlow({
            dirty: true,
            save: async () => {
                calls.push('save')
                return true
            },
            publish: async () => {
                calls.push('publish')
                return { version: 4, createdAt: 't' }
            },
        })
        expect(calls).toEqual(['save', 'publish'])
        expect(result).toEqual({ ok: true, version: 4, duplicate: false, note: '已发布 v4', saved: true })
    })

    it('dirty 且保存失败：中止发布（publish 不得调用），note null 不覆盖保存错误面', async () => {
        const calls: string[] = []
        const result = await runPublishFlow({
            dirty: true,
            save: async () => {
                calls.push('save')
                return false
            },
            publish: async () => {
                calls.push('publish')
                return { version: 9, createdAt: 't' }
            },
        })
        expect(calls).toEqual(['save'])
        expect(result).toEqual({ ok: false, note: null })
    })

    it('duplicate:true 也算发布成功（幂等跳过），note 提示与最新版本一致', async () => {
        const publish = async (): Promise<PublishResult> => ({ version: 2, createdAt: 't', duplicate: true })
        const clean = await runPublishFlow({ dirty: false, save: async () => true, publish })
        expect(clean).toEqual({
            ok: true,
            version: 2,
            duplicate: true,
            note: '内容与 v2 一致，跳过重复发布',
            saved: false,
        })
        const dirty = await runPublishFlow({ dirty: true, save: async () => true, publish })
        expect(dirty.ok).toBe(true)
    })

    it('publish 打回（422 invalid_graph 等）：中止并返回稳定码在前文案', async () => {
        const result = await runPublishFlow({
            dirty: false,
            save: async () => true,
            publish: async () => {
                throw new ApiError(422, 'invalid_graph', '图预检失败')
            },
        })
        expect(result).toEqual({ ok: false, note: '发布失败：invalid_graph：图预检失败' })
    })
})
