#!/bin/sh
# spec §6.2 curl 冒烟(契约面),双轨通用:compose 轨(cd example && docker compose up
# --build)或 dev 轨(cd example/server && go run .)任一起服务后运行。
# 用法: scripts/smoke.sh [BASE],默认 http://localhost:8080(compose 轨验收口径)。
# 断言 1–3(11 票)读路径(组 3 另含图片层适配断言:contain(image-layer-fit 06 票,
# ADR 0023)+ fill/tile 四档同画布对照(image-fit-fill-tile 06 票,ADR 0024)
# + 圆形照片框 wire 腿:照片格 spec.clip=circle 条件写键殿后 fit + 四边 3px 环带
# 声明(image-clip-shape 06 票,ADR 0026));
# 4–7、10、11(12 票)写端点/上传/上限;6 段含数据源
# 实体 CRUD + 绑定通道(23 票);8–9(13 票)渲染管线 + keep-all + .cache 生效 + PDF 产物轨
# (PDF 集成共识:format=pdf 单文档轨轻断言;模板自带圆形照片框后,PDF 渲染隐式
# 走矢量 ClipEllipse 圆框路径,点位级 parity 归 go-canvas 包 CI)+ 批量产物像素
# 采样腿(主页产物圆框 10 点采样,双轨圆形腿之批量产物侧);
# 12(卡片操作修订 + 版本化)DELETE /api/templates/{id}:渲染前先 publish(缺省
# 渲染语义 = 最新已发布,未发布 409,spec §4);204 + 渲染记录级联清 + 版本行
# 级联清 + 产物文件保留;
# 13(28 票)DELETE /api/datasources/{id}:被引用 409 data_source_in_use → 解绑 →
# 204 → 404 闭环;14(网络字体方案)GET /api/fonts:清单白名单 {label, ref} 直链;
# 15 版本化轨(模板草稿-发布-版本化,spec §7 九断言):publish 201 v2 / 重复
# publish 200 duplicate / GET versions / 无 body 渲染 = 最新已发布带 version 字段 /
# 显式选版 / draft 哨兵 / 未发布 409 / version_not_found 404 / revert 内容一致。
# 16 渲染历史(渲染历史对比):GET /api/templates/{id}/renders → 数组 id 降序、
# 响应形状与 POST render 一致、draft 行 version null、无记录模板空数组。
# 任一断言失败非零退出(spec §6.2)。出网注意:渲染走 picsum 徽标(spec §5.3)。
# 模板内容 = 页序列(pages,ADR 0025):记录/写载荷/版本快照均为 pages 单键形状
# (canvases/flowChain 不再在场);页 id 前端生成、服务端只裁决非空字符串且页内
# 唯一,flow 语义错透传引擎 sentinel,页级形状错归服务端自有码 page_invalid。
# 注意:断言 3/8 依赖种子模板仍绑种子数据源(主页 1 + 续页 2 的 3 页容量数学)
# ——UI 里重绑/改数据源后会失真,属预期;版本化轨断言依赖重置运行库(启动存量
# 回填使 seed 持 v1 且草稿列与 v1 字节相等——pages 存储写侧恒经 marshal 归一,
# 触碰回写不改字节;本轨先断言未改草稿发布 = duplicate v1,改写后才首发 v2;
# 复跑不重置时版本号续增断言失真),重置:compose 轨
# `docker compose down -v && docker compose up -d`(卷随清,重启播种自动补回);
# dev 轨停服删 server/data/app.db 重启。旧形状库(页序列翻转前的 canvases/
# flow_chain 列)启动即自愈重建三表 + 种子重播(ADR 0025),无需手工迁移。
set -eu

BASE="${1:-http://localhost:8080}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

die() { echo "FAIL: $*" >&2; exit 1; }
command -v curl >/dev/null || die "需要 curl"
command -v python3 >/dev/null || die "需要 python3"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# get <path> <outfile>:curl -f 取响应(非 2xx 即失败)
get() {
	curl -fsS -o "$2" "$BASE$1" || die "GET $1 请求失败(服务未起或非 2xx)"
}

# assert_json <file> <python 表达式>:对解析后的 JSON(变量 d)断言,失败即中止
assert_json() {
	python3 - "$1" "$2" <<'PYEOF'
import json, re, sys
with open(sys.argv[1], encoding="utf-8") as f:
    d = json.load(f)
assert eval(sys.argv[2], {"d": d, "re": re}), "断言失败: %s\n实际值: %s" % (sys.argv[2], json.dumps(d, ensure_ascii=False)[:600])
PYEOF
}

# request <method> <path> <ctype> <datafile> <outfile>:返回 http 状态码
# (不用 -f:错误路径断言需要拿到状态码与错误信封)
request() {
	curl -sS -o "$5" -w '%{http_code}' -X "$1" -H "Content-Type: $3" --data-binary @"$4" "$BASE$2"
}

# expect_error <method> <path> <ctype> <datafile> <want-status> <want-code>:
# 状态码与错误信封 code 双断言
expect_error() {
	_status=$(request "$1" "$2" "$3" "$4" "$TMP/err.json")
	[ "$_status" = "$5" ] || die "$1 $2 期望 $5,实得 $_status"
	assert_json "$TMP/err.json" "d['error']['code'] == '$6'"
}

# assert_png_size <file> <width> <height> <label>:PNG 头(IHDR)尺寸断言——
# 直链素材内在尺寸的契约面(tile 平铺单元 = 内在尺寸,尺寸错即平铺语义错)
assert_png_size() {
	python3 - "$1" "$2" "$3" "$4" <<'PYEOF'
import struct, sys
f = open(sys.argv[1], "rb")
assert f.read(16) == b"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR", "不是 PNG 或头部异常"
w, h = struct.unpack(">II", f.read(8))
assert (w, h) == (int(sys.argv[2]), int(sys.argv[3])), \
	"%s 应为 %sx%s,实得 %dx%d" % (sys.argv[4], sys.argv[2], sys.argv[3], w, h)
PYEOF
}

