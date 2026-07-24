#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
compose_file="$repo_root/infra/compose.yaml"

docker compose \
  -f "$compose_file" \
  config \
  --format json \
  --no-interpolate \
  --no-normalize \
  --no-path-resolution \
| ruby -rjson -e '
  compose = JSON.parse(STDIN.read)
  service = compose.fetch("services").fetch("sub2api")
  build = service.fetch("build")

  expected = {
    "context" => "../upstream/sub2api",
    "dockerfile" => "Dockerfile",
  }
  expected.each do |key, value|
    actual = build[key]
    abort "sub2api build #{key}: expected #{value.inspect}, got #{actual.inspect}" unless actual == value
  end

  expected_args = {
    "VERSION" => "v0.1.164-xingqiao-contact-v1",
    "COMMIT" => "cd8bb98c44303b2c8f04c0da340447c992f0cb7d",
  }
  actual_args = build.fetch("args")
  expected_args.each do |key, value|
    actual = actual_args[key]
    abort "sub2api build arg #{key}: expected #{value.inspect}, got #{actual.inspect}" unless actual == value
  end

  expected_image = "xingqiao-sub2api:v0.1.164-contact-v1"
  abort "unexpected sub2api image #{service["image"].inspect}" unless service["image"] == expected_image

  volumes = service.fetch("volumes")
  data_mount_present = volumes.any? do |volume|
    volume["type"] == "volume" &&
      volume["source"] == "sub2api_data" &&
      volume["target"] == "/app/data"
  end
  abort "sub2api_data mount was removed" unless data_mount_present
  abort "sub2api_data named volume was removed" unless compose.fetch("volumes").key?("sub2api_data")
'

printf 'custom Sub2API build contract is valid\n'
