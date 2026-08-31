#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'native_concurrency_guard status=failed: %s\n' "$1" >&2
  exit 1
}

worktree=
while (($#)); do
  case "$1" in
    --worktree)
      (($# >= 2)) || fail '--worktree requires a value'
      worktree=$2
      shift 2
      ;;
    *) fail "unknown argument: $1" ;;
  esac
done

[[ "$worktree" == /* && -d "$worktree" && ! -L "$worktree" ]] \
  || fail '--worktree must be an absolute non-symlink directory'
worktree=$(cd "$worktree" && pwd -P)

backend="$worktree/upstream/sub2api/backend"
service="$backend/internal/service/openai_shared_health.go"
handler_dir="$backend/internal/handler"
[[ -f "$backend/go.mod" ]] || fail 'Sub2API backend go.mod is missing'
[[ -f "$service" && ! -L "$service" ]] || fail 'OpenAI shared health service source is missing'
[[ -d "$handler_dir" && ! -L "$handler_dir" ]] || fail 'OpenAI handler directory is missing'

# Keep the source-level contract strict so a stale candidate cannot silently
# restore the custom Redis admission gate while preserving the old method names.
ruby - "$service" "$handler_dir" "$backend" <<'RUBY'
service_path, handler_dir, backend = ARGV
source = File.binread(service_path).force_encoding(Encoding::UTF_8)
abort "native_concurrency_guard status=failed: service source is not valid UTF-8" unless source.valid_encoding?

normalize = ->(value) { value.gsub("\t", "  ").gsub(/\r\n?/, "\n") }
acquire_signature = 'func (s *OpenAIGatewayService) AcquireOpenAIAdmission(_ int64, _ OpenAIAdmissionRequestShape) (func(), OpenAISharedAdmissionDecision) {'
acquire_expected = <<~GO.chomp
  #{acquire_signature}
    return func() {}, OpenAISharedAdmissionDecision{Allowed: true, Reason: "disabled"}
  }
GO
acquire_blocks = source.scan(/^#{Regexp.escape(acquire_signature)}.*?^\}/m)
abort "native_concurrency_guard status=failed: AcquireOpenAIAdmission must be a single permanent no-op" unless acquire_blocks.length == 1 && normalize.call(acquire_blocks.first) == normalize.call(acquire_expected)

record_signature = 'func (s *OpenAIGatewayService) RecordOpenAISlowSessionGuard(_ int64, _ *OpenAIForwardResult, _ bool) {'
record_expected = "#{record_signature}\n}\n"
record_blocks = source.scan(/^#{Regexp.escape(record_signature)}.*?^\}/m)
abort "native_concurrency_guard status=failed: RecordOpenAISlowSessionGuard must be empty" unless record_blocks.length == 1 && normalize.call(record_blocks.first + "\n") == normalize.call(record_expected)

handlers = Dir[File.join(handler_dir, '*.go')].reject { |path| path.end_with?('_test.go') }
abort 'native_concurrency_guard status=failed: no runtime handlers found' if handlers.empty?
forbidden = [
  '.AcquireOpenAIAdmission(',
  '.RecordOpenAISlowSessionGuard(',
  '.WithOpenAIAdmissionRequestShape(',
  '.WithOpenAIFirstSemanticOutputCallback(',
  '.ClassifyOpenAIAdmissionRequestShape(',
  'openai.admission_rejected'
]
handlers.each do |path|
  body = File.binread(path).force_encoding(Encoding::UTF_8)
  abort "native_concurrency_guard status=failed: handler source is not valid UTF-8: #{path}" unless body.valid_encoding?
  forbidden.each do |token|
    abort "native_concurrency_guard status=failed: custom admission call #{token} found in #{path}" if body.include?(token)
  end
end

# Scan every production Go source so a task cannot bypass the named handler
# checks by moving or renaming the custom admission call.
runtime_sources = Dir[File.join(backend, 'internal', '**', '*.go')].reject { |path| path.end_with?('_test.go') }
custom_admission_call = /\.(?:Acquire|Renew|Release|Record|Has)[A-Za-z0-9_]*(?:Admission|SlowSessionGuard)[A-Za-z0-9_]*\s*\(/
runtime_sources.each do |path|
  body = File.binread(path).force_encoding(Encoding::UTF_8)
  abort "native_concurrency_guard status=failed: runtime source is not valid UTF-8: #{path}" unless body.valid_encoding?
  match = body.match(custom_admission_call)
  abort "native_concurrency_guard status=failed: custom admission-like call #{match[0]} found in #{path}" if match
end

required_native_handlers = %w[openai_chat_completions.go openai_gateway_handler.go]
required_native_handlers.each do |name|
  path = File.join(handler_dir, name)
  body = File.binread(path).force_encoding(Encoding::UTF_8)
  abort "native_concurrency_guard status=failed: required native slot handler is missing: #{name}" unless body.include?('acquireResponsesAccountSlot(') && body.include?('accountReleaseFunc()')
end
RUBY

printf 'native_concurrency_guard status=passed mode=native_account_concurrency_only\n'
