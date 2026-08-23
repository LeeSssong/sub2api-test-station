#!/usr/bin/env bash
set -euo pipefail

login_view=upstream/sub2api/frontend/src/views/auth/LoginView.vue
[[ -f "$login_view" ]] || { echo 'lab login view is missing' >&2; exit 1; }

# The isolated lab has no public registration flow; its login footer must stay hidden.
grep -Fq 'v-if="!backendModeEnabled && !isAdminLab" #footer' "$login_view" \
  || { echo 'lab login still exposes the registration footer' >&2; exit 1; }

echo 'admin lab frontend auth surface: passed'
