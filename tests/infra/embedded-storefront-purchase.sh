#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"

fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }
require() { rg -Fq -- "$1" "$2" || fail "missing $1 in $2"; }
reject() { ! rg -Fq -- "$1" "$2" || fail "forbidden $1 in $2"; }

CADDY=infra/Caddyfile
reject '@embedded_storefront_purchase' "$CADDY"
reject '/purchase.html' "$CADDY"
require '{$STOREFRONT_SITE_ADDRESS:https://shop.xingqiaolab.top} {' "$CADDY"
require 'header_down Content-Security-Policy "frame-src " "frame-src https://shop.xingqiaolab.top https://catfk.com "' "$CADDY"
require '@catfk_storefront_entry {' "$CADDY"
require 'path / /shop/DLK8SNUJ /shop/DLK8SNUJ/' "$CADDY"
require 'route {' "$CADDY"
require 'redir https://catfk.com/shop/DLK8SNUJ?{http.request.uri.query} 302' "$CADDY"
require 'frame-ancestors https://api.xingqiaolab.top' "$CADDY"
require 'Cache-Control "no-store, max-age=0"' "$CADDY"
reject 'reverse_proxy https://catfk.com' "$CADDY"
reject '/shopApi/' "$CADDY"
require 'respond 404' "$CADDY"
require 'reverse_proxy sub2api:8080' "$CADDY"
[[ $(rg -c '^\tlog \{$' "$CADDY") -eq 1 ]] || fail "storefront access logging must remain disabled to avoid token leakage"

CONFIGURE=ops/configure-embedded-storefront.sh
require 'STORE_URL=${STORE_URL:-https://catfk.com/shop/DLK8SNUJ}' "$CONFIGURE"
require 'MENU_ID=${MENU_ID:-xingqiao-storefront}' "$CONFIGURE"
require 'MENU_LABEL=${MENU_LABEL:-充值/订阅}' "$CONFIGURE"
require 'PAYMENT_ENABLED=${PAYMENT_ENABLED:-false}' "$CONFIGURE"
require '.payment_enabled = $paymentEnabled' "$CONFIGURE"
require 'custom_menu_items' "$CONFIGURE"
require 'X-API-Key' "$CONFIGURE"
require 'jq --arg id "$MENU_ID"' "$CONFIGURE"
require 'https://catfk.com/shop/DLK8SNUJ' "$CONFIGURE"

printf 'PASS: embedded storefront native menu contract\n'
