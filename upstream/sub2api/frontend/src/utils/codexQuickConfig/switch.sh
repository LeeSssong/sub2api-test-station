#!/bin/bash
set -euo pipefail
umask 077
export PATH="${PATH:-}:/usr/bin:/bin"
PAYLOAD='@@CONFIG@@'
CATALOG_PAYLOAD='@@CATALOG@@'
CONFIG_DIR="${CODEX_HOME:-${HOME}/.codex}"
STAGE=''
LOCK=''
BACKUP=''
CONFIG_CHANGED=0
CATALOG_CHANGED=0

finish() {
  status=$?
  trap - EXIT
  if [ "$status" -ne 0 ]; then
    if [ "$CONFIG_CHANGED" -eq 1 ]; then
      if [ -f "$BACKUP/config.toml" ]; then cp -p "$BACKUP/config.toml" "$CONFIG_DIR/config.toml" || printf '恢复失败，备份：%s\n' "$BACKUP" >&2
      else rm -f "$CONFIG_DIR/config.toml"; fi
    fi
    if [ "$CATALOG_CHANGED" -eq 1 ]; then
      if [ -f "$BACKUP/xingqiao-models.json" ]; then cp -p "$BACKUP/xingqiao-models.json" "$CONFIG_DIR/.xingqiao/models.json" || printf '恢复失败，备份：%s\n' "$BACKUP" >&2
      else rm -f "$CONFIG_DIR/.xingqiao/models.json"; fi
    fi
    printf '\n配置未完成，请查看上方具体错误。已有备份：%s\n' "${BACKUP:-未写入配置}" >&2
  fi
  [ -z "$STAGE" ] || rm -rf "$STAGE"
  [ -z "$LOCK" ] || rmdir "$LOCK" 2>/dev/null || :
  if [ -t 0 ]; then printf '\n按回车键关闭窗口。'; read -r _ || :; fi
  exit "$status"
}
trap finish EXIT
fail() { printf '%s\n' "$*" >&2; exit 1; }

printf '\n星桥AI · Codex 配置工具\n\n操作前请完全退出 Codex。\n1. 连接星桥AI\n2. 切回官方 Codex\n0. 退出\n'
while :; do
  printf '\n请选择：'
  IFS= read -r choice || exit 0
  case "$choice" in 1|2) break ;; 0) exit 0 ;; *) printf '请输入 1、2 或 0。\n' ;; esac
done

# Only owner-controlled targets are repaired. Never recurse over a user's home,
# change login credentials, or change operating-system execution protections.
check_target() {
  [ ! -L "$1" ] || fail "无法配置：路径为符号链接，请先确认实际目录：$1"
  if [ -e "$1" ]; then
    [ -O "$1" ] || fail "无法自动修复非本人所有的路径：$1。请让管理员恢复此路径的所有权后重试。"
    if [ "$2" = dir ]; then
      [ -d "$1" ] || fail "目录路径已被文件占用：$1"
      chmod u+rwx "$1" || fail "无法修复目录权限：$1"
    else
      [ -f "$1" ] || fail "文件路径已被文件夹或其他对象占用：$1"
      chmod u+rw "$1" || fail "无法修复文件权限：$1"
    fi
  fi
}
check_target "$CONFIG_DIR" dir
mkdir -p "$CONFIG_DIR" || fail "无法创建配置目录：$CONFIG_DIR"
CONFIG_DIR="$(cd "$CONFIG_DIR" && pwd -P)"
for name in .xingqiao xingqiao-backups; do check_target "$CONFIG_DIR/$name" dir; done
for name in config.toml .xingqiao/official.toml; do check_target "$CONFIG_DIR/$name" file; done
if [ "$choice" = 1 ] && [ -n "$CATALOG_PAYLOAD" ]; then check_target "$CONFIG_DIR/.xingqiao/models.json" file; fi
mkdir -p "$CONFIG_DIR/.xingqiao" "$CONFIG_DIR/xingqiao-backups"
chmod 700 "$CONFIG_DIR/.xingqiao" "$CONFIG_DIR/xingqiao-backups"
[ ! -L "$CONFIG_DIR/.xingqiao-lock" ] || fail '配置锁路径异常。'
mkdir "$CONFIG_DIR/.xingqiao-lock" 2>/dev/null || fail '另一个配置工具正在运行；请先关闭它。若曾被强制退出，请确认后移除 .xingqiao-lock 目录再重试。'
LOCK="$CONFIG_DIR/.xingqiao-lock"
STAGE="$(mktemp -d "$CONFIG_DIR/.xingqiao-tmp.XXXXXX")"