# assert_png_pixels <file> <label> <points-json>:渲染产物逐点像素采样断言——
# 点 = [x, y, "#rrggbb"](期望色 ±2/通道)。纯 stdlib 解码(zlib + 四种行滤波
# 逆变换),服务端产物为 Go png.Encode(NRGBA 全不透明 → 8bit RGB)。期望色口
# 径:采样点须落在无抗锯齿的实色区(渲染端圆掩码/描边为二值覆盖无抗锯齿)与
# 水印 tile 透明区(锚点由组 3 wire 断言钉死),期望色即基色、无混色无水印叠色
assert_png_pixels() {
	python3 - "$1" "$2" "$3" <<'PYEOF'
import json, struct, sys, zlib
path, label, points = sys.argv[1], sys.argv[2], json.loads(sys.argv[3])
data = open(path, "rb").read()
assert data[:8] == b"\x89PNG\r\n\x1a\n", f"{label}: 不是 PNG"
pos, idat, w, h, ct = 8, b"", 0, 0, 0
while pos < len(data):
    ln = struct.unpack(">I", data[pos:pos+4])[0]
    typ = data[pos+4:pos+8]
    if typ == b"IHDR":
        w, h, depth, ct, _, _, il = struct.unpack(">IIBBBBB", data[pos+8:pos+8+ln])
        assert depth == 8 and il == 0 and ct in (2, 6), \
            f"{label}: 不支持的 PNG 形态 depth={depth} ct={ct} interlace={il}"
    elif typ == b"IDAT":
        idat += data[pos+8:pos+8+ln]
    pos += 12 + ln
nch = 3 if ct == 2 else 4
raw = zlib.decompress(idat)
stride = w * nch
rows = []
prev = bytearray(stride)
p = 0
for _ in range(h):
    f = raw[p]; p += 1
    line = bytearray(raw[p:p+stride]); p += stride
    for i in range(stride):
        a = line[i-nch] if i >= nch else 0
        b = prev[i]
        c = prev[i-nch] if i >= nch else 0
        if f == 1: line[i] = (line[i] + a) & 255
        elif f == 2: line[i] = (line[i] + b) & 255
        elif f == 3: line[i] = (line[i] + (a + b) // 2) & 255
        elif f == 4:
            pa, pb, pc = abs(b - c), abs(a - c), abs(a + b - 2 * c)
            pr = a if (pa <= pb and pa <= pc) else (b if pb <= pc else c)
            line[i] = (line[i] + pr) & 255
    rows.append(line)
    prev = line
for x, y, want in points:
    want_rgb = tuple(int(want[i:i+2], 16) for i in (1, 3, 5))
    row = rows[y]
    got = tuple(row[x*nch:x*nch+3])
    assert len(got) == 3, f"{label}: 像素({x},{y}) 取样越界(解码器缺陷)"
    assert all(abs(g - e) <= 2 for g, e in zip(got, want_rgb)), \
        f"{label}: 像素({x},{y}) 期望 {want}±2, 实得 rgb{got}"
print(f"   {label}: {len(points)} 点像素采样断言通过")
PYEOF
}

echo "==1/16 GET /api/health =="
get /api/health "$TMP/health.json"
assert_json "$TMP/health.json" 'd.get("status") == "ok"'

echo "==2/16 GET /api/templates =="
get /api/templates "$TMP/templates.json"
assert_json "$TMP/templates.json" 'isinstance(d, list) and len(d) >= 1'
assert_json "$TMP/templates.json" '[t["updatedAt"] for t in d] == sorted((t["updatedAt"] for t in d), reverse=True)'
assert_json "$TMP/templates.json" 'all(set(t.keys()) == {"id", "name", "updatedAt"} for t in d)'
# SEED_ID 取种子模板:spec §6.2 读「首项」,但写端点在场后列表首项可能已是
# 冒烟产物(updatedAt 更新)——按 spec §5.2 钉名取回种子,保证 1–3 断言可复跑
SEED_ID="$(python3 -c '
import json, sys
items = json.load(open(sys.argv[1], encoding="utf-8"))
seeds = [t for t in items if t["name"] == "结业证书 · 批量打印页"]
assert seeds, "列表中找不到种子模板「结业证书 · 批量打印页」(可能已被改名)"
print(seeds[0]["id"])
' "$TMP/templates.json")"
echo "   SEED_ID=$SEED_ID"

echo "==3/16 GET /api/templates/{SEED_ID} =="
get "/api/templates/$SEED_ID" "$TMP/template.json"
# 页序列形状(ADR 0025):恰 2 页、页名钉死(主页/续页)、页 id 在场(非空字符串)
# 且页内唯一(编辑层引用键,seed fixture 落 id pg_main/pg_cont);旧 canvases/
# flowChain 双键不再在场(无双轨过渡,spec)
assert_json "$TMP/template.json" 'len(d["pages"]) == 2 and [p["name"] for p in d["pages"]] == ["主页", "续页"]'
assert_json "$TMP/template.json" 'all(isinstance(p["id"], str) and p["id"] for p in d["pages"]) and len({p["id"] for p in d["pages"]}) == 2'
assert_json "$TMP/template.json" '"canvases" not in d and "flowChain" not in d'
# 页 flow 形状:主页 fixed / 续页 paged + omitIfEmpty——键面镜像引擎链节点去
# frame(帧位 = 页序,页载荷不携带 frame;无 flow = 链外页)
assert_json "$TMP/template.json" 'd["pages"][0]["flow"] == {"mode": "fixed"}'
assert_json "$TMP/template.json" 'd["pages"][1]["flow"] == {"mode": "paged", "omitIfEmpty": True}'
# 画布背景迁移(ADR 0022):种子两页白色底已迁 canvas.background——background
# 落键且仅 color 分量(wire 条件写键),图层树不再有「页面底色」垫底层
assert_json "$TMP/template.json" 'all(p["graph"]["canvas"].get("background") == {"color": "#ffffff"} for p in d["pages"])'
assert_json "$TMP/template.json" 'all(all(l["name"] != "页面底色" for l in p["graph"]["layers"]) for p in d["pages"])'
# 数据源是独立实体:模板持引用(spec §2.4 数据源段),引用的实体可回读且含 schema/data
assert_json "$TMP/template.json" 'isinstance(d["dataSourceId"], int) and d["dataSourceId"] > 0'
DS_ID="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["dataSourceId"])' "$TMP/template.json")"
get "/api/datasources/$DS_ID" "$TMP/ds.json"
assert_json "$TMP/ds.json" 'd["schema"] is not None and d["data"] is not None'
assert_json "$TMP/ds.json" 'd["data"]["certificates"] and len(d["data"]["certificates"]) == 5'
# 图片层适配 contain(ADR 0023,本期 06 票):照片格(表格行内 160×400 表达式图片
# {{row.photo}})spec.fit=contain,两页同构;徽标位(72×72)保持 cover 缺省不落键,
# 作现行裁切对照;dataset 行 1 照片换 320×160 横幅比例——错形进盒不再裁切、
# 圆内 letterbox 空隙露层背景灰(渲染像素面由组 8 采样腿钉,此处 wire 契约面)
assert_json "$TMP/template.json" 'all(
	(cells := next(l for l in p["graph"]["layers"] if l["name"] == "证书卡片区")["template"]["cells"])
	and cells[0]["content"]["data"]["expression"] == "{{row.photo}}"
	and cells[0]["content"]["spec"].get("fit") == "contain"
	for p in d["pages"]
)'
assert_json "$TMP/template.json" '"fit" not in next(l for l in d["pages"][0]["graph"]["layers"] if l["name"] == "机构徽标")["spec"]'
assert_json "$TMP/ds.json" 'd["data"]["certificates"][0]["photo"] == "assets/u/banner-photo.png"'
# 圆形照片框 wire 腿(编辑器消费的模板面,image-clip-shape 06 票,ADR 0026):
# 两页照片格 content 图片层 spec.clip=circle(真圆形学员头像位,fixture 假圆
# 退役)+ 四边 3px 同色环带声明(渲染端按 min 宽/top 首色解析描边参数);clip
# 条件写键、键序殿后 spec 末位 fit 之后(json.load 保序,钉响应 marshal 键序);
# 徽标层不落 clip 键(矩形缺省对照——徽标层 wire 零变化,适用域仅此一层)
assert_json "$TMP/template.json" 'all(
	(cells := next(l for l in p["graph"]["layers"] if l["name"] == "证书卡片区")["template"]["cells"])
	and cells[0]["content"]["spec"].get("clip") == "circle"
	and cells[0]["content"]["spec"]["shape"]["border"] == {
		"top": {"width": 3, "color": "#1f4e79"}, "right": {"width": 3, "color": "#1f4e79"},
		"bottom": {"width": 3, "color": "#1f4e79"}, "left": {"width": 3, "color": "#1f4e79"}}
	for p in d["pages"]
)'
assert_json "$TMP/template.json" 'all(
	(spec := next(l for l in p["graph"]["layers"] if l["name"] == "证书卡片区")["template"]["cells"][0]["content"]["spec"])
	and list(spec)[-2:] == ["fit", "clip"]
	for p in d["pages"]
)'
assert_json "$TMP/template.json" '"clip" not in next(l for l in d["pages"][0]["graph"]["layers"] if l["name"] == "机构徽标")["spec"]'
get /assets/u/banner-photo.png "$TMP/banner.png"
assert_png_size "$TMP/banner.png" 320 160 "横幅照片(与照片格 160x400 错形)"
# 图片层适配四档同画布对照(ADR 0024,本期 06 票):主页 cover(徽标缺省不落
# 键,上已钉)与 contain(照片格,上已钉两页)存量保留,新增 fill 与 tile——
# fill:装饰纹理带,192×192 纹理/渐变素材(比例失真不敏感)进 794×70 固定横带,
# 整图拉伸无裁切无空隙;tile:防伪水印,128×128 印章按内在尺寸 1:1 平铺盖证书
# 卡片区(714×800 @ (40,190),priority 40 压表上层),left/top 首块锚定全铺、
# 右/下边缘半块截断。渲染像素面(纹理拉伸形变、水印网格截断)经 dev/compose
# 双轨渲染产物取样验证,属人工目验轨不在 smoke(沿本文件契约断言形态)
assert_json "$TMP/template.json" '(
	band := next(l for l in d["pages"][0]["graph"]["layers"] if l["name"] == "装饰纹理带")
) and band["spec"].get("fit") == "fill" and (band["spec"]["shape"]["width"], band["spec"]["shape"]["height"], band["spec"]["position"]["x"], band["spec"]["position"]["y"]) == (794, 70, 0, 120) and band["data"] == {"valueType": "StaticValue", "value": "assets/u/texture-fill.png"}'
assert_json "$TMP/template.json" '(
	mark := next(l for l in d["pages"][0]["graph"]["layers"] if l["name"] == "防伪水印")
) and mark["spec"].get("fit") == "tile" and mark["spec"]["align"] == {"horizontal": "left", "vertical": "top"} and (mark["spec"]["shape"]["width"], mark["spec"]["shape"]["height"], mark["spec"]["position"]["x"], mark["spec"]["position"]["y"]) == (714, 800, 40, 190) and mark["priority"] == 40 and mark["data"] == {"valueType": "StaticValue", "value": "assets/u/watermark-tile.png"}'
# 两块 showcase 素材随 seed 补齐直链下发;tile 单元 = 内在尺寸(1:1 零参数),尺寸钉死
get /assets/u/texture-fill.png "$TMP/texture.png"
assert_png_size "$TMP/texture.png" 192 192 "纹理素材(fill 拉伸进 794x70 横带)"
get /assets/u/watermark-tile.png "$TMP/watermark.png"
assert_png_size "$TMP/watermark.png" 128 128 "水印素材(tile 平铺单元 = 内在尺寸)"

