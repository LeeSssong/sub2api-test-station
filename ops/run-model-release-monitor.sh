#!/bin/sh
set -eu

umask 077

fail() {
  printf '%s\n' "model_release_monitor status=failed"
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

root=${MODEL_RELEASE_ROOT:-}
admin_key_file=${MODEL_RELEASE_ADMIN_KEY_FILE:-}
evidence_dir=${MODEL_RELEASE_EVIDENCE_DIR:-}
runner_image=${MODEL_RELEASE_RUNNER_IMAGE:-}
docker_network=${MODEL_RELEASE_DOCKER_NETWORK:-}
docker_bin=${MODEL_RELEASE_DOCKER_BIN:-/usr/bin/docker}

absolute_directory "$root" || fail
absolute_file "$admin_key_file" || fail
absolute_directory "$evidence_dir" || fail
[ -x "$docker_bin" ] || fail

case "$runner_image" in
  sub2api-relay-ops:*) ;;
  *) fail ;;
esac

[ "$docker_network" = "sub2api_default" ] || fail

for source_file in \
  collect-model-release-snapshot.rb \
  evaluate-model-release-readiness.rb \
  model-release-policy.rb \
  model-release-policy-v1.yaml
do
  [ -f "$root/$source_file" ] || fail
done

printf '%s\n' "model_release_monitor status=started"
if "$docker_bin" run --rm --network "$docker_network" \
  --user 10002:10002 --read-only --cap-drop ALL \
  --security-opt no-new-privileges --pids-limit 64 --memory 128m --cpus 0.25 \
  --tmpfs /tmp:rw,nosuid,nodev,noexec,size=16m \
  -v "$root:/work:ro" \
  -v "$admin_key_file:/run/secrets/sub2api-admin-api-key:ro" \
  -v "$evidence_dir:/var/lib/model-release:rw" \
  --entrypoint /bin/sh "$runner_image" -ec '
    ruby /work/collect-model-release-snapshot.rb collect \
      --policy /work/model-release-policy-v1.yaml \
      --base-url http://sub2api:8080 \
      --admin-key-file /run/secrets/sub2api-admin-api-key \
      --output /var/lib/model-release/model-release-snapshot.json
    ruby /work/evaluate-model-release-readiness.rb evaluate \
      --policy /work/model-release-policy-v1.yaml \
      --snapshot /var/lib/model-release/model-release-snapshot.json \
      --output /var/lib/model-release/model-release-result.json
  '
then
  printf '%s\n' "model_release_monitor status=succeeded"
else
  fail
fi
