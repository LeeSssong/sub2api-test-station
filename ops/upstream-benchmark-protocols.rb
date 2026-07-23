# frozen_string_literal: true

module UpstreamBenchmark
  module Protocols
    class BaseAdapter
      ERROR_CATEGORIES = [
        [/rate.?limit|too.?many.?requests/, "rate_limited"],
        [/insufficient.?balance|insufficient.?quota|billing|payment.?required/, "insufficient_balance"],
        [/authentication|unauthorized|invalid.?api.?key|permission|forbidden/, "authentication"],
        [/model.*(?:not.?found|unavailable)|unsupported.?model/, "model_unavailable"],
        [/invalid.?request|bad.?request|validation|unprocessable/, "request_rejected"],
        [/internal|server|upstream|service.?unavailable/, "upstream_http"]
      ].freeze

      def initialize(terminal_events)
        @terminal_events = Array(terminal_events).freeze
      end

      def models_request(path:)
        { method: :get, path: path, payload: nil }
      end

      def parse_models(document)
        Array(document["data"]).map do |item|
          item["id"] if item.is_a?(Hash) && item["id"].is_a?(String)
        end.compact
      end

      def classify_error(document)
        error = document.is_a?(Hash) ? document["error"] : nil
        value = if error.is_a?(Hash)
                  error["type"] || error["code"] || "protocol_error"
                else
                  "protocol_error"
                end
        normalized = Redactor.clean(value.to_s).downcase
        match = ERROR_CATEGORIES.find { |pattern, _category| normalized.match?(pattern) }
        match ? match.last : "protocol_error"
      end

      private

      def normalized_usage(input_tokens, output_tokens, total_tokens)
        input = input_tokens.to_i
        output = output_tokens.to_i
        total = total_tokens.nil? ? input + output : total_tokens.to_i
        {
          "input_tokens" => input,
          "output_tokens" => output,
          "prompt_tokens" => input,
          "completion_tokens" => output,
          "total_tokens" => total
        }
      end
    end

    class ChatCompletionsAdapter < BaseAdapter
      def generate_request(model:, prompt:, max_output_tokens:, stream:)
        {
          "model" => model,
          "messages" => [{ "role" => "user", "content" => prompt }],
          "max_tokens" => max_output_tokens,
          "stream" => stream
        }
      end

      def normalize_usage(raw)
        source = raw.is_a?(Hash) ? raw : {}
        normalized_usage(
          source["prompt_tokens"],
          source["completion_tokens"],
          source["total_tokens"]
        )
      end

      def terminal_event?(event)
        @terminal_events.include?(event)
      end
    end

    class ResponsesAdapter < BaseAdapter
      def generate_request(model:, prompt:, max_output_tokens:, stream:)
        {
          "model" => model,
          "input" => prompt,
          "max_output_tokens" => max_output_tokens,
          "stream" => stream
        }
      end

      def normalize_usage(raw)
        source = raw.is_a?(Hash) ? raw : {}
        normalized_usage(
          source["input_tokens"],
          source["output_tokens"],
          source["total_tokens"]
        )
      end

      def terminal_event?(event)
        type = event.is_a?(Hash) ? event["type"] : event
        @terminal_events.include?(type)
      end
    end

    REGISTRY = {
      "chat_completions" => ChatCompletionsAdapter,
      "responses" => ResponsesAdapter
    }.freeze

    module_function

    def build(name, terminal_events:)
      klass = REGISTRY[name]
      raise ValidationError, "unsupported benchmark protocol: #{name}" unless klass

      klass.new(terminal_events)
    end
  end
end
