// 时间戳本地化纯逻辑测试（评审修复 F4 抽取缝）：解析失败原样回显 + 合法时间戳
// 走 zh-CN 本地化。输出文本受运行环境时区/ICU 影响，断言形状而非具体时刻文本。
import { describe, expect, it } from 'vitest'

import { formatTimestamp } from './format'

describe('formatTimestamp（RFC3339 → zh-CN 本地时间，24 小时制）', () => {
    it('解析失败原样回显（宽容面：不伪造非法时间的显示）', () => {
        expect(formatTimestamp('not-a-date')).toBe('not-a-date')
    })

    it('合法时间戳 → zh-CN 斜杠日期形态（yyyy/m/d 开头）', () => {
        expect(formatTimestamp('2026-01-02T03:04:05Z')).toMatch(/^\d{4}\/\d{1,2}\/\d{1,2} /)
    })
})