if printf '' | base64 --decode >/dev/null 2>&1; then DECODE='--decode'
elif printf '' | base64 -D >/dev/null 2>&1; then DECODE='-D'
else fail '系统 base64 解码组件不可用。'; fi
decode() { printf '%s' "$1" | base64 "$DECODE"; }

# Track TOML multiline strings so apparent keys/tables inside instructions are
# preserved. Selectors may be bare or quoted. Refuse multiline managed values.
transform() {
  awk -v mode="$1" -v bom="$(printf '\357\273\277')" '
  function scan(s, i,c,t,q,esc) {
    q=""; esc=0
    for(i=1;i<=length(s);i++) {
      c=substr(s,i,1); t=substr(s,i,3)
      if(multi!="") {
        if(multi=="\"\"\"" && c=="\\") {i++; continue}
        if(t==multi) {multi=""; i+=2}
      } else if(q!="") {
        if(q=="\"" && c=="\\") {i++; continue}
        if(c==q) q=""
      } else {
        if(c=="#") break
        if(t=="\"\"\"" || t==sprintf("%c%c%c",39,39,39)) {multi=t; i+=2}
        else if(c=="\"" || c==sprintf("%c",39)) q=c
      }
    }
  }
  BEGIN {root=1; managed=0; multi=""; table=""; state="official"}
  {
    raw=$0; if(NR==1 && index(raw,bom)==1) raw=substr(raw,length(bom)+1); sub(/\r$/,"",raw); line=raw; sub(/^[ \t]+/,"",line); free=(multi=="")
    if(free && line ~ /^\[/) {
      root=0
      parent=(line ~ /^\[[ \t]*(model_providers|"model_providers"|\047model_providers\047)[ \t]*\]/)
      managed=(line ~ /^\[[ \t]*(model_providers|"model_providers"|\047model_providers\047)[ \t]*\.[ \t]*(xingqiao|"xingqiao"|\047xingqiao\047)[ \t]*(\]|\.)/)
    }
    key=line; sub(/[ \t]*=.*/,"",key); gsub(/[\047\"]/,"",key); sub(/[ \t]+$/,"",key)
    if(free && line ~ /=/ && ((root && (key=="model_providers" || key ~ /^model_providers\.xingqiao(\.|$)/)) || (parent && key=="xingqiao"))) exit 42
    if(free && root && key=="model_provider") {
      value=line; sub(/^[^=]*=[ \t]*/,"",value); sub(/[ \t]*(#.*)?$/,"",value)
      if(value !~ /^[\047"][A-Za-z0-9_-]+[\047"]$/) exit 42
      gsub(/[\047"]/ ,"",value)
      state=(value=="openai" ? "official" : (value=="xingqiao" ? "xingqiao" : "custom"))
    }
    if(free && root && key=="openai_base_url") custom_url=1
    selected=free && root && line ~ /=/ && (key=="model_provider" || key=="model" || key=="review_model" || key=="model_catalog_json" || key=="openai_base_url")
    scan(raw)
    if(selected && multi!="") exit 42
    if(free && raw=="# Xingqiao managed provider") next
    if(mode=="roots" && selected) print raw
    if(mode=="body" && !selected && !managed) print raw
    if(mode=="provider" && managed) print raw
  }
  END {if(multi!="") exit 42; if(mode=="state") print (custom_url ? "custom" : state)}
  ' "$2" || fail '配置格式无法安全处理（连接参数或提供商定义格式不受支持），原配置已保留。'
}

CURRENT="$STAGE/current.toml"
if [ -f "$CONFIG_DIR/config.toml" ]; then cp "$CONFIG_DIR/config.toml" "$CURRENT"; else : > "$CURRENT"; fi
transform body "$CURRENT" > "$STAGE/body.toml"
transform roots "$CURRENT" > "$STAGE/current-roots.toml"
OFFICIAL="$CONFIG_DIR/.xingqiao/official.toml"
CURRENT_STATE="$(transform state "$CURRENT")"
SAVE_OFFICIAL=0
if [ "$CURRENT_STATE" = official ]; then
  # Refresh the snapshot when the user has changed official settings since a
  # previous switch. Repeated relay runs must never overwrite this snapshot.
  if [ -s "$CURRENT" ]; then cp "$CURRENT" "$STAGE/official.toml"
  else printf 'model_provider = "openai"\n' > "$STAGE/official.toml"; fi
  SAVE_OFFICIAL=1
elif [ -f "$OFFICIAL" ]; then cp "$OFFICIAL" "$STAGE/official.toml"
else
  printf 'model_provider = "openai"\n' > "$STAGE/official.toml"
  cat "$STAGE/body.toml" >> "$STAGE/official.toml"
  SAVE_OFFICIAL=1
fi

if [ "$choice" = 1 ]; then
  decode "$PAYLOAD" > "$STAGE/payload.toml" || fail '内嵌配置解码失败，原配置已保留。'
  transform roots "$STAGE/payload.toml" > "$STAGE/new.toml"
  if [ -n "$CATALOG_PAYLOAD" ]; then
    decode "$CATALOG_PAYLOAD" > "$STAGE/catalog.json" || fail '内嵌模型目录解码失败，原配置已保留。'
    # TOML basic strings require escaping path delimiters and control characters.
    catalog_path="$(printf '%s' "$CONFIG_DIR/.xingqiao/models.json" | sed 's/\\/\\\\/g; s/"/\\"/g')"
    case "$catalog_path" in *$'\n'*|*$'\r'*|*$'\t'*) fail '配置目录包含不支持的控制字符。' ;; esac
    printf 'model_catalog_json = "%s"\n' "$catalog_path" >> "$STAGE/new.toml"
  fi
  cat "$STAGE/body.toml" >> "$STAGE/new.toml"
  printf '# Xingqiao managed provider\n' >> "$STAGE/new.toml"
  transform provider "$STAGE/payload.toml" >> "$STAGE/new.toml"
else
  transform body "$STAGE/official.toml" > "$STAGE/official-body.toml"
  # Preserve exact original formatting when other settings have not changed.
  if cmp -s "$STAGE/body.toml" "$STAGE/official-body.toml"; then cp "$STAGE/official.toml" "$STAGE/new.toml"
  else
    transform roots "$STAGE/official.toml" > "$STAGE/new.toml"
    cat "$STAGE/body.toml" >> "$STAGE/new.toml"
  fi
fi

BACKUP="$(mktemp -d "$CONFIG_DIR/xingqiao-backups/$(date +%Y%m%d-%H%M%S).XXXXXX")"
for name in config.toml; do
  if [ -f "$CONFIG_DIR/$name" ]; then cp -p "$CONFIG_DIR/$name" "$BACKUP/$name"; chmod 600 "$BACKUP/$name"; fi
done
if [ "$choice" = 1 ] && [ -n "$CATALOG_PAYLOAD" ] && [ -f "$CONFIG_DIR/.xingqiao/models.json" ]; then
  cp -p "$CONFIG_DIR/.xingqiao/models.json" "$BACKUP/xingqiao-models.json"; chmod 600 "$BACKUP/xingqiao-models.json"
fi
if [ "$SAVE_OFFICIAL" = 1 ]; then mv -f "$STAGE/official.toml" "$OFFICIAL"; fi
if [ "$choice" = 1 ] && [ -n "$CATALOG_PAYLOAD" ]; then
  CATALOG_CHANGED=1
  mv -f "$STAGE/catalog.json" "$CONFIG_DIR/.xingqiao/models.json"
fi
CONFIG_CHANGED=1
mv -f "$STAGE/new.toml" "$CONFIG_DIR/config.toml"
chmod 600 "$CONFIG_DIR/config.toml"
CONFIG_CHANGED=0
CATALOG_CHANGED=0
printf '\n配置已保存，请完全退出并重新启动 Codex。\n配置文件：%s\n切换前备份：%s\n' "$CONFIG_DIR/config.toml" "$BACKUP"
if [ "$choice" = 2 ]; then printf '已切回官方；如有需要，请在 Codex 中完成官方登录。\n'; fi
