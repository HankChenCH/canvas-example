#!/bin/sh
# spec §6.2 curl 冒烟(契约面):前置 dev server 跑起(cd example/server && go run .)。
# 用法: scripts/smoke.sh [BASE],默认 http://localhost:8080。
# 本票(11)交付骨架 + 断言 1–3;断言 4–11 随 12/13 票追加。
# 任一断言失败非零退出(spec §6.2)。
set -eu

BASE="${1:-http://localhost:8080}"

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
import json, sys
with open(sys.argv[1], encoding="utf-8") as f:
    d = json.load(f)
assert eval(sys.argv[2], {"d": d}), "断言失败: %s\n实际值: %s" % (sys.argv[2], json.dumps(d, ensure_ascii=False)[:600])
PYEOF
}

echo "== 1/11 GET /api/health =="
get /api/health "$TMP/health.json"
assert_json "$TMP/health.json" 'd.get("status") == "ok"'

echo "== 2/11 GET /api/templates =="
get /api/templates "$TMP/templates.json"
assert_json "$TMP/templates.json" 'isinstance(d, list) and len(d) >= 1'
assert_json "$TMP/templates.json" '[t["updatedAt"] for t in d] == sorted((t["updatedAt"] for t in d), reverse=True)'
assert_json "$TMP/templates.json" 'all(set(t.keys()) == {"id", "name", "updatedAt"} for t in d)'
SEED_ID="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))[0]["id"])' "$TMP/templates.json")"
echo "   SEED_ID=$SEED_ID"

echo "== 3/11 GET /api/templates/{SEED_ID} =="
get "/api/templates/$SEED_ID" "$TMP/template.json"
assert_json "$TMP/template.json" 'len(d["canvases"]) == 2 and [c["name"] for c in d["canvases"]] == ["主页", "续页"]'
assert_json "$TMP/template.json" 'isinstance(d["flowChain"], list) and len(d["flowChain"]) == 2'
assert_json "$TMP/template.json" 'd["datasetSchema"] is not None and d["dataset"] is not None'

# ---- 断言 4–11(12/13 票追加):写端点 / 渲染管线 / 上传 / 413 与坏 body ----

echo "smoke: 断言 1–3 全绿 (BASE=$BASE)"
