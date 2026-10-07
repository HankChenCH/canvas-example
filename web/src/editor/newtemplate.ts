// 新建模板纯逻辑（22 票空白入口 + 24 票底图快捷入口，spec §4.1 列表页新建）：
// 载荷工厂 + 单调用编排。空白图 = 单帧 A4 竖版 794×1123（与证书场景 seed 同规格）、
// layers 空、帧名「第 1 帧」——形状与 §6.2 冒烟断言 4 同源（服务端保存预检与前端
// decodeGraph 双吃）；底图图 = 画布宽高取图片像素宽高、单层 ImageLayer「底图」铺满
// 画布（wire 逐键同 @hankchen/canvas encodeGraph canonical 输出，测试以往返恒等锁死）。
// flowChain 显式 null = 空链 = 纯文档管线（spec §3.1）；不带 dataSourceId（spec
// §2.4 #3，数据源是独立实体，后续经编辑器抽屉绑定）。与 Vue/DOM 解耦——尺寸解码
// （createImageBitmap）留宿主，编排消费已解码字节 + 像素尺寸；HTTP 客户端经调用方
// 注入（测试免 mock fetch），失败文案稳定码在前、不向上抛。
import { formatApiError } from '../api'
import type { TemplateRecord, TemplateWritePayload } from '../api'

/** 空白帧名（域语言是帧，帧 tab 显同名） */
export const BLANK_FRAME_NAME = '第 1 帧'
/** 空白画布尺寸：A4 竖版，与 seed 场景同规格 */
export const BLANK_CANVAS_WIDTH = 794
export const BLANK_CANVAS_HEIGHT = 1123

/** 空白模板载荷（spec §2.4 #3）：POST /api/templates 直接可用 */
export function blankTemplatePayload(name: string): TemplateWritePayload {
    return {
        name,
        canvases: [
            {
                name: BLANK_FRAME_NAME,
                graph: { canvas: { width: BLANK_CANVAS_WIDTH, height: BLANK_CANVAS_HEIGHT }, layers: [] },
            },
        ],
        flowChain: null,
    }
}

/** 底图图层名（层面板显示与后续选中参照） */
export const BASE_IMAGE_LAYER_NAME = '底图'
/** 底图优先级：seed「页面底色」同约定（priority 最高 = 视觉最底）；编辑器画拉
 *  建层取现最小 −1，新层自然落底图之上 */
export const BASE_IMAGE_PRIORITY = 100

/** 底图模板载荷（24 票，spec §2.4 #3 载荷形状）：图片像素尺寸即画布与图层尺寸，
 *  src 为上传响应 url（前导斜杠形态，spec §2.2）原样进 data.value（StaticValue
 *  两键形态——PHP ImageLayer::fromGraph / Go n.Data.Value / JS decode 三端同源） */
export function imageTemplatePayload(
    name: string,
    image: { src: string; width: number; height: number },
): TemplateWritePayload {
    return {
        name,
        canvases: [
            {
                name: BLANK_FRAME_NAME,
                graph: {
                    canvas: { width: image.width, height: image.height },
                    layers: [
                        {
                            type: 'ImageLayer',
                            name: BASE_IMAGE_LAYER_NAME,
                            priority: BASE_IMAGE_PRIORITY,
                            spec: {
                                shape: {
                                    width: image.width,
                                    height: image.height,
                                    autoWidth: false,
                                    autoHeight: false,
                                    lineHeight: 1,
                                    padding: { top: 0, bottom: 0, left: 0, right: 0 },
                                    border: { top: null, bottom: null, left: null, right: null },
                                    backgroundColor: null,
                                },
                                align: { horizontal: 'left', vertical: 'top' },
                                position: { x: 0, y: 0, position: 'top-left' },
                            },
                            data: { valueType: 'StaticValue', value: image.src },
                        },
                    ],
                },
            },
        ],
        flowChain: null,
    }
}

export type CreateTemplateResult = { ok: true; record: TemplateRecord } | { ok: false; message: string }

/** 从零建模板编排：名字裁剪 → POST /templates → 全量记录。空名就地拒绝不打
 *  服务端（先问名再建；弃置模板可经列表页卡片删除回收，spec §2.4 #10）；
 *  失败返回稳定码在前的可读文案。 */
export async function createBlankTemplate(params: {
    name: string
    createTemplate: (payload: TemplateWritePayload) => Promise<TemplateRecord>
}): Promise<CreateTemplateResult> {
    const name = params.name.trim()
    if (!name) return { ok: false, message: '模板名不能为空' }
    try {
        return { ok: true, record: await params.createTemplate(blankTemplatePayload(name)) }
    } catch (e) {
        return { ok: false, message: formatApiError(e) }
    }
}

/** 底图图片描述：字节（上传用）+ 浏览器解码出的像素尺寸（画布/图层尺寸来源）。
 *  由宿主在选图时经 createImageBitmap 解码组装（DOM 通道不进本模块）。 */
export interface BaseImageFile {
    name: string
    mime: string
    bytes: Uint8Array
    width: number
    height: number
}

/** 底图建模板编排（24 票）：名字与选图就地校验 → POST /assets 上传 →
 *  POST /templates 建模板。先传后建——上传打回则无模板落地；建模板打回时
 *  已传资源成为孤儿文件（/api/assets 无 DELETE，与编辑器上传即写 graph 同
 *  口径，demo 粒度接受）。失败返回稳定码在前的可读文案。 */
export async function createImageTemplate(params: {
    name: string
    image: BaseImageFile | null
    uploadAsset: (file: { name: string; mime: string; bytes: Uint8Array }) => Promise<{ url: string }>
    createTemplate: (payload: TemplateWritePayload) => Promise<TemplateRecord>
}): Promise<CreateTemplateResult> {
    const name = params.name.trim()
    if (!name) return { ok: false, message: '模板名不能为空' }
    if (!params.image) return { ok: false, message: '请先选择底图图片' }
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
            })),
        }
    } catch (e) {
        return { ok: false, message: formatApiError(e) }
    }
}
