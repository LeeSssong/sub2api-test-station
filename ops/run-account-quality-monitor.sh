#!/bin/sh
set -eu

umask 077

fail() {
  code=${1:-40}
  printf '%s\n' "account_quality_monitor status=failed"
  exit "$code"
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
release_env_file=${ACCOUNT_QUALITY_RELEASE_ENV_FILE:-/opt/sub2api/production/release.env}
evidence_dir=${ACCOUNT_QUALITY_EVIDENCE_DIR:-}
runner_image=${ACCOUNT_QUALITY_RUNNER_IMAGE:-}
docker_network=${ACCOUNT_QUALITY_DOCKER_NETWORK:-}
docker_bin=${ACCOUNT_QUALITY_DOCKER_BIN:-/usr/bin/docker}

absolute_directory "$root" || fail 40
absolute_file "$admin_key_file" || fail 42
absolute_file "$release_env_file" || fail 40
absolute_directory "$evidence_dir" || fail 40
[ -x "$docker_bin" ] || fail 43
[ -f "$root/collect-account-quality-pulse.rb" ] || fail 40
evidence_owner=$(stat -c '%u:%g %a' "$evidence_dir" 2>/dev/null || stat -f '%u:%g %Lp' "$evidence_dir" 2>/dev/null || printf '%s' unknown)
[ "$evidence_owner" = "${ACCOUNT_QUALITY_EXPECTED_EVIDENCE_OWNER:-10002:10002 700}" ] || fail 41

case "$runner_image" in
  sub2api-relay-ops:*) ;;
  *) fail 40 ;;
esac

[ "$docker_network" = "sub2api_default" ] || fail 40

active_upstream=$(awk -F= '
  $1 == "SUB2API_ACTIVE_UPSTREAM" { count += 1; value = $2 }
  END { if (count == 1) print value; else exit 1 }
' "$release_env_file") || fail 40
case "$active_upstream" in
  sub2api-blue:8080|sub2api-green:8080) ;;
  *) fail 40 ;;
esac

if ! "$docker_bin" run --rm --user 10002:10002 --read-only --cap-drop ALL \
  --security-opt no-new-privileges --pids-limit 64 --memory 128m --cpus 0.25 \
  --tmpfs /tmp:rw,nosuid,nodev,noexec,size=16m \
  -v "$evidence_dir:/var/lib/account-quality:rw" \
  --entrypoint /usr/local/bin/ruby "$runner_image" -e '
    require "tempfile"
    dir = "/var/lib/account-quality"
    tmp = Tempfile.new([".uid10002-preflight-", ".tmp"], dir)
    final = File.join(dir, ".uid10002-preflight-final")
    begin
      tmp.write("uid10002-preflight")
      tmp.flush
      tmp.fsync
      tmp.close
      File.rename(tmp.path, final)
      raise "readback" unless File.binread(final) == "uid10002-preflight"
      File.delete(final)
      File.open(dir, "r") { |directory| directory.fsync }
    ensure
      tmp.close! rescue nil
      File.delete(final) if File.exist?(final)
    end
  ' >/dev/null 2>&1
then
  fail 41
fi

printf '%s\n' "account_quality_monitor status=started"
if "$docker_bin" run --rm --network "$docker_network" \
  --user 10002:10002 --read-only --cap-drop ALL \
  --security-opt no-new-privileges --pids-limit 64 --memory 128m --cpus 0.25 \
  --tmpfs /tmp:rw,nosuid,nodev,noexec,size=16m \
  -v "$root:/work:ro" \
  -v "$admin_key_file:/run/secrets/sub2api-admin-api-key:ro" \
  -v "$evidence_dir:/var/lib/account-quality:rw" \
  -e "ACCOUNT_QUALITY_BASE_URL=http://$active_upstream" \
  --entrypoint /bin/sh "$runner_image" -ec '
    ruby /work/collect-account-quality-pulse.rb collect \
      --base-url "$ACCOUNT_QUALITY_BASE_URL" \
      --admin-key-file /run/secrets/sub2api-admin-api-key \
      --output /var/lib/account-quality/account-quality-result.json
  ' >/dev/null 2>&1
then
  printf '%s\n' "account_quality_monitor status=succeeded"
else
  status=$?
  case "$status" in
    46) fail 46 ;;
    124|137) fail 45 ;;
    125|126|127) fail 43 ;;
    *) fail 44 ;;
  esac
fi
