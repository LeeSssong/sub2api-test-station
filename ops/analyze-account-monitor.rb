#!/usr/bin/env ruby
# frozen_string_literal: true

require "json"
require "optparse"
require "time"

MINIMUM_SCORE_DELTA = 0.05
FORBIDDEN_KEY = /\A(?:api[_-]?key|access[_-]?token|auth[_-]?token|cookie|authorization|password|secret|credential|raw[_-]?response|response[_-]?body|base[_-]?url|request[_-]?header)\z/i

def fail_with(message)
  warn message
  exit 1
end

def forbidden_value?(value)
  case value
  when Hash
    value.any? { |key, child| key.to_s.match?(FORBIDDEN_KEY) || forbidden_value?(child) }
  when Array
    value.any? { |child| forbidden_value?(child) }
  else
    false
  end
end

def clamp(value)
  [[value, 0.0].max, 1.0].min
end

def max_utilization(account)
  windows = account.fetch("usage_windows", {})
  windows.values.map { |window| window.fetch("utilization", 0.0).to_f }.max.to_f.clamp(0.0, 1.0)
end

def score(account)
  stability = clamp(account.fetch("success_rate").to_f)
  ttft = account["ttft_p95_ms"]
  latency = account["latency_p95_ms"]
  performance = ttft && latency ? clamp(1.0 - ((ttft.to_f + latency.to_f) / 2.0 / 2000.0)) : 0.0
  multiplier = 1.0 / (1.0 + [account.fetch("multiplier").to_f, 0.0].max)
  headroom = (1.0 - max_utilization(account) + (1.0 - [account.fetch("request_count").to_f / 1000.0, 1.0].min)) / 2.0
  (0.40 * stability) + (0.25 * performance) + (0.20 * multiplier) + (0.15 * headroom)
end

def usable_latest_status?(account)
  status = account.fetch("latest_status", "").to_s.downcase.strip
  return false if ["", "failed", "error", "http_error", "timeout", "balance_exhausted", "account_test_error"].include?(status)

  account.fetch("error_code", "").to_s.empty?
end

def account_view(account)
  windows = account.fetch("usage_windows", {})
  usage = windows.keys.sort.map { |name| "#{name} #{format('%.1f%%', windows.fetch(name).fetch('utilization').to_f * 100)}" }.join("、")
  usage = "无" if usage.empty?
  {
    "account_id" => account.fetch("account_id"),
    "name" => account.fetch("name"),
    "model_id" => account.fetch("model_id", ""),
    "success_rate" => format("%.1f%%", account.fetch("success_rate").to_f * 100),
    "ttft" => account["ttft_p95_ms"] ? format("%.0fms", account["ttft_p95_ms"]) : "未知",
    "latency" => account["latency_p95_ms"] ? format("%.0fms", account["latency_p95_ms"]) : "未知",
    "multiplier" => "#{account.fetch("multiplier")}x",
    "usage_windows" => usage,
    "status" => account.fetch("stale", false) ? "证据已过期" : "正常"
  }
end

