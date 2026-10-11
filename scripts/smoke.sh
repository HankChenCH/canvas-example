#!/bin/sh
# spec §6.2 curl 冒烟(契约面),双轨通用:compose 轨(cd example && docker compose up
# --build)或 dev 轨(cd example/server && go run .)任一起服务后运行。
# 用法: scripts/smoke.sh [BASE],默认 http://localhost:8080(compose 轨验收口径)。
# 断言 1–3(11 票)读路径;4–7、10、11(12 票)写端点/上传/上限;6 段含数据源
# 实体 CRUD + 绑定通道(23 票);8–9(13 票)渲染管线 + keep-all + .cache 生效;
# 12(卡片操作修订 + 版本化)DELETE /api/templates/{id}:渲染前先 publish(缺省
# 渲染语义 = 最新已发布,未发布 409,spec §4);204 + 渲染记录级联清 + 版本行
# 级联清 + 产物文件保留;
# 13(28 票)DELETE /api/datasources/{id}:被引用 409 data_source_in_use → 解绑 →
# 204 → 404 闭环;14(网络字体方案)GET /api/fonts:清单白名单 {label, ref} 直链;
# 15 版本化轨(模板草稿-发布-版本化,spec §7 九断言):publish 201 v2 / 重复
# publish 200 duplicate / GET versions / 无 body 渲染 = 最新已发布带 version 字段 /
# 显式选版 / draft 哨兵 / 未发布 409 / version_not_found 404 / revert 内容一致。
# 任一断言失败非零退出(spec §6.2)。出网注意:渲染走 picsum 徽标(spec §5.3)。
# 注意:断言 3/8 依赖种子模板仍绑种子数据源(3 页容量数学)——UI 里重绑/改数据源
# 后会失真,属预期;版本化轨断言依赖重置运行库(存量回填使 seed 持 v1,本轨首发
# publish 拿 v2;发布幂等断言在复跑时变 duplicate),重置:停服删 server/data/app.db
# 重启(播种自动补回)。
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

echo "== 1/13 GET /api/health =="
get /api/health "$TMP/health.json"
assert_json "$TMP/health.json" 'd.get("status") == "ok"'

echo "== 2/13 GET /api/templates =="
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

echo "== 3/13 GET /api/templates/{SEED_ID} =="
get "/api/templates/$SEED_ID" "$TMP/template.json"
assert_json "$TMP/template.json" 'len(d["canvases"]) == 2 and [c["name"] for c in d["canvases"]] == ["主页", "续页"]'
assert_json "$TMP/template.json" 'isinstance(d["flowChain"], list) and len(d["flowChain"]) == 2'
# 数据源是独立实体:模板持引用(spec §2.4 数据源段),引用的实体可回读且含 schema/data
assert_json "$TMP/template.json" 'isinstance(d["dataSourceId"], int) and d["dataSourceId"] > 0'
DS_ID="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["dataSourceId"])' "$TMP/template.json")"
get "/api/datasources/$DS_ID" "$TMP/ds.json"
assert_json "$TMP/ds.json" 'd["schema"] is not None and d["data"] is not None'
assert_json "$TMP/ds.json" 'd["data"]["certificates"] and len(d["data"]["certificates"]) == 5'

echo "== 4/13 POST /api/templates(最小模板 → 201 自增 id,未绑数据源) =="
cat > "$TMP/min-template.json" <<'EOF'
{"name":"冒烟最小模板","canvases":[{"name":"单页","graph":{"canvas":{"width":794,"height":1123},"layers":[]}}]}
EOF
_status=$(request POST /api/templates application/json "$TMP/min-template.json" "$TMP/t1.json")
[ "$_status" = "201" ] || die "POST /api/templates 期望 201,实得 $_status"
assert_json "$TMP/t1.json" "isinstance(d['id'], int) and d['id'] > $SEED_ID"
assert_json "$TMP/t1.json" "d['createdAt'] == d['updatedAt']"
assert_json "$TMP/t1.json" "d['dataSourceId'] is None and d['flowChain'] is None"
T1="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["id"])' "$TMP/t1.json")"
echo "   T1=$T1"

