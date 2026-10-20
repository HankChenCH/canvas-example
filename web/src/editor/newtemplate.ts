// 新建模板纯逻辑（22 票空白入口 + 24 票图片快捷入口，spec §4.1 列表页新建；页模型
// 适配工票 03）：
// 载荷工厂 + 单调用编排。空白图 = 单页 A4 竖版 794×1123（与证书场景 seed 同规格）、
// layers 空——形状与 §6.2 冒烟断言 4 同源（服务端保存预检与前端
// decodeGraph 双吃）；图片建 = 画布宽高取图片像素宽高、图片声明为画布背景
// （ADR 0022：canvas.background，cover 零裁切——底图图层惯例退役，spec Q6 迁移）
// （wire 逐键同 @hankchen/canvas encodeGraph canonical 输出，测试以往返恒等锁死）。
// 页形状 = spec 页形状契约：id 前端生成、name 不落键（缺省显示「第 N 页」）、无
// flow 键（链外页）；dataSourceId 随建随绑
// （新建弹窗可选数据源；null = 暂不绑定 → 载荷省略该键，稍后经编辑器抽屉绑定）。
// 与 Vue/DOM 解耦——尺寸解码
// （createImageBitmap）留宿主，编排消费已解码字节 + 像素尺寸；HTTP 客户端经调用方
// 注入（测试免 mock fetch），失败文案稳定码在前、不向上抛。
import { formatApiError } from '../api'
import type { TemplateCreatePayload, TemplateRecord } from '../api'

import { newPageId } from './pages'

/** 空白画布尺寸：A4 竖版，与 seed 场景同规格 */
export const BLANK_CANVAS_WIDTH = 794
export const BLANK_CANVAS_HEIGHT = 1123

/** 空白模板载荷（spec §2.4 #3 + 页形状契约）：POST /api/templates 直接可用。
 *  dataSourceId
 *  null = 暂不绑定 → 载荷省略该键（与服务端 *int64 缺省 nil 同语义），非 null =
 *  随建随绑（服务端做存在性先验） */
export function blankTemplatePayload(name: string, dataSourceId: number | null): TemplateCreatePayload {
    return {
        name,
        pages: [
            {
                id: newPageId(),
                graph: { canvas: { width: BLANK_CANVAS_WIDTH, height: BLANK_CANVAS_HEIGHT }, layers: [] },
            },
        ],
        ...(dataSourceId === null ? {} : { dataSourceId }),
    }
}

/** 图片模板载荷（24 票，spec §2.4 #3 载荷形状）：图片像素尺寸即画布尺寸，图片
 *  声明为画布背景（ADR 0022：canvas.background，cover 铺满零裁切，不再产生底图
 *  图层——spec Q6 迁移）；src 为上传响应 url（前导斜杠形态，spec §2.2）原样进
 *  background.image。wire 条件写键：仅 image 分量落键（color 未设不落键，键序
 *  color, image——与 @hankchen/canvas encodeGraph canonical 输出逐键一致） */
export function imageTemplatePayload(
    name: string,
    image: { src: string; width: number; height: number },
    dataSourceId: number | null,
): TemplateCreatePayload {
    return {
        name,
        pages: [
            {
                id: newPageId(),
                graph: {
                    canvas: {
                        width: image.width,
                        height: image.height,
                        background: { image: image.src },
                    },
                    layers: [],
                },
            },
        ],
        ...(dataSourceId === null ? {} : { dataSourceId }),
    }
}

export type CreateTemplateResult = { ok: true; record: TemplateRecord } | { ok: false; message: string }

/** 从零建模板编排：名字裁剪 → POST /templates → 全量记录。空名就地拒绝不打
 *  服务端（先问名再建；弃置模板可经列表页卡片删除回收，spec §2.4 #10）；
 *  dataSourceId 随建随绑透传（null = 暂不绑定）；失败返回稳定码在前的可读文案。 */
export async function createBlankTemplate(params: {
    name: string
    dataSourceId: number | null
    createTemplate: (payload: TemplateCreatePayload) => Promise<TemplateRecord>
}): Promise<CreateTemplateResult> {
    const name = params.name.trim()
    if (!name) return { ok: false, message: '模板名不能为空' }
    try {
        return { ok: true, record: await params.createTemplate(blankTemplatePayload(name, params.dataSourceId)) }
    } catch (e) {
        return { ok: false, message: formatApiError(e) }
    }
}

/** 待建背景图片描述：字节（上传用）+ 浏览器解码出的像素尺寸（画布尺寸来源）。
 *  由宿主在选图时经 createImageBitmap 解码组装（DOM 通道不进本模块）。 */
export interface BackgroundImageFile {
    name: string
    mime: string
    bytes: Uint8Array
    width: number
    height: number
}

/** 图片建模板编排（24 票）：名字与选图就地校验 → POST /assets 上传 →
 *  POST /templates 建模板（dataSourceId 随建随绑透传，null = 暂不绑定）。先传后建
 *  ——上传打回则无模板落地；建模板打回时已传资源成为孤儿文件（/api/assets 无
 *  DELETE，与编辑器上传即写 graph 同口径，demo 粒度接受）。失败返回稳定码在前的
 *  可读文案。 */
export async function createImageTemplate(params: {
    name: string
    dataSourceId: number | null
    image: BackgroundImageFile | null
    uploadAsset: (file: { name: string; mime: string; bytes: Uint8Array }) => Promise<{ url: string }>
    createTemplate: (payload: TemplateCreatePayload) => Promise<TemplateRecord>
}): Promise<CreateTemplateResult> {
    const name = params.name.trim()
    if (!name) return { ok: false, message: '模板名不能为空' }
    if (!params.image) return { ok: false, message: '请先选择背景图片' }
    try {
        const { url } = await params.uploadAsset({
            name: params.image.name,
            mime: params.image.mime,
            bytes: params.image.bytes,
        })
        return {
            ok: true,
            record: await params.createTemplate(imageTemplatePayload(name, {
                src: url,
                width: params.image.width,
                height: params.image.height,
            }, params.dataSourceId)),
        }
    } catch (e) {
        return { ok: false, message: formatApiError(e) }
    }
}
