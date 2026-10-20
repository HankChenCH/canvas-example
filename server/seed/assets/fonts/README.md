# seed 字体资产

`NotoSansSC-Regular.ttf`（Google Fonts 分发的 Noto Sans SC 可变字体，默认实例
wght 400 = Regular）由两条渲染轨共用，graph 内以 `assets/fonts/NotoSansSC-Regular.ttf`
相对路径引用。

**约束：必须是 glyf 轮廓的单体 TTF，勿换回官方 OTF。** PDF 渲染轨
（go-canvas/pdf-renderer → go-pdf/fpdf）的 TTF 解析器拒收 CFF 轮廓——
官方渠道的 `NotoSansSC-Regular.otf`（OTTO 头）在 PDF 轨注册字体即报
「字体解析失败(TTF 格式不合法或为 fpdf 不支持的变体)」。