echo "==4/16 POST /api/templates(最小模板 → 201 自增 id,未绑数据源) =="
# 最小合法载荷 = pages 形态(ADR 0025):单页(画布 + 空图层序列),无 flow 键
# (= 链外页,全页无 flow = 空链 = 合法纯文档管线)
cat > "$TMP/min-template.json" <<'EOF'
{"name":"冒烟最小模板","pages":[{"id":"pg_smoke_min","name":"单页","graph":{"canvas":{"width":794,"height":1123},"layers":[]}}]}
EOF
_status=$(request POST /api/templates application/json "$TMP/min-template.json" "$TMP/t1.json")
[ "$_status" = "201" ] || die "POST /api/templates 期望 201,实得 $_status"
assert_json "$TMP/t1.json" "isinstance(d['id'], int) and d['id'] > $SEED_ID"
assert_json "$TMP/t1.json" "d['createdAt'] == d['updatedAt']"
assert_json "$TMP/t1.json" "d['dataSourceId'] is None and 'canvases' not in d and 'flowChain' not in d"
assert_json "$TMP/t1.json" "len(d['pages']) == 1 and d['pages'][0]['id'] == 'pg_smoke_min' and d['pages'][0]['name'] == '单页'"
assert_json "$TMP/t1.json" "'flow' not in d['pages'][0] and d['pages'][0]['graph']['canvas'] == {'width': 794, 'height': 1123}"
T1="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["id"])' "$TMP/t1.json")"
echo "   T1=$T1"

echo "==5/16 PUT /api/templates/{T1}(改 name;pages 整存替换,绑定引用不被触碰) =="
cat > "$TMP/put-t1.json" <<'EOF'
{"name":"冒烟改名模板","pages":[{"id":"pg_smoke_min","name":"单页","graph":{"canvas":{"width":794,"height":1123},"layers":[]}}]}
EOF
_status=$(request PUT "/api/templates/$T1" application/json "$TMP/put-t1.json" "$TMP/t1-put.json")
[ "$_status" = "200" ] || die "PUT /api/templates/$T1 期望 200,实得 $_status"
assert_json "$TMP/t1-put.json" "d['name'] == '冒烟改名模板' and [p['id'] for p in d['pages']] == ['pg_smoke_min']"
assert_json "$TMP/t1-put.json" "d['dataSourceId'] is None"

