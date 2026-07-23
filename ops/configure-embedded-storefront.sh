#!/usr/bin/env bash
set -euo pipefail

: "${ADMIN_API_URL:?Set ADMIN_API_URL, for example https://api.xingqiaolab.top}"
: "${ADMIN_API_KEY_FILE:?Set ADMIN_API_KEY_FILE to the protected Sub2API admin API key file}"

STORE_URL=${STORE_URL:-https://catfk.com/shop/DLK8SNUJ}
MENU_ID=${MENU_ID:-xingqiao-storefront}
MENU_LABEL=${MENU_LABEL:-充值/订阅}
MENU_SORT_ORDER=${MENU_SORT_ORDER:-90}
PAYMENT_ENABLED=${PAYMENT_ENABLED:-false}

[[ -r "$ADMIN_API_KEY_FILE" ]] || { printf 'Admin API key file is not readable: %s\n' "$ADMIN_API_KEY_FILE" >&2; exit 1; }
command -v curl >/dev/null || { printf 'curl is required\n' >&2; exit 1; }
command -v jq >/dev/null || { printf 'jq is required\n' >&2; exit 1; }
[[ "$PAYMENT_ENABLED" == "true" || "$PAYMENT_ENABLED" == "false" ]] || {
  printf 'PAYMENT_ENABLED must be true or false\n' >&2
  exit 1
}

admin_key=$(tr -d '\r\n' < "$ADMIN_API_KEY_FILE")
[[ -n "$admin_key" ]] || { printf 'Admin API key file is empty\n' >&2; exit 1; }

settings=$(curl --fail-with-body --silent --show-error \
  -H "X-API-Key: $admin_key" \
  "$ADMIN_API_URL/api/v1/admin/settings")

payload=$(jq --arg id "$MENU_ID" \
  --arg label "$MENU_LABEL" \
  --arg url "$STORE_URL" \
  --argjson paymentEnabled "$PAYMENT_ENABLED" \
  --argjson sortOrder "$MENU_SORT_ORDER" '
  (.data // .) as $settings
  | $settings
  | .payment_enabled = $paymentEnabled
  | .custom_menu_items = (
      ((.custom_menu_items // []) | map(select(.id != $id)))
      + [{
          id: $id,
          label: $label,
          icon_svg: "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 24 24\" fill=\"none\" stroke=\"currentColor\" stroke-width=\"2\"><rect x=\"3\" y=\"5\" width=\"18\" height=\"14\" rx=\"2\"/><path d=\"M3 10h18\"/><path d=\"M7 15h3\"/></svg>",
          url: $url,
          visibility: "user",
          sort_order: $sortOrder
        }]
    )
' <<< "$settings")

curl --fail-with-body --silent --show-error \
  -X PUT \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $admin_key" \
  --data-binary "$payload" \
  "$ADMIN_API_URL/api/v1/admin/settings" \
  | jq -e 'if has("data") then .data else . end | .custom_menu_items | any(.id == "xingqiao-storefront")' >/dev/null

printf 'Configured embedded storefront menu: %s\n' "$MENU_ID"