echo "== 5/13 PUT /api/templates/{T1}(改 name;绑定引用不被触碰) =="
cat > "$TMP/put-t1.json" <<'EOF'
{"name":"冒烟改名模板","canvases":[{"name":"单页","graph":{"canvas":{"width":794,"height":1123},"layers":[]}}]}
EOF
_status=$(request PUT "/api/templates/$T1" application/json "$TMP/put-t1.json" "$TMP/t1-put.json")
[ "$_status" = "200" ] || die "PUT /api/templates/$T1 期望 200,实得 $_status"
assert_json "$TMP/t1-put.json" "d['name'] == '冒烟改名模板'"
assert_json "$TMP/t1-put.json" "d['dataSourceId'] is None"
assert_json "$TMP/t1-put.json" "d['flowChain'] is None"

echo "== 6/13 数据源实体 CRUD + 绑定通道(CRUD 校验码 / 绑定 / 解绑 / 404) =="
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
{"name":"冒烟随建绑定模板","canvases":[{"name":"单页","graph":{"canvas":{"width":794,"height":1123},"layers":[]}}],"dataSourceId":$DS1}
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

echo "== 7/13 POST /api/templates 保存预检打回(未知图层类型 / 双 paged 链) =="
cat > "$TMP/bad-layer.json" <<'EOF'
{"name":"坏图层模板","canvases":[{"name":"单页","graph":{"canvas":{"width":794,"height":1123},"layers":[{"type":"GhostLayer","priority":0}]}}]}
EOF
expect_error POST /api/templates application/json "$TMP/bad-layer.json" 400 unknown_layer_type

cat > "$TMP/double-paged.json" <<'EOF'
{"name":"双paged模板","canvases":[
  {"name":"帧一","graph":{"canvas":{"width":794,"height":1123},"layers":[]}},
  {"name":"帧二","graph":{"canvas":{"width":794,"height":1123},"layers":[]}}],
 "flowChain":[{"frame":0,"mode":"paged"},{"frame":1,"mode":"paged"}]}
EOF
expect_error POST /api/templates application/json "$TMP/double-paged.json" 400 flow_chain_invalid

echo "== 10/13 POST /api/assets(multipart 上传 → url 形态 + 可 GET) =="
PNG="$ROOT/server/seed/assets/u/student-1.png"
[ -f "$PNG" ] || die "上传样张缺失: $PNG"
_status=$(curl -sS -o "$TMP/upload.json" -w '%{http_code}' -F "file=@$PNG;type=image/png" "$BASE/api/assets")
[ "$_status" = "201" ] || die "POST /api/assets 期望 201,实得 $_status"
assert_json "$TMP/upload.json" "re.match(r'^/assets/u/[0-9a-f]{16}\.[A-Za-z0-9]{1,8}$', d['url']) is not None"
UPLOAD_URL="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["url"])' "$TMP/upload.json")"
curl -fsS -o "$TMP/upload-roundtrip.png" "$BASE$UPLOAD_URL" || die "GET $UPLOAD_URL 失败"
cmp -s "$TMP/upload-roundtrip.png" "$PNG" || die "上传回读字节与原文件不一致"

echo "== 11/13 请求体上限与坏 body(413 / 400) =="
python3 -c 'import json,sys; sys.stdout.write(json.dumps({"name":"a" * (10 * 1024 * 1024)}))' > "$TMP/big.json"
expect_error POST /api/templates application/json "$TMP/big.json" 413 request_too_large

printf 'not json' > "$TMP/not-json.json"
expect_error POST /api/templates application/json "$TMP/not-json.json" 400 invalid_json

# ---- 断言 8–9(13 票):渲染管线 → RenderRecord + PNG 直链 ----