echo "==6/16 数据源实体 CRUD + 绑定通道(CRUD 校验码 / 绑定 / 解绑 / 404) =="
# 合法创建 → 201 全量记录
cat > "$TMP/ds-ok.json" <<'EOF'
{"name":"冒烟数据源","schema":{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","properties":{"student":{"type":"string"}},"required":["student"]},"data":{"student":"林晚晴"}}
EOF
_status=$(request POST /api/datasources application/json "$TMP/ds-ok.json" "$TMP/ds-ok-resp.json")
[ "$_status" = "201" ] || die "POST /api/datasources 期望 201,实得 $_status"
assert_json "$TMP/ds-ok-resp.json" "d['name'] == '冒烟数据源' and d['data'] == {'student': '林晚晴'}"
DS1="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["id"])' "$TMP/ds-ok-resp.json")"
echo "   DS1=$DS1"

cat > "$TMP/ds-bad-schema.json" <<'EOF'
{"name":"坏schema","schema":{"type":42},"data":{}}
EOF
expect_error POST /api/datasources application/json "$TMP/ds-bad-schema.json" 400 schema_invalid

cat > "$TMP/ds-mismatch.json" <<'EOF'
{"name":"坏data","schema":{"type":"object"},"data":[1,2]}
EOF
expect_error POST /api/datasources application/json "$TMP/ds-mismatch.json" 400 dataset_schema_mismatch

# PUT 更新 → 200 整存替换;寻址失败 → 404 data_source_not_found
cat > "$TMP/ds-update.json" <<'EOF'
{"name":"冒烟改名数据源","schema":{"type":"object"},"data":{"kept":true}}
EOF
_status=$(request PUT "/api/datasources/$DS1" application/json "$TMP/ds-update.json" "$TMP/ds-update-resp.json")
[ "$_status" = "200" ] || die "PUT /api/datasources/$DS1 期望 200,实得 $_status"
assert_json "$TMP/ds-update-resp.json" "d['name'] == '冒烟改名数据源' and d['data'] == {'kept': True}"
expect_error GET "/api/datasources/999999" application/json /dev/null 404 data_source_not_found

# 绑定 → 200 引用列更新;其后 PUT 模板不触碰绑定(spec §2.4 #5);
# 绑定不存在的数据源 → 404 data_source_not_found;解绑 → null
cat > "$TMP/bind.json" <<EOF
{"dataSourceId":$DS1}
EOF
_status=$(request PUT "/api/templates/$T1/datasource" application/json "$TMP/bind.json" "$TMP/bind-resp.json")
[ "$_status" = "200" ] || die "PUT …/datasource(绑定)期望 200,实得 $_status"
assert_json "$TMP/bind-resp.json" "d['dataSourceId'] == $DS1"
_status=$(request PUT "/api/templates/$T1" application/json "$TMP/put-t1.json" "$TMP/t1-put2.json")
[ "$_status" = "200" ] || die "绑定后 PUT 模板期望 200,实得 $_status"
assert_json "$TMP/t1-put2.json" "d['dataSourceId'] == $DS1"

cat > "$TMP/bind-bad.json" <<'EOF'
{"dataSourceId":999999}
EOF
expect_error PUT "/api/templates/$T1/datasource" application/json "$TMP/bind-bad.json" 404 data_source_not_found

# 创建即绑定(另存为单调用路径,spec §2.4 #3):POST 模板携带 dataSourceId → 201 随建随绑
cat > "$TMP/t2.json" <<EOF
{"name":"冒烟随建绑定模板","pages":[{"id":"pg_smoke_bind","name":"单页","graph":{"canvas":{"width":794,"height":1123},"layers":[]}}],"dataSourceId":$DS1}
EOF
_status=$(request POST /api/templates application/json "$TMP/t2.json" "$TMP/t2-resp.json")
[ "$_status" = "201" ] || die "POST /api/templates(带 dataSourceId)期望 201,实得 $_status"
assert_json "$TMP/t2-resp.json" "d['dataSourceId'] == $DS1"

cat > "$TMP/unbind.json" <<'EOF'
{"dataSourceId":null}
EOF
_status=$(request PUT "/api/templates/$T1/datasource" application/json "$TMP/unbind.json" "$TMP/unbind-resp.json")
[ "$_status" = "200" ] || die "PUT …/datasource(解绑)期望 200,实得 $_status"
assert_json "$TMP/unbind-resp.json" "d['dataSourceId'] is None"

echo "==7/16 POST /api/templates 保存预检打回(未知图层类型 / 双 paged / 页级形状错) =="
# 未知图层类型:graph 解码沿用既有 code 分流(透传不动)
cat > "$TMP/bad-layer.json" <<'EOF'
{"name":"坏图层模板","pages":[{"id":"pg_smoke_bad_layer","graph":{"canvas":{"width":794,"height":1123},"layers":[{"type":"GhostLayer","priority":0}]}}]}
EOF
expect_error POST /api/templates application/json "$TMP/bad-layer.json" 400 unknown_layer_type

# 双 paged:flow 语义错透传引擎流链校验 sentinel(页 flow 无 frame 键——帧位 =
# 页序由编译边界注入;帧序不合法 → flow_chain_invalid 不变)
cat > "$TMP/double-paged.json" <<'EOF'
{"name":"双paged模板","pages":[
  {"id":"pg_smoke_dp_a","graph":{"canvas":{"width":794,"height":1123},"layers":[]},"flow":{"mode":"paged"}},
  {"id":"pg_smoke_dp_b","graph":{"canvas":{"width":794,"height":1123},"layers":[]},"flow":{"mode":"paged"}}]}
EOF
expect_error POST /api/templates application/json "$TMP/double-paged.json" 400 flow_chain_invalid

# 页级形状错(ADR 0025 新增服务端自有码):页 id 缺失 → page_invalid(与流语义
# 错分流——「形状没写对」和「策略不合法」分报,spec 用户故事 8)
cat > "$TMP/no-page-id.json" <<'EOF'
{"name":"页id缺失模板","pages":[{"graph":{"canvas":{"width":794,"height":1123},"layers":[]}}]}
EOF
expect_error POST /api/templates application/json "$TMP/no-page-id.json" 400 page_invalid

# 页 id 页内重复 → page_invalid(重复键会互相覆盖工程数据,spec 用户故事 13)
cat > "$TMP/dup-page-id.json" <<'EOF'
{"name":"页id重复模板","pages":[
  {"id":"pg_smoke_dup","graph":{"canvas":{"width":794,"height":1123},"layers":[]}},
  {"id":"pg_smoke_dup","graph":{"canvas":{"width":794,"height":1123},"layers":[]}}]}
EOF
expect_error POST /api/templates application/json "$TMP/dup-page-id.json" 400 page_invalid

echo "==10/16 POST /api/assets(multipart 上传 → url 形态 + 可 GET) =="
PNG="$ROOT/server/seed/assets/u/student-1.png"
[ -f "$PNG" ] || die "上传样张缺失: $PNG"
_status=$(curl -sS -o "$TMP/upload.json" -w '%{http_code}' -F "file=@$PNG;type=image/png" "$BASE/api/assets")
[ "$_status" = "201" ] || die "POST /api/assets 期望 201,实得 $_status"
assert_json "$TMP/upload.json" "re.match(r'^/assets/u/[0-9a-f]{16}\.[A-Za-z0-9]{1,8}$', d['url']) is not None"
UPLOAD_URL="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["url"])' "$TMP/upload.json")"
curl -fsS -o "$TMP/upload-roundtrip.png" "$BASE$UPLOAD_URL" || die "GET $UPLOAD_URL 失败"
cmp -s "$TMP/upload-roundtrip.png" "$PNG" || die "上传回读字节与原文件不一致"

echo "==11/16 请求体上限与坏 body(413 / 400) =="
python3 -c 'import json,sys; sys.stdout.write(json.dumps({"name":"a" * (10 * 1024 * 1024)}))' > "$TMP/big.json"
expect_error POST /api/templates application/json "$TMP/big.json" 413 request_too_large

printf 'not json' > "$TMP/not-json.json"
expect_error POST /api/templates application/json "$TMP/not-json.json" 400 invalid_json

# ---- 断言 8–9(13 票):渲染管线 → RenderRecord + PNG 直链 ----

echo "==8/16 POST /api/templates/{SEED_ID}/render(渲染 → 201 RenderRecord) =="
# 无 body POST(服务端不读,spec §2.4 #7);徽标走 picsum 物化(唯一外网点)
_status=$(request POST "/api/templates/$SEED_ID/render" application/json /dev/null "$TMP/render1.json")
[ "$_status" = "201" ] || die "POST render 期望 201,实得 $_status"
assert_json "$TMP/render1.json" "d['templateId'] == $SEED_ID and isinstance(d['createdAt'], str)"
assert_json "$TMP/render1.json" "len(d['images']) == 3"
assert_json "$TMP/render1.json" "[ (i['frame'], i['name']) for i in d['images'] ] == [(0, '主页'), (1, '续页'), (1, '续页')]"
assert_json "$TMP/render1.json" "[i['url'] for i in d['images']] == [f'/renders/{d[\"id\"]}/{n}.png' for n in (1, 2, 3)]"
RENDER1_URLS="$(python3 -c 'import json,sys; print("\n".join(i["url"] for i in json.load(open(sys.argv[1], encoding="utf-8"))["images"]))' "$TMP/render1.json")"
for u in $RENDER1_URLS; do
	_ct="$(curl -fsS -o "$TMP/render-png.bin" -w '%{content_type}' "$BASE$u")" || die "GET $u 失败"
	[ "$_ct" = "image/png" ] || die "GET $u Content-Type = $_ct, 期望 image/png"
	[ -s "$TMP/render-png.bin" ] || die "GET $u 响应体为空"
done

# 圆形照片框像素采样腿(双轨圆形腿之批量产物侧,image-clip-shape 06 票,ADR
# 0026):主页产物上照片格圆框 10 点采样。几何:照片格 160×400 非方盒 @ (40,190),
# 内切正圆心 (120,390) 半径 80——环带 = 外缘内缩 3px 描边带(半径 77..80,基点
# 处像素中心全覆盖为纯描边色);行 1 横幅 contain 缩 160×80 进圆,圆心露太阳色;
# 行 2 方形人像 contain 恰满内切圆外接方,圆心露头部色;圆内 letterbox(照片带
# 上方)露层背景 #e8ecef;圆外盒内三点露格子背景 #f3f5f7——圆外不落墨 + 正圆
# 判别(若误画满盒椭圆,(120,220) 必为层背景非格子背景)。fixture 假圆退役,
# 圆由渲染端原生绘出
get "/renders/$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["id"])' "$TMP/render1.json")/1.png" "$TMP/render-page1.png"
assert_png_pixels "$TMP/render-page1.png" "圆形照片框(主页批量产物)" '[
	[120, 311, "#1f4e79"], [120, 468, "#1f4e79"],
	[41, 390, "#1f4e79"], [198, 390, "#1f4e79"],
	[120, 390, "#e8a33d"], [120, 790, "#2e6f6c"], [120, 340, "#e8ecef"],
	[50, 205, "#f3f5f7"], [120, 220, "#f3f5f7"], [190, 575, "#f3f5f7"]
]'

# keep-all 不清理(spec §2.4 #7):PUT 触碰模板(pages 原样回写——写侧 marshal
# 归一,字节不变,版本化轨 duplicate 判定依赖此性质)后再渲染,
# 旧 renderId 的 url 仍服务旧图;同时覆盖 .cache 生效——二次渲染不再下载
get "/api/templates/$SEED_ID" "$TMP/seed-before-touch.json"
python3 -c '
import json, sys
d = json.load(open(sys.argv[1], encoding="utf-8"))
json.dump({"name": d["name"], "pages": d["pages"]},
          open(sys.argv[2], "w", encoding="utf-8"), ensure_ascii=False)
' "$TMP/seed-before-touch.json" "$TMP/seed-touch-payload.json"
_status=$(request PUT "/api/templates/$SEED_ID" application/json "$TMP/seed-touch-payload.json" "$TMP/seed-touched.json")
[ "$_status" = "200" ] || die "PUT 模板(触碰)期望 200,实得 $_status"

# 缓存断言面按轨探测(断言 8 伴随):compose 轨 .cache 在容器内(运行相 WORKDIR=/app,
# 不落宿主),经 docker compose exec 探针;dev 轨缓存在宿主 CWD(example/server)。
# compose 项目内有 server 容器在跑即取容器探针,否则宿主路径。
compose_srv() { docker compose -f "$ROOT/compose.yaml" "$@"; }
COMPOSE_SERVER_CTR="$(compose_srv ps -q server 2>/dev/null || true)"
if [ -n "$COMPOSE_SERVER_CTR" ]; then
	echo "   缓存探针面 = compose 容器内(.cache 不落宿主)"
	cache_count() {
		compose_srv exec -T server sh -c 'find .cache -type f | wc -l' | tr -d '[:space:]'
	}
	cache_newer() {
		compose_srv exec -T server sh -c 'find .cache -type f -newer /tmp/smoke-cache-marker'
	}
	compose_srv exec -T server touch /tmp/smoke-cache-marker >/dev/null
else
	echo "   缓存探针面 = 宿主 $ROOT/server/.cache(dev 轨)"
	cache_count() {
		find "$ROOT/server/.cache" -type f 2>/dev/null | wc -l | tr -d ' '
	}
	cache_newer() {
		find "$ROOT/server/.cache" -type f -newer "$TMP/cache-marker" 2>/dev/null
	}
	touch "$TMP/cache-marker"
fi

CACHE_COUNT="$(cache_count)"
[ "$CACHE_COUNT" -gt 0 ] || die "首次渲染后应有物化缓存文件(徽标下载/QR 物化)"
[ -z "$(cache_newer)" ] || die "触碰前不应有新缓存写入"

_status=$(request POST "/api/templates/$SEED_ID/render" application/json /dev/null "$TMP/render2.json")
[ "$_status" = "201" ] || die "二次渲染期望 201,实得 $_status"
assert_json "$TMP/render2.json" "d['id'] != $(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["id"])' "$TMP/render1.json")"
CACHE_COUNT2="$(cache_count)"
[ "$CACHE_COUNT2" = "$CACHE_COUNT" ] || die "二次渲染不应新增缓存: $CACHE_COUNT → $CACHE_COUNT2(命中即跳过)"
NEWER="$(cache_newer)"
[ -z "$NEWER" ] || die "二次渲染有缓存文件被改写: $NEWER"

for u in $RENDER1_URLS; do
	_ct="$(curl -fsS -o /dev/null -w '%{content_type}' "$BASE$u")" || die "旧渲染 GET $u 失败(keep-all 不清理)"
	[ "$_ct" = "image/png" ] || die "旧渲染 GET $u Content-Type = $_ct, 期望 image/png"
done

# PDF 产物轨(PDF 集成共识,单轨):{"format":"pdf"} → 201 images 单条目
# document.pdf,直链 application/pdf + %PDF 魔数非空;format 非法值 400
# invalid_json。轻断言口径:字形/点位级 parity 归 go-canvas 包 CI,example 不装
# poppler。同轮验 PDF 轨缓存命中——物化器与位图轨同源,徽标/QR 已由前面渲染
# 物化,PDF 渲染零新增缓存
printf '{"format":"pdf"}' > "$TMP/render-pdf-body.json"
_status=$(request POST "/api/templates/$SEED_ID/render" application/json "$TMP/render-pdf-body.json" "$TMP/render-pdf.json")
[ "$_status" = "201" ] || die "POST render format=pdf 期望 201,实得 $_status"
assert_json "$TMP/render-pdf.json" "d['images'] == [{'frame': 0, 'name': 'document.pdf', 'url': f\"/renders/{d['id']}/document.pdf\"}]"
PDF_URL="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["images"][0]["url"])' "$TMP/render-pdf.json")"
_ct="$(curl -fsS -o "$TMP/render-pdf.bin" -w '%{content_type}' "$BASE$PDF_URL")" || die "GET $PDF_URL 失败"
[ "$_ct" = "application/pdf" ] || die "GET $PDF_URL Content-Type = $_ct, 期望 application/pdf"
[ -s "$TMP/render-pdf.bin" ] || die "PDF 产物为空"
head -c 5 "$TMP/render-pdf.bin" | grep -q '^%PDF-' || die "PDF 产物缺 %PDF- 魔数"

printf '{"format":"jpg"}' > "$TMP/render-badfmt.json"
expect_error POST "/api/templates/$SEED_ID/render" application/json "$TMP/render-badfmt.json" 400 invalid_json

CACHE_COUNT_PDF="$(cache_count)"
# 零增量断言适用域 = 本种子语料轮(徽标/QR 已被前轮渲染物化);反例域 = 含未
# 物化远程资产的文档,其 PDF 渲染会新增缓存,本断言不适用
[ "$CACHE_COUNT_PDF" = "$CACHE_COUNT2" ] || die "PDF 渲染不应新增缓存(本种子语料轮内,物化器与位图轨同源): $CACHE_COUNT2 → $CACHE_COUNT_PDF"

echo "==9/16 POST /api/templates/{不存在的id}/render(404) =="
expect_error POST "/api/templates/999999/render" application/json /dev/null 404 template_not_found

# ---- 断言 12(卡片操作修订):DELETE /api/templates/{id} → 204 无响应体;
# 渲染记录行级联清;产物文件保留(keep-all 快照直链仍可用);再删/非整数 id 404 ----

echo "==12/16 DELETE /api/templates/{id}(删除 → 204,记录级联清,产物文件保留) =="
cat > "$TMP/t3.json" <<'EOF'
{"name":"冒烟删除模板","pages":[{"id":"pg_smoke_del","name":"单页","graph":{"canvas":{"width":794,"height":1123},"layers":[]}}]}
EOF
_status=$(request POST /api/templates application/json "$TMP/t3.json" "$TMP/t3-resp.json")
[ "$_status" = "201" ] || die "POST /api/templates(删除用)期望 201,实得 $_status"
T3="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["id"])' "$TMP/t3-resp.json")"
echo "   T3=$T3"

# 先发布 T3(publish v1):缺省渲染语义 = 最新已发布(明示 breaking change),
# 未发布模板渲染 409——渲染断言前先固化 v1,顺带覆盖新模板 publish v1 路径
_status=$(request POST "/api/templates/$T3/publish" application/json /dev/null "$TMP/t3-pub.json")
[ "$_status" = "201" ] || die "POST publish(T3) 期望 201,实得 $_status"
assert_json "$TMP/t3-pub.json" "d['version'] == 1 and isinstance(d['createdAt'], str)"

# 先渲染一次:造渲染记录行 + 落盘产物(单页无 flow = 空链直通 1 页,无外网依赖)
_status=$(request POST "/api/templates/$T3/render" application/json /dev/null "$TMP/render3.json")
[ "$_status" = "201" ] || die "删除前渲染期望 201,实得 $_status"
RENDER3_URL="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["images"][0]["url"])' "$TMP/render3.json")"

_status=$(curl -sS -o "$TMP/del.out" -w '%{http_code}' -X DELETE "$BASE/api/templates/$T3")
[ "$_status" = "204" ] || die "DELETE /api/templates/$T3 期望 204,实得 $_status"
if [ -s "$TMP/del.out" ]; then die "DELETE 响应体应为空"; fi

# 模板行与渲染记录行级联清:GET/render 寻址 404;版本行随模板级联清(versions
# 寻址随模板消失,事务级联细节由 store 单测钉死);产物文件保留直链仍服务
expect_error GET "/api/templates/$T3" application/json /dev/null 404 template_not_found
expect_error GET "/api/templates/$T3/versions" application/json /dev/null 404 template_not_found
expect_error POST "/api/templates/$T3/render" application/json /dev/null 404 template_not_found
_ct="$(curl -fsS -o /dev/null -w '%{content_type}' "$BASE$RENDER3_URL")" || die "已删模板产物 GET $RENDER3_URL 失败"
[ "$_ct" = "image/png" ] || die "已删模板产物 Content-Type = $_ct, 期望 image/png"
expect_error DELETE "/api/templates/$T3" application/json /dev/null 404 template_not_found
expect_error DELETE "/api/templates/abc" application/json /dev/null 404 template_not_found

# ---- 断言 13(28 票):DELETE /api/datasources/{id} —— 被模板引用 409
# data_source_in_use(引用不悬空)→ 解绑 → 204 无响应体 → 行消失 404;再删与
# 非整数 id 404 ----

echo "==13/16 DELETE /api/datasources/{id}(被引用 409 → 解绑 → 204,404) =="
cat > "$TMP/ds-del.json" <<'EOF'
{"name":"冒烟删除数据源","schema":{"type":"object"},"data":{"k":"v"}}
EOF
_status=$(request POST /api/datasources application/json "$TMP/ds-del.json" "$TMP/ds-del-resp.json")
[ "$_status" = "201" ] || die "POST /api/datasources(删除用)期望 201,实得 $_status"
DS2="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["id"])' "$TMP/ds-del-resp.json")"
echo "   DS2=$DS2"

# 先绑定 T1 → 删除被 409 拒(引用不悬空)
cat > "$TMP/bind2.json" <<EOF
{"dataSourceId":$DS2}
EOF
_status=$(request PUT "/api/templates/$T1/datasource" application/json "$TMP/bind2.json" "$TMP/bind2-resp.json")
[ "$_status" = "200" ] || die "PUT …/datasource(绑定 DS2)期望 200,实得 $_status"
expect_error DELETE "/api/datasources/$DS2" application/json /dev/null 409 data_source_in_use

# 解绑 → 204 无响应体;行消失 GET/再删/非整数 id 404
cat > "$TMP/unbind2.json" <<'EOF'
{"dataSourceId":null}
EOF
_status=$(request PUT "/api/templates/$T1/datasource" application/json "$TMP/unbind2.json" "$TMP/unbind2-resp.json")
[ "$_status" = "200" ] || die "PUT …/datasource(解绑 DS2)期望 200,实得 $_status"
_status=$(curl -sS -o "$TMP/ds-del.out" -w '%{http_code}' -X DELETE "$BASE/api/datasources/$DS2")
[ "$_status" = "204" ] || die "DELETE /api/datasources/$DS2 期望 204,实得 $_status"
if [ -s "$TMP/ds-del.out" ]; then die "DELETE 数据源响应体应为空"; fi
expect_error GET "/api/datasources/$DS2" application/json /dev/null 404 data_source_not_found
expect_error DELETE "/api/datasources/$DS2" application/json /dev/null 404 data_source_not_found
expect_error DELETE "/api/datasources/abc" application/json /dev/null 404 data_source_not_found

# ---- 断言 14(网络字体方案):GET /api/fonts —— seed/fonts.json 白名单只读下发,
# 条目 {label, ref} 且 ref 为 https 直链(清单 ≠ 物化,服务端不下发字体字节) ----

echo "==14/16 GET /api/fonts(字体清单白名单) =="
get /api/fonts "$TMP/fonts.json"
assert_json "$TMP/fonts.json" 'isinstance(d, list) and len(d) >= 1'
assert_json "$TMP/fonts.json" 'all(set(f.keys()) == {"label", "ref"} and f["label"] and f["ref"].startswith("https://") for f in d)'

# ---- 断言 15 版本化轨(模板草稿-发布-版本化,spec §7 九断言):基于 SEED_ID。
# 前置:重置运行库(见头部注记)。启动存量回填把 seed 草稿列原样固化为 v1
# (pages 存储写侧恒经 marshal 归一:播种、触碰回写、回滚产出的草稿文本逐字节
# 相同),故未改草稿的发布命中幂等 duplicate v1;改写草稿后才首发 v2。
# 复跑不重置时版本号续增,本轨断言失真(依赖重置) ----

echo "==15/16 版本化轨:publish / versions / 渲染选版 / revert(spec §7 九断言) =="

# (1) 未改草稿发布 → 200 duplicate v1:草稿列文本与回填 v1 字节相等,幂等跳过
_status=$(request POST "/api/templates/$SEED_ID/publish" application/json /dev/null "$TMP/pub0.json")
[ "$_status" = "200" ] || die "未改草稿 publish 期望 200 duplicate,实得 $_status"
assert_json "$TMP/pub0.json" "d['version'] == 1 and d['duplicate'] is True and isinstance(d['createdAt'], str)"

# (2) 改写草稿(单页 pages 载荷,页 id 换新)→ publish 首发新版本 201 → v2
# (201 新版本不带 duplicate 键)
cat > "$TMP/seed-rewrite.json" <<'EOF'
{"name":"结业证书 · 批量打印页","pages":[{"id":"pg_smoke_rewrite","name":"冒烟草稿改写页","graph":{"canvas":{"width":794,"height":1123},"layers":[]}}]}
EOF
_status=$(request PUT "/api/templates/$SEED_ID" application/json "$TMP/seed-rewrite.json" "$TMP/seed-rewritten.json")
[ "$_status" = "200" ] || die "改写 seed 草稿期望 200,实得 $_status"
_status=$(request POST "/api/templates/$SEED_ID/publish" application/json /dev/null "$TMP/pub1.json")
[ "$_status" = "201" ] || die "publish(SEED) 期望 201,实得 $_status"
assert_json "$TMP/pub1.json" "d['version'] == 2 and isinstance(d['createdAt'], str) and 'duplicate' not in d"
PUB1_CREATED="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["createdAt"])' "$TMP/pub1.json")"

# (3) 重复 publish 200 duplicate:true(草稿与 v2 字节相等,幂等跳过,createdAt 沿用)
_status=$(request POST "/api/templates/$SEED_ID/publish" application/json /dev/null "$TMP/pub2.json")
[ "$_status" = "200" ] || die "重复 publish 期望 200,实得 $_status"
assert_json "$TMP/pub2.json" "d['version'] == 2 and d['duplicate'] is True and d['createdAt'] == '$PUB1_CREATED'"

# (4) GET versions 含 v1/v2,version 降序,轻量两字段(不吐 pages);单版本全量
# {version,pages,createdAt} —— 快照 = pages 整体原样,页 id 在版本行内(spec §3)
get "/api/templates/$SEED_ID/versions" "$TMP/versions.json"
assert_json "$TMP/versions.json" '[v["version"] for v in d] == [2, 1]'
assert_json "$TMP/versions.json" "all(set(v.keys()) == {'version', 'createdAt'} for v in d)"
get "/api/templates/$SEED_ID/versions/1" "$TMP/v1-full.json"
assert_json "$TMP/v1-full.json" "set(d.keys()) == {'version', 'pages', 'createdAt'} and d['version'] == 1"
assert_json "$TMP/v1-full.json" "[p['id'] for p in d['pages']] == ['pg_main', 'pg_cont'] and [p['name'] for p in d['pages']] == ['主页', '续页']"

# (5) 无 body 渲染 = 最新已发布 v2(改写单页稿 → 1 页),渲染记录带 version 字段
_status=$(request POST "/api/templates/$SEED_ID/render" application/json /dev/null "$TMP/render-latest.json")
[ "$_status" = "201" ] || die "无 body 渲染期望 201,实得 $_status"
assert_json "$TMP/render-latest.json" "d['version'] == 2 and len(d['images']) == 1"
assert_json "$TMP/render-latest.json" "[(i['frame'], i['name']) for i in d['images']] == [(0, '冒烟草稿改写页')]"

# (6) {"version":1} 显式渲染历史版本(= 种子页序列),记录 version = 1,
# 页归属按页序编号:frame 字段名不变,语义自然延续为页下标(ADR 0025)
cat > "$TMP/render-v1-req.json" <<'EOF'
{"version":1}
EOF
_status=$(request POST "/api/templates/$SEED_ID/render" application/json "$TMP/render-v1-req.json" "$TMP/render-v1.json")
[ "$_status" = "201" ] || die "显式 v1 渲染期望 201,实得 $_status"
assert_json "$TMP/render-v1.json" "d['version'] == 1 and len(d['images']) == 3"
assert_json "$TMP/render-v1.json" "[(i['frame'], i['name']) for i in d['images']] == [(0, '主页'), (1, '续页'), (1, '续页')]"

# (7) {"version":"draft"} 哨兵渲染草稿列现值(= v2 内容),记录 version null
cat > "$TMP/render-draft-req.json" <<'EOF'
{"version":"draft"}
EOF
_status=$(request POST "/api/templates/$SEED_ID/render" application/json "$TMP/render-draft-req.json" "$TMP/render-draft.json")
[ "$_status" = "201" ] || die "draft 哨兵渲染期望 201,实得 $_status"
assert_json "$TMP/render-draft.json" "d['version'] is None and len(d['images']) == 1"

# (8) 新建未发布模板渲染 409 template_unpublished(状态冲突型);
# {"version":99} 指定不存在的版本 → 404 version_not_found
cat > "$TMP/t4.json" <<'EOF'
{"name":"冒烟未发布模板","pages":[{"id":"pg_smoke_unpub","name":"单页","graph":{"canvas":{"width":794,"height":1123},"layers":[]}}]}
EOF
_status=$(request POST /api/templates application/json "$TMP/t4.json" "$TMP/t4-resp.json")
[ "$_status" = "201" ] || die "POST /api/templates(未发布渲染用)期望 201,实得 $_status"
T4="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["id"])' "$TMP/t4-resp.json")"
echo "   T4=$T4"
expect_error POST "/api/templates/$T4/render" application/json /dev/null 409 template_unpublished
cat > "$TMP/render-99-req.json" <<'EOF'
{"version":99}
EOF
expect_error POST "/api/templates/$SEED_ID/render" application/json "$TMP/render-99-req.json" 404 version_not_found

# (9) revert 后 GET /templates/{id} 的 pages 与目标版本一致(版本线线性,回滚
# 不造版):当前草稿 = 改写单页稿,revert v1 回种子页序列,全量记录与 v1 版本行
# 对齐(含页 id);回滚不刷新版本线
cat > "$TMP/revert-req.json" <<'EOF'
{"version":1}
EOF
_status=$(request POST "/api/templates/$SEED_ID/revert" application/json "$TMP/revert-req.json" "$TMP/revert-resp.json")
[ "$_status" = "200" ] || die "revert v1 期望 200,实得 $_status"
get "/api/templates/$SEED_ID" "$TMP/seed-after-revert.json"
python3 - "$TMP/v1-full.json" "$TMP/seed-after-revert.json" <<'PYEOF'
import json, sys
v1 = json.load(open(sys.argv[1], encoding="utf-8"))
cur = json.load(open(sys.argv[2], encoding="utf-8"))
assert cur["pages"] == v1["pages"], \
    "revert 后 pages 应与版本 1 一致:\n%s\nvs\n%s" % (
        json.dumps(cur["pages"], ensure_ascii=False)[:400],
        json.dumps(v1["pages"], ensure_ascii=False)[:400])
print("   revert 内容一致断言通过(pages 含页 id 逐字节对齐)")
PYEOF
get "/api/templates/$SEED_ID/versions" "$TMP/versions-after-revert.json"
assert_json "$TMP/versions-after-revert.json" '[v["version"] for v in d] == [2, 1]'

# ---- 断言 16 渲染历史(渲染历史对比):基于 SEED_ID。组 15 已对 seed 渲染三次
# (latest v2 / 显式 v1 / draft),历史列表应有 ≥ 3 条,id 降序,最新为 draft 行
# (version null);无记录模板回 200 空数组 ----

echo "== 16/16 渲染历史:GET /api/templates/{id}/renders =="
get "/api/templates/$SEED_ID/renders" "$TMP/renders-history.json"
assert_json "$TMP/renders-history.json" 'isinstance(d, list) and len(d) >= 3'
assert_json "$TMP/renders-history.json" 'all(r["id"] < r0["id"] for r0, r in zip(d, d[1:]))'  # id 降序（最新在前）
assert_json "$TMP/renders-history.json" "all(set(r.keys()) == {'id', 'templateId', 'version', 'createdAt', 'images'} and r['templateId'] == $SEED_ID and isinstance(r['createdAt'], str) and isinstance(r['images'], list) and len(r['images']) >= 1 and all(img['url'].startswith('/renders/') for img in r['images']) for r in d)"
assert_json "$TMP/renders-history.json" 'd[0]["version"] is None'  # 最新一条 = 组 15(6) 的 draft 渲染
assert_json "$TMP/renders-history.json" '[r["version"] for r in d[1:3]] == [1, 2]'  # 次序 = 显式 v1 → 缺省 v2
# 无记录模板(T4,组 15(7) 新建未发布)→ 200 空数组
get "/api/templates/$T4/renders" "$TMP/renders-empty.json"
assert_json "$TMP/renders-empty.json" 'd == []'

echo "smoke: 页序列契约断言 1–14 + 版本化轨 9 断言 + 渲染历史 6 断言全绿 (BASE=$BASE)"