def analyze(document)
  evidence_state = document.fetch("stale", false) ? "stale" : "fresh"
  groups = Hash.new { |hash, key| hash[key] = [] }
  names = {}
  document.fetch("accounts").each do |account|
    account.fetch("group_ids").each_with_index do |group_id, index|
      groups[group_id] << account
      names[group_id] = account.fetch("group_names")[index] if account.fetch("group_names")[index]
    end
  end

  output = groups.keys.sort.map do |group_id|
    rows = groups.fetch(group_id).sort_by { |account| account.fetch("account_id") }
    current_rows = rows.select { |account| account.fetch("status") == "active" && account.fetch("schedulable") }
    group = {
      "group_id" => group_id,
      "group_name" => names[group_id].to_s,
      "current_account_id" => current_rows.empty? ? 0 : current_rows.first.fetch("account_id"),
      "decision" => "insufficient_evidence",
      "score_delta" => 0.0,
      "reasons" => [],
      "evidence_state" => evidence_state,
      "current" => current_rows.empty? ? {} : account_view(current_rows.first)
    }
    if current_rows.empty?
      group["evidence_state"] = "no_current_account"
      group["reasons"] = ["当前分组没有 active + schedulable 账号"]
      next group
    end
    current = current_rows.first
    if evidence_state != "fresh"
      group["reasons"] = ["监控证据已过期，暂不建议更换"]
      next group
    end

    candidate = nil
    candidate_state = nil
    current_rows.drop(1).each do |item|
      if item.fetch("status") != "active" || !item.fetch("schedulable")
        candidate_state = "candidate_inactive_or_unschedulable"
        break
      elsif item.fetch("stale", false)
        candidate_state = "stale"
        break
      elsif current.fetch("sample_count") < 3 || item.fetch("sample_count") < 3
        candidate_state = "insufficient_samples"
        break
      elsif current.fetch("model_id").to_s.empty? || item.fetch("model_id") != current.fetch("model_id")
        candidate_state = "incompatible_model"
        break
      elsif !usable_latest_status?(current) || !usable_latest_status?(item)
        candidate_state = "recent_failure"
        break
      end
    end
    if candidate_state
      group["evidence_state"] = candidate_state
      group["reasons"] = ["候选账号证据#{candidate_state == "stale" ? "已过期" : "不足"}，暂不建议"]
      next group
    end

    eligible = current_rows.drop(1).select do |item|
      item.fetch("status") == "active" && item.fetch("schedulable") && !item.fetch("stale", false) &&
        item.fetch("sample_count") >= 3 && item.fetch("model_id") == current.fetch("model_id") && usable_latest_status?(item)
    end
    if eligible.empty?
      group["reasons"] = ["没有满足证据条件的候选账号"]
      next group
    end
    candidate = eligible.max_by { |item| [score(item), -item.fetch("account_id")] }
    delta = score(candidate) - score(current)
    group["candidate_account_id"] = candidate.fetch("account_id")
    group["candidate"] = account_view(candidate)
    group["score_delta"] = delta.round(4)
    if delta >= MINIMUM_SCORE_DELTA
      group["decision"] = "candidate_better"
      group["reasons"] = ["稳定性更高：#{format('%.1f%%', candidate.fetch('success_rate') * 100)} vs #{format('%.1f%%', current.fetch('success_rate') * 100)}"]
      group["reasons"] << "TTFT 更低" if candidate["ttft_p95_ms"].to_f < current["ttft_p95_ms"].to_f
      group["reasons"] << "倍率更低" if candidate.fetch("multiplier") < current.fetch("multiplier")
    else
      group["decision"] = "current_ok"
      group["evidence_state"] = "margin_below_threshold"
      group["reasons"] = ["综合评分差值 #{format('%.4f', delta)}，低于 0.05 更换阈值"]
    end
    group
  end
  { "evidence_state" => evidence_state, "groups" => output }
end

def main
  command = ARGV.shift
  fail_with("usage: analyze --input INPUT --output OUTPUT") unless command == "analyze"
  options = {}
  OptionParser.new do |parser|
    parser.on("--input PATH") { |value| options[:input] = value }
    parser.on("--output PATH") { |value| options[:output] = value }
  end.parse!(ARGV)
  fail_with("input and output are required") unless options[:input] && options[:output]
  document = JSON.parse(File.read(options[:input]))
  fail_with("input contains forbidden keys") if forbidden_value?(document)
  fail_with("projection schema mismatch") unless document.fetch("schema_version") == 1 && document["accounts"].is_a?(Array)
  result = analyze(document)
  temporary = "#{options[:output]}.tmp.#{$$}"
  File.write(temporary, JSON.generate(result))
  File.rename(temporary, options[:output])
  result.fetch("groups").each do |group|
    puts "#{group.fetch('group_name')}：#{group.fetch('decision')}"
  end
rescue JSON::ParserError, KeyError, TypeError, Errno::ENOENT => e
  fail_with("invalid account monitor projection: #{e.message}")
end

main
