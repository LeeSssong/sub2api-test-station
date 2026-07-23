#!/bin/sh
set -eu

umask 077

fail() {
  printf '%s\n' "account_quality_monitor status=failed"
  exit 1
}

absolute_directory() {
  case "$1" in
    /*) [ -d "$1" ] ;;
    *) return 1 ;;
  esac
}

absolute_file() {
  case "$1" in
    /*) [ -f "$1" ] ;;
    *) return 1 ;;
  esac
}

root=${ACCOUNT_QUALITY_ROOT:-}
admin_key_file=${ACCOUNT_QUALITY_ADMIN_KEY_FILE:-}
evidence_dir=${ACCOUNT_QUALITY_EVIDENCE_DIR:-}
runner_image=${ACCOUNT_QUALITY_RUNNER_IMAGE:-}
docker_network=${ACCOUNT_QUALITY_DOCKER_NETWORK:-}
docker_bin=${ACCOUNT_QUALITY_DOCKER_BIN:-/usr/bin/docker}

absolute_directory "$root" || fail
absolute_file "$admin_key_file" || fail
absolute_directory "$evidence_dir" || fail
[ -x "$docker_bin" ] || fail
[ -f "$root/collect-account-quality-pulse.rb" ] || fail

case "$runner_image" in
  sub2api-relay-ops:*) ;;
  *) fail ;;
esac

[ "$docker_network" = "sub2api_default" ] || fail

printf '%s\n' "account_quality_monitor status=started"
if "$docker_bin" run --rm --network "$docker_network" \
  --user 10002:10002 --read-only --cap-drop ALL \
  --security-opt no-new-privileges --pids-limit 64 --memory 128m --cpus 0.25 \
  --tmpfs /tmp:rw,nosuid,nodev,noexec,size=16m \
  -v "$root:/work:ro" \
  -v "$admin_key_file:/run/secrets/sub2api-admin-api-key:ro" \
  -v "$evidence_dir:/var/lib/account-quality:rw" \
  --entrypoint /bin/sh "$runner_image" -ec '
    ruby /work/collect-account-quality-pulse.rb collect \
      --base-url http://sub2api:8080 \
      --admin-key-file /run/secrets/sub2api-admin-api-key \
      --output /var/lib/account-quality/account-quality-result.json
  ' >/dev/null 2>&1
then
  printf '%s\n' "account_quality_monitor status=succeeded"
else
  fail
fi