echo "== 8/13 POST /api/templates/{SEED_ID}/render(渲染 → 201 RenderRecord) =="
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

# keep-all 不清理(spec §2.4 #7):PUT 触碰模板(内容原样回写)后再渲染,
# 旧 renderId 的 url 仍服务旧图;同时覆盖 .cache 生效——二次渲染不再下载
get "/api/templates/$SEED_ID" "$TMP/seed-before-touch.json"
python3 -c '
import json, sys
d = json.load(open(sys.argv[1], encoding="utf-8"))
json.dump({"name": d["name"], "canvases": d["canvases"], "flowChain": d["flowChain"]},
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

echo "== 9/13 POST /api/templates/{不存在的id}/render(404) =="
expect_error POST "/api/templates/999999/render" application/json /dev/null 404 template_not_found

# ---- 断言 12(卡片操作修订):DELETE /api/templates/{id} → 204 无响应体;
# 渲染记录行级联清;产物文件保留(keep-all 快照直链仍可用);再删/非整数 id 404 ----

echo "== 12/13 DELETE /api/templates/{id}(删除 → 204,记录级联清,产物文件保留) =="
cat > "$TMP/t3.json" <<'EOF'
{"name":"冒烟删除模板","canvases":[{"name":"单页","graph":{"canvas":{"width":794,"height":1123},"layers":[]}}]}
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

# 先渲染一次:造渲染记录行 + 落盘产物(单帧空链直通 1 页,无外网依赖)
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

echo "== 13/13 DELETE /api/datasources/{id}(被引用 409 → 解绑 → 204,404) =="
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

echo "== 14/14 GET /api/fonts(字体清单白名单) =="
get /api/fonts "$TMP/fonts.json"
assert_json "$TMP/fonts.json" 'isinstance(d, list) and len(d) >= 1'
assert_json "$TMP/fonts.json" 'all(set(f.keys()) == {"label", "ref"} and f["label"] and f["ref"].startswith("https://") for f in d)'

# ---- 断言 15 版本化轨(模板草稿-发布-版本化,spec §7 九断言):基于 SEED_ID。
# 前置:重置运行库后存量回填使 seed 持 v1(草稿列文本原样固化),本轨首发
# publish 产生 v2;复跑不重置时 (1) 变 200 duplicate,依赖重置(见头部注记) ----

echo "== 15/15 版本化轨:publish / versions / 渲染选版 / revert(spec §7 九断言) =="

# (1) publish 201 → v2(seed 经回填已有 v1);201 新版本不带 duplicate 键
_status=$(request POST "/api/templates/$SEED_ID/publish" application/json /dev/null "$TMP/pub1.json")
[ "$_status" = "201" ] || die "publish(SEED) 期望 201,实得 $_status"
assert_json "$TMP/pub1.json" "d['version'] == 2 and isinstance(d['createdAt'], str) and 'duplicate' not in d"
PUB1_CREATED="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["createdAt"])' "$TMP/pub1.json")"

# (2) 重复 publish 200 duplicate:true(草稿与 v2 字节相等,幂等跳过,createdAt 沿用)
_status=$(request POST "/api/templates/$SEED_ID/publish" application/json /dev/null "$TMP/pub2.json")
[ "$_status" = "200" ] || die "重复 publish 期望 200,实得 $_status"
assert_json "$TMP/pub2.json" "d['version'] == 2 and d['duplicate'] is True and d['createdAt'] == '$PUB1_CREATED'"

# (3) GET versions 含 v1/v2,version 降序,轻量两字段(不吐 canvases)
get "/api/templates/$SEED_ID/versions" "$TMP/versions.json"
assert_json "$TMP/versions.json" '[v["version"] for v in d] == [2, 1]'
assert_json "$TMP/versions.json" "all(set(v.keys()) == {'version', 'createdAt'} for v in d)"

# (4) 无 body 渲染 = 最新已发布 v2,渲染记录带 version 字段(明示 breaking change)
_status=$(request POST "/api/templates/$SEED_ID/render" application/json /dev/null "$TMP/render-latest.json")
[ "$_status" = "201" ] || die "无 body 渲染期望 201,实得 $_status"
assert_json "$TMP/render-latest.json" "d['version'] == 2 and len(d['images']) == 3"

# (5) {"version":1} 显式渲染历史版本,记录 version = 1
cat > "$TMP/render-v1-req.json" <<'EOF'
{"version":1}
EOF
_status=$(request POST "/api/templates/$SEED_ID/render" application/json "$TMP/render-v1-req.json" "$TMP/render-v1.json")
[ "$_status" = "201" ] || die "显式 v1 渲染期望 201,实得 $_status"
assert_json "$TMP/render-v1.json" "d['version'] == 1"

# (6) {"version":"draft"} 哨兵渲染草稿列现值,记录 version null(未对应发布版本)
cat > "$TMP/render-draft-req.json" <<'EOF'
{"version":"draft"}
EOF
_status=$(request POST "/api/templates/$SEED_ID/render" application/json "$TMP/render-draft-req.json" "$TMP/render-draft.json")
[ "$_status" = "201" ] || die "draft 哨兵渲染期望 201,实得 $_status"
assert_json "$TMP/render-draft.json" "d['version'] is None and len(d['images']) == 3"

# (7) 新建未发布模板渲染 409 template_unpublished(状态冲突型)
cat > "$TMP/t4.json" <<'EOF'
{"name":"冒烟未发布模板","canvases":[{"name":"单页","graph":{"canvas":{"width":794,"height":1123},"layers":[]}}]}
EOF
_status=$(request POST /api/templates application/json "$TMP/t4.json" "$TMP/t4-resp.json")
[ "$_status" = "201" ] || die "POST /api/templates(未发布渲染用)期望 201,实得 $_status"
T4="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["id"])' "$TMP/t4-resp.json")"
echo "   T4=$T4"
expect_error POST "/api/templates/$T4/render" application/json /dev/null 409 template_unpublished

# (8) {"version":99} 指定不存在的版本 → 404 version_not_found
cat > "$TMP/render-99-req.json" <<'EOF'
{"version":99}
EOF
expect_error POST "/api/templates/$SEED_ID/render" application/json "$TMP/render-99-req.json" 404 version_not_found

# (9) revert 后 GET /templates/{id} 的 canvases 与目标版本一致(版本线线性,
# 回滚不造版):先改写 seed 草稿,再 revert v1,全量记录内容与 v1 版本行对齐
get "/api/templates/$SEED_ID/versions/1" "$TMP/v1-full.json"
cat > "$TMP/seed-rewrite.json" <<'EOF'
{"name":"结业证书 · 批量打印页","canvases":[{"name":"冒烟草稿改写帧","graph":{"canvas":{"width":794,"height":1123},"layers":[]}}],"flowChain":null}
EOF
_status=$(request PUT "/api/templates/$SEED_ID" application/json "$TMP/seed-rewrite.json" "$TMP/seed-rewritten.json")
[ "$_status" = "200" ] || die "改写 seed 草稿期望 200,实得 $_status"
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
assert cur["canvases"] == v1["canvases"], \
    "revert 后 canvases 应与版本 1 一致:\n%s\nvs\n%s" % (
        json.dumps(cur["canvases"], ensure_ascii=False)[:400],
        json.dumps(v1["canvases"], ensure_ascii=False)[:400])
assert cur["flowChain"] == v1["flowChain"], "revert 后 flowChain 应与版本 1 一致"
print("   revert 内容一致断言通过")
PYEOF
get "/api/templates/$SEED_ID/versions" "$TMP/versions-after-revert.json"
assert_json "$TMP/versions-after-revert.json" '[v["version"] for v in d] == [2, 1]'

echo "smoke: 断言 1–14 + 版本化轨 9 断言全绿 (BASE=$BASE)"
