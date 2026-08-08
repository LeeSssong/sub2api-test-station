# frozen_string_literal: true

require "yaml"

ROOT = File.expand_path("../..", __dir__)
INVENTORY_PATH = File.join(ROOT, "docs/contracts/sub2api-customization-inventory.yaml")
ADMIN_CONTRACT_PATH = File.join(ROOT, "docs/contracts/admin-experience-contract.md")

REQUIRED_CAPABILITY_FIELDS = %w[
  capability owner source_paths api_paths tables ui_routes source_of_truth acceptance_evidence
].freeze
ALLOWED_OWNERS = %w[core adapter control_plane host].freeze
REQUIRED_ADMIN_ROUTES = %w[
  /admin/accounts/monitor
  /admin/operations/account-profitability
  /admin/ops
  /admin/usage
  /admin/groups
  /admin/accounts
  /admin/channels/pricing
  /admin/settings
].freeze

abort "inventory file missing: #{INVENTORY_PATH}" unless File.file?(INVENTORY_PATH)
abort "admin contract missing: #{ADMIN_CONTRACT_PATH}" unless File.file?(ADMIN_CONTRACT_PATH)

inventory = YAML.load_file(INVENTORY_PATH)
abort "inventory must be a mapping" unless inventory.is_a?(Hash)

capabilities = inventory.fetch("capabilities")
abort "capabilities must be a non-empty array" unless capabilities.is_a?(Array) && !capabilities.empty?

capabilities.each_with_index do |record, index|
  abort "capability #{index} must be a mapping" unless record.is_a?(Hash)
  missing = REQUIRED_CAPABILITY_FIELDS.reject { |field| record.key?(field) }
  abort "capability #{index} missing: #{missing.join(', ')}" unless missing.empty?
  abort "capability #{index} has invalid owner" unless ALLOWED_OWNERS.include?(record["owner"])
  REQUIRED_CAPABILITY_FIELDS.drop(2).each do |field|
    value = record[field]
    abort "capability #{index} field #{field} must be an array" unless value.is_a?(Array)
  end
end

manifest = inventory.fetch("production_manifest")
%w[sub2api caddy postgresql redis worker].each do |service|
  entry = manifest.fetch(service)
  %w[image image_id].each do |field|
    abort "production manifest #{service}.#{field} is required" unless entry[field].is_a?(String) && !entry[field].empty?
  end
end
%w[migrations_hash active_slot release_state].each do |field|
  value = manifest[field]
  abort "production manifest #{field} is required" unless value.is_a?(String) && !value.empty?
end
evidence = manifest.fetch("acceptance_evidence")
abort "production manifest acceptance_evidence is required" unless evidence.is_a?(Array) && !evidence.empty?

admin_contract = File.read(ADMIN_CONTRACT_PATH)
missing_routes = REQUIRED_ADMIN_ROUTES.reject { |route| admin_contract.include?(route) }
abort "admin contract missing routes: #{missing_routes.join(', ')}" unless missing_routes.empty?
%w[同域登录 2FA 原 URL 字段 筛选 排序 分页 刷新 CSV 降级].each do |term|
  abort "admin contract missing requirement: #{term}" unless admin_contract.include?(term)
end

puts "PASS capabilities=#{capabilities.length} required_admin_routes=#{REQUIRED_ADMIN_ROUTES.length}"
