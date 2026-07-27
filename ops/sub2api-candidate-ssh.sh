#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'candidate_ssh status=failed\n' >&2
  exit 1
}

original=${SSH_ORIGINAL_COMMAND:-}
[[ -n "$original" && "$original" != *$'\n'* && "$original" != *$'\r'* ]] || fail
[[ "$original" =~ ^[a-zA-Z0-9_./:@+-]+([[:space:]][a-zA-Z0-9_./:@+-]+){4}$ ]] || fail

IFS=' ' read -r -a parts <<<"$original"
[[ ${#parts[@]} -eq 5 && "${parts[0]}" == prepare ]] || fail
reference=${parts[1]}
version=${parts[2]}
official_commit=${parts[3]}
source_commit=${parts[4]}

[[ "$reference" =~ ^ghcr\.io/leesssong/xingqiao-sub2api@sha256:[a-f0-9]{64}$ ]] || fail
[[ "$version" =~ ^[0-9]+(\.[0-9]+){1,2}$ ]] || fail
[[ "$official_commit" =~ ^[a-f0-9]{40}$ && "$source_commit" =~ ^[a-f0-9]{40}$ ]] || fail

sudo_bin=/usr/bin/sudo
if [[ "$(uname -s)" != Linux && ${SUB2API_CANDIDATE_TEST_MODE:-} == 1 &&
      -n ${SUB2API_CANDIDATE_TEST_LOG:-} && ${SUB2API_CANDIDATE_SUDO:-} = /* ]]; then
  sudo_bin=$SUB2API_CANDIDATE_SUDO
fi
[[ -x "$sudo_bin" ]] || fail

export PATH=/usr/sbin:/usr/bin:/sbin:/bin
exec "$sudo_bin" /usr/local/libexec/sub2api-candidate-loader \
  prepare "$reference" "$version" "$official_commit" "$source_commit"
