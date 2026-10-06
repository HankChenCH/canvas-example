#!/bin/sh
# spec §6.2 curl 冒烟(契约面),双轨通用:compose 轨(cd example && docker compose up
# --build)或 dev 轨(cd example/server && go run .)任一起服务后运行。
# 用法: scripts/smoke.sh [BASE],默认 http://localhost:8080(compose 轨验收口径)。
# 断言 1–3(11 票)读路径;4–7、10、11(12 票)写端点/上传/上限;
# 8–9(13 票)渲染管线 + keep-all + .cache 生效。
# 任一断言失败非零退出(spec §6.2)。出网注意:渲染走 picsum 徽标(spec §5.3)。
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

echo "== 1/11 GET /api/health =="
get /api/health "$TMP/health.json"
assert_json "$TMP/health.json" 'd.get("status") == "ok"'

echo "== 2/11 GET /api/templates =="
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

echo "== 3/11 GET /api/templates/{SEED_ID} =="
get "/api/templates/$SEED_ID" "$TMP/template.json"
assert_json "$TMP/template.json" 'len(d["canvases"]) == 2 and [c["name"] for c in d["canvases"]] == ["主页", "续页"]'
assert_json "$TMP/template.json" 'isinstance(d["flowChain"], list) and len(d["flowChain"]) == 2'
assert_json "$TMP/template.json" 'd["datasetSchema"] is not None and d["dataset"] is not None'

echo "== 4/11 POST /api/templates(最小模板 → 201 自增 id) =="
cat > "$TMP/min-template.json" <<'EOF'
{"name":"冒烟最小模板","canvases":[{"name":"单页","graph":{"canvas":{"width":794,"height":1123},"layers":[]}}]}
EOF
_status=$(request POST /api/templates application/json "$TMP/min-template.json" "$TMP/t1.json")
[ "$_status" = "201" ] || die "POST /api/templates 期望 201,实得 $_status"
assert_json "$TMP/t1.json" "isinstance(d['id'], int) and d['id'] > $SEED_ID"
assert_json "$TMP/t1.json" "d['createdAt'] == d['updatedAt']"
assert_json "$TMP/t1.json" "d['dataset'] is None and d['datasetSchema'] is None and d['flowChain'] is None"
T1="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["id"])' "$TMP/t1.json")"
echo "   T1=$T1"

echo "== 5/11 PUT /api/templates/{T1}(改 name;dataset 仍为 null) =="
cat > "$TMP/put-t1.json" <<'EOF'
{"name":"冒烟改名模板","canvases":[{"name":"单页","graph":{"canvas":{"width":794,"height":1123},"layers":[]}}]}
EOF
_status=$(request PUT "/api/templates/$T1" application/json "$TMP/put-t1.json" "$TMP/t1-put.json")
[ "$_status" = "200" ] || die "PUT /api/templates/$T1 期望 200,实得 $_status"
assert_json "$TMP/t1-put.json" "d['name'] == '冒烟改名模板'"
assert_json "$TMP/t1-put.json" "d['dataset'] is None and d['datasetSchema'] is None"
assert_json "$TMP/t1-put.json" "d['flowChain'] is None"

echo "== 6/11 PUT /api/templates/{T1}/dataset(合法 200 / 坏 schema 400 / 不过 schema 400) =="
cat > "$TMP/ds-ok.json" <<'EOF'
{"schema":{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","properties":{"student":{"type":"string"}},"required":["student"]},"data":{"student":"林晚晴"}}
EOF
_status=$(request PUT "/api/templates/$T1/dataset" application/json "$TMP/ds-ok.json" "$TMP/ds-ok-resp.json")
[ "$_status" = "200" ] || die "PUT dataset(合法)期望 200,实得 $_status"
assert_json "$TMP/ds-ok-resp.json" "d['dataset'] == {'student': '林晚晴'}"

cat > "$TMP/ds-bad-schema.json" <<'EOF'
{"schema":{"type":42},"data":{}}
EOF
expect_error PUT "/api/templates/$T1/dataset" application/json "$TMP/ds-bad-schema.json" 400 schema_invalid

cat > "$TMP/ds-mismatch.json" <<'EOF'
{"schema":{"type":"object"},"data":[1,2]}
EOF
expect_error PUT "/api/templates/$T1/dataset" application/json "$TMP/ds-mismatch.json" 400 dataset_schema_mismatch

# 票面第三条:数据源就位后 PUT 模板,验证 dataset/datasetSchema 仍在(spec §2.4 #5)
_status=$(request PUT "/api/templates/$T1" application/json "$TMP/put-t1.json" "$TMP/t1-put2.json")
[ "$_status" = "200" ] || die "数据源就位后 PUT 模板期望 200,实得 $_status"
assert_json "$TMP/t1-put2.json" "d['dataset'] == {'student': '林晚晴'}"
assert_json "$TMP/t1-put2.json" "d['datasetSchema'] is not None"

echo "== 7/11 POST /api/templates 保存预检打回(未知图层类型 / 双 paged 链) =="
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

echo "== 10/11 POST /api/assets(multipart 上传 → url 形态 + 可 GET) =="
PNG="$ROOT/server/seed/assets/u/student-1.png"
[ -f "$PNG" ] || die "上传样张缺失: $PNG"
_status=$(curl -sS -o "$TMP/upload.json" -w '%{http_code}' -F "file=@$PNG;type=image/png" "$BASE/api/assets")
[ "$_status" = "201" ] || die "POST /api/assets 期望 201,实得 $_status"
assert_json "$TMP/upload.json" "re.match(r'^/assets/u/[0-9a-f]{16}\.[A-Za-z0-9]{1,8}$', d['url']) is not None"
UPLOAD_URL="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["url"])' "$TMP/upload.json")"
curl -fsS -o "$TMP/upload-roundtrip.png" "$BASE$UPLOAD_URL" || die "GET $UPLOAD_URL 失败"
cmp -s "$TMP/upload-roundtrip.png" "$PNG" || die "上传回读字节与原文件不一致"

echo "== 11/11 请求体上限与坏 body(413 / 400) =="
python3 -c 'import json,sys; sys.stdout.write(json.dumps({"name":"a" * (10 * 1024 * 1024)}))' > "$TMP/big.json"
expect_error POST /api/templates application/json "$TMP/big.json" 413 request_too_large

printf 'not json' > "$TMP/not-json.json"
expect_error POST /api/templates application/json "$TMP/not-json.json" 400 invalid_json

# ---- 断言 8–9(13 票):渲染管线 → RenderRecord + PNG 直链 ----

echo "== 8/11 POST /api/templates/{SEED_ID}/render(渲染 → 201 RenderRecord) =="
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

echo "== 9/11 POST /api/templates/{不存在的id}/render(404) =="
expect_error POST "/api/templates/999999/render" application/json /dev/null 404 template_not_found

echo "smoke: 断言 1–11 全绿 (BASE=$BASE)"
