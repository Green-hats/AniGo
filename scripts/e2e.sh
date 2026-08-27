#!/bin/bash
# anigo 端到端全链路集成测试
# 覆盖: 基础API / 前端页面 / 配置 / AI / 元数据 / RSS解析 / 订阅管理 / 115下载 / 通知 / 导出导入 / 删除
#
# 用法: ./scripts/e2e.sh [端口] [hook端口]
# 依赖: curl / python3 / unzip

set -e
PORT="${1:-7810}"
HOOK_PORT="${2:-7811}"
BIN=/tmp/anigo-e2e
CFG=/tmp/anigo-e2e-test
rm -rf "$CFG"

PASS=0
FAIL=0

check() {
  local desc="$1"; local want="$2"; local got="$3"
  if [ "$got" = "$want" ]; then
    echo "  ✓ $desc"
    PASS=$((PASS+1))
  else
    echo "  ✗ $desc (期望 $want, 实际 $got)"
    FAIL=$((FAIL+1))
  fi
}

# 定位项目根目录
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

cd "$ROOT/backend"
go build -o "$BIN" ./cmd/anigo
CONFIG="$CFG" PORT=$PORT "$BIN" >/tmp/anigo-e2e.log 2>&1 &
SRV=$!
sleep 2

echo "=== 0. 登录 ==="
TOKEN=$(curl -s -X POST http://127.0.0.1:$PORT/api/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"admin"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["token"])')
check "登录获取 token" "1" "$(echo "$TOKEN" | python3 -c 'import sys;print(1 if len(sys.stdin.read().strip())>0 else 0)')"
AUTH="Authorization: Bearer $TOKEN"

echo "=== 1. 基础 API ==="
check "ping" "200" "$(curl -s http://127.0.0.1:$PORT/api/ping | python3 -c 'import sys,json;print(json.load(sys.stdin)["code"])')"

echo "=== 2. 前端页面 (单二进制) ==="
check "首页 title" "AniGo - 云端追番" "$(curl -s http://127.0.0.1:$PORT/ | grep -oP '(?<=<title>)[^<]*')"
check "SPA 路由兜底" "200" "$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:$PORT/home)"

echo "=== 3. 配置 ==="
check "默认配置读取" "deepseek-v4-flash" "$(curl -s -H "$AUTH" -X POST http://127.0.0.1:$PORT/api/config | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["aiModel"])')"
check "setConfig 修改" "30" "$(curl -s -H "$AUTH" -X POST http://127.0.0.1:$PORT/api/setConfig -H 'Content-Type: application/json' -d '{"rssSleepMinutes":30}' >/dev/null; curl -s -H "$AUTH" -X POST http://127.0.0.1:$PORT/api/config | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["rssSleepMinutes"])')"
check "未登录访问被拒" "401" "$(curl -s -X POST http://127.0.0.1:$PORT/api/config | python3 -c 'import sys,json;print(json.load(sys.stdin)["code"])')"

echo "=== 4. AI 连通 ==="
check "aiPing" "ok" "$(curl -s --max-time 30 -H "$AUTH" -X POST http://127.0.0.1:$PORT/api/aiPing | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["reply"])')"

echo "=== 5. 元数据 ==="
check "searchBgm" "12" "$(curl -s --max-time 30 -H "$AUTH" -X POST http://127.0.0.1:$PORT/api/searchBgm -H 'Content-Type: application/json' -d '{"text":"间谍过家家"}' | python3 -c 'import sys,json;print(len(json.load(sys.stdin)["data"]))')"
check "gardenList 周数" "7" "$(curl -s --max-time 30 -H "$AUTH" -X POST http://127.0.0.1:$PORT/api/gardenList | python3 -c 'import sys,json;print(len(json.load(sys.stdin)["data"]))')"

echo "=== 6. RSS 解析 + AI 剧集提取 ==="
check "previewAni 条数>0" "1" "$(curl -s --max-time 60 -H "$AUTH" -X POST http://127.0.0.1:$PORT/api/previewAni -H 'Content-Type: application/json' -d '{"title":"间谍过家家","season":1,"subgroup":"ANi","url":"https://api.animes.garden/feed.xml?subject=477825&fansub=ANi"}' | python3 -c 'import sys,json;d=json.load(sys.stdin)["data"];print(1 if len(d["items"])>0 else 0)')"

echo "=== 7. 订阅管理 ==="
ANI_ID=$(curl -s --max-time 30 -H "$AUTH" -X POST http://127.0.0.1:$PORT/api/rssToAni -H 'Content-Type: application/json' -d '{"url":"https://api.animes.garden/feed.xml?subject=477825&fansub=ANi","type":"garden"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["id"])')
check "rssToAni 创建订阅" "1" "$(echo $ANI_ID | python3 -c 'import sys;print(1 if len(sys.stdin.read().strip())>0 else 0)')"
curl -s -H "$AUTH" -X POST http://127.0.0.1:$PORT/api/addAni -H 'Content-Type: application/json' -d "{\"id\":\"$ANI_ID\",\"title\":\"间谍过家家\",\"season\":1,\"enable\":true}" >/dev/null
check "addAni 添加" "1" "$(curl -s -H "$AUTH" -X POST http://127.0.0.1:$PORT/api/listAni | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["total"])')"
curl -s -H "$AUTH" -X POST "http://127.0.0.1:$PORT/api/batchEnable?value=false" -H 'Content-Type: application/json' -d "[\"$ANI_ID\"]" >/dev/null
check "batchEnable 停用" "False" "$(curl -s -H "$AUTH" -X POST http://127.0.0.1:$PORT/api/listAni | python3 -c 'import sys,json;d=json.load(sys.stdin)["data"];[print(a["enable"]) for w in d["weekList"] for a in w["items"]]')"
check "下载路径模板" "番剧/间谍过家家/Season 1" "$(curl -s -H "$AUTH" -X POST http://127.0.0.1:$PORT/api/downloadPath -H 'Content-Type: application/json' -d '{"title":"间谍过家家","season":1}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["downloadPath"])')"

echo "=== 8. 下载 (115 登录) ==="
check "115 登录状态" "True" "$(curl -s -H "$AUTH" -X POST http://127.0.0.1:$PORT/api/downloadStatus | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["loginOK"])')"
check "115 登录测试" "登录成功" "$(curl -s --max-time 30 -H "$AUTH" -X POST http://127.0.0.1:$PORT/api/downloadLoginTest -H 'Content-Type: application/json' -d '{}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["message"])')"

echo "=== 9. 通知 (WebHook 本地接收) ==="
python3 -c "
import http.server
class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        self.send_response(200); self.end_headers()
    def log_message(self, *a): pass
http.server.HTTPServer(('127.0.0.1', $HOOK_PORT), H).serve_forever()
" >/tmp/anigo-hook.log 2>&1 &
HOOK=$!
sleep 1
check "testNotification" "发送成功" "$(curl -s --max-time 10 -H "$AUTH" -X POST http://127.0.0.1:$PORT/api/testNotification -H 'Content-Type: application/json' -d '{"enable":true,"retry":1,"comment":"t","notificationType":"WEB_HOOK","webHookMethod":"POST","webHookUrl":"http://127.0.0.1:'$HOOK_PORT'/hook","notificationTemplate":"${text}","statusList":["ERROR"],"sort":1}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["message"])')"

echo "=== 10. 导出/导入配置 ==="
curl -s -H "$AUTH" -o /tmp/anigo-backup.zip -w "%{http_code}" http://127.0.0.1:$PORT/api/exportConfig >/tmp/export_code.txt
check "导出配置 zip" "200" "$(cat /tmp/export_code.txt)"
check "zip 含配置文件" "1" "$(unzip -l /tmp/anigo-backup.zip 2>/dev/null | grep -c 'config.v2.json')"
check "导入配置" "导入成功" "$(curl -s -H "$AUTH" -X POST http://127.0.0.1:$PORT/api/importConfig -F 'file=@/tmp/anigo-backup.zip' | python3 -c 'import sys,json;print(json.load(sys.stdin)["message"])')"

echo "=== 11. 删除订阅 ==="
check "deleteAni" "0" "$(curl -s -H "$AUTH" -X POST http://127.0.0.1:$PORT/api/deleteAni -H 'Content-Type: application/json' -d "[\"$ANI_ID\"]" >/dev/null; curl -s -H "$AUTH" -X POST http://127.0.0.1:$PORT/api/listAni | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["total"])')"

kill $SRV $HOOK 2>/dev/null
rm -f /tmp/anigo-backup.zip /tmp/export_code.txt

echo ""
echo "====================================="
echo "通过: $PASS  失败: $FAIL"
echo "====================================="
[ $FAIL -eq 0 ] && echo "✅ 全部通过" || echo "❌ 有失败项"
exit $FAIL