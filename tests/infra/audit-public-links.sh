#!/usr/bin/env bash
set -euo pipefail

BASE_URL=${BASE_URL:-https://api.xingqiaolab.top}

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

url_origin() {
  local url=$1
  if [[ "$url" =~ ^https?://[^/?#]+ ]]; then
    printf '%s\n' "${BASH_REMATCH[0]}"
    return 0
  fi
  return 1
}

BASE_ORIGIN=$(url_origin "$BASE_URL") || fail "BASE_URL must be an absolute HTTP(S) origin: $BASE_URL"
[[ "$BASE_URL" == "$BASE_ORIGIN" || "$BASE_URL" == "$BASE_ORIGIN/" ]] || \
  fail "BASE_URL must not include a path, query, or fragment: $BASE_URL"

TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

paths=(
  /
  /docs
  /docs/
  /docs/assets/01-create-key.png
  /docs/assets/02-select-group.png
  /docs/assets/03-key-actions.png
  /docs/assets/04-ccswitch.png
  /docs/assets/05-usage-and-billing.png
  /docs/does-not-exist
  /support
  /login
  /keys
  /usage
  /custom/xingqiao-storefront
)

reject_relay_hosts() {
  local response_file=$1
  if grep -Eqi 'api3\.xmhbao\.cn|43-133-75-82\.sslip\.io|([[:alnum:]-]+\.)?(xmhbao\.cn|sslip\.io)' "$response_file"; then
    fail "relay hostname found in response for $CURRENT_PATH"
  fi
}

fetch() {
  local path=$1
  local response_file=$2
  local result

  if ! result=$(curl --silent --show-error --location --max-redirs 5 \
    --output "$response_file" \
    --write-out '%{http_code}\t%{num_redirects}\t%{url_effective}' \
    "${BASE_ORIGIN}${path}"); then
    fail "request failed for $path"
  fi

  IFS=$'\t' read -r REQUEST_STATUS REQUEST_REDIRECTS REQUEST_FINAL_URL <<< "$result"
  [[ "$REQUEST_STATUS" =~ ^[0-9]{3}$ ]] || fail "missing HTTP status for $path"
  [[ "$REQUEST_REDIRECTS" =~ ^[0-9]+$ ]] || fail "missing redirect count for $path"
  REQUEST_FINAL_ORIGIN=$(url_origin "$REQUEST_FINAL_URL") || fail "final URL is not absolute for $path"
}

require_same_origin() {
  [[ "$REQUEST_FINAL_ORIGIN" == "$BASE_ORIGIN" ]] || \
    fail "final URL leaves the public origin for $CURRENT_PATH: $REQUEST_FINAL_URL"
}

print_result() {
  printf '%-34s %-7s %-10s %-55s %s\n' \
    "$CURRENT_PATH" "$REQUEST_STATUS" "$REQUEST_REDIRECTS" "$REQUEST_FINAL_URL" "$1"
}

audit_path() {
  local path=$1
  local classification=$2
  local response_file="$TEMP_DIR/$((${#path} + ${RANDOM})).body"
  CURRENT_PATH=$path

  fetch "$path" "$response_file"
  reject_relay_hosts "$response_file"
  require_same_origin

  case "$path" in
    /docs)
      [[ "$REQUEST_STATUS" == '200' ]] || fail "/docs must finish with 200, got $REQUEST_STATUS"
      [[ "$REQUEST_REDIRECTS" == '1' ]] || fail "/docs must redirect exactly once, got $REQUEST_REDIRECTS"
      [[ "$REQUEST_FINAL_URL" == "$BASE_ORIGIN/docs/" ]] || \
        fail "/docs must finish at $BASE_ORIGIN/docs/, got $REQUEST_FINAL_URL"
      ;;
    /docs/|/docs/assets/*)
      [[ "$REQUEST_STATUS" == '200' ]] || fail "$path must return 200, got $REQUEST_STATUS"
      ;;
    /docs/does-not-exist)
      [[ "$REQUEST_STATUS" == '404' ]] || fail "$path must return 404, got $REQUEST_STATUS"
      ;;
    *)
      (( REQUEST_STATUS < 500 )) || fail "$path must not return a 5xx status, got $REQUEST_STATUS"
      ;;
  esac

  print_result "$classification"
}

printf '%-34s %-7s %-10s %-55s %s\n' 'source path' 'status' 'redirects' 'final URL' 'classification'

for path in "${paths[@]}"; do
  case "$path" in
    /docs) classification='docs redirect' ;;
    /docs/) classification='docs index' ;;
    /docs/assets/*) classification='docs asset' ;;
    /docs/does-not-exist) classification='docs missing' ;;
    /custom/xingqiao-storefront) classification='storefront' ;;
    *) classification='application page' ;;
  esac
  audit_path "$path" "$classification"
done

CURRENT_PATH=/api/v1/settings/public
settings_file="$TEMP_DIR/settings-public.json"
fetch "$CURRENT_PATH" "$settings_file"
reject_relay_hosts "$settings_file"
require_same_origin
[[ "$REQUEST_STATUS" == '200' ]] || fail "$CURRENT_PATH must return 200, got $REQUEST_STATUS"
jq -e '
  (.data // .) |
  .doc_url == "https://api.xingqiaolab.top/docs/" and
  .balance_low_notify_recharge_url == "https://api.xingqiaolab.top/custom/xingqiao-storefront" and
  (.custom_menu_items | any(.id == "xingqiao-storefront"))
' "$settings_file" >/dev/null || fail 'public settings do not expose the required public links'
print_result 'public settings'

printf 'PASS: public link audit\n'
