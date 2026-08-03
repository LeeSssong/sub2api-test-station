#!/bin/zsh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WORKBENCH_DIR="$SCRIPT_DIR/tools/local-workbench"
SESSION_NAME="local-workbench"
PORT="${PORT:-4318}"
WORKBENCH_DB="${WORKBENCH_DB:-$WORKBENCH_DIR/data/workbench.sqlite}"
URL="http://127.0.0.1:$PORT"

if curl -fsS --max-time 2 "$URL/api/batches" >/dev/null 2>&1; then
  open "$URL"
  echo "工作台已在运行：$URL"
  exit 0
fi

if ! command -v screen >/dev/null 2>&1; then
  echo "未找到 screen，请先安装或直接运行：" >&2
  echo "  cd \"$WORKBENCH_DIR\" && PORT=$PORT WORKBENCH_DB=\"$WORKBENCH_DB\" npm start" >&2
  exit 1
fi

screen -S "$SESSION_NAME" -X quit >/dev/null 2>&1 || true
screen -dmS "$SESSION_NAME" zsh -lc \
  "cd \"$WORKBENCH_DIR\" && exec env PORT=\"$PORT\" WORKBENCH_DB=\"$WORKBENCH_DB\" npm start"

for attempt in {1..10}; do
  if curl -fsS --max-time 2 "$URL/api/batches" >/dev/null 2>&1; then
    open "$URL"
    echo "工作台已启动：$URL"
    echo "后台会话：$SESSION_NAME"
    exit 0
  fi
  sleep 1
done

echo "工作台启动失败，请查看会话：screen -r $SESSION_NAME" >&2
exit 1
