#!/usr/bin/env ruby
# frozen_string_literal: true

require "json"
require "socket"
require "tempfile"
require "timeout"

IMAGE = "caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d"
ROOT = File.expand_path("../..", __dir__)
PRODUCTION_CONFIG = File.join(ROOT, "infra", "Caddyfile")

def fail_test(message)
  warn "FAIL: #{message}"
  exit 1
end

def free_port
  server = TCPServer.new("127.0.0.1", 0)
  port = server.addr.fetch(1)
  server.close
  port
end

def response_for_request(client)
  header = +""
  header << client.readpartial(4096) until header.include?("\r\n\r\n")
  content_length = header[/\r\nContent-Length:\s*(\d+)/i, 1].to_i
  body = header.split("\r\n\r\n", 2).fetch(1).dup
  body << client.readpartial([content_length - body.bytesize, 4096].min) while body.bytesize < content_length
  payload = { "ok" => true, "bytes" => body.bytesize }.to_json
  client.write("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: #{payload.bytesize}\r\nConnection: close\r\n\r\n#{payload}")
rescue EOFError, Errno::ECONNRESET, IOError
  # The incomplete-upload case is expected to close before the body ends.
ensure
  client.close rescue nil
end

def start_upstream(port)
  server = TCPServer.new("127.0.0.1", port)
  stopping = false
  thread = Thread.new do
    until stopping
      begin
        client = server.accept_nonblock
      rescue IO::WaitReadable
        begin
          IO.select([server], nil, nil, 0.1)
        rescue IOError, Errno::EBADF
          break
        end
        next
      rescue IOError, Errno::EBADF
        break
      end
      Thread.new(client) { |socket| response_for_request(socket) }
    end
  end
  [server, thread, -> { stopping = true; server.close rescue nil; thread.join(2) }]
end

def read_caddy_config(read_body_timeout: nil)
  source = File.read(PRODUCTION_CONFIG)
  # Import production policy and make only local-harness substitutions: no
  # ACME, a local HTTP listener, and an injected upstream. The timeout and
  # fallback proxy blocks remain production text.
  source.sub!("{\n\tservers {", "{\n\tauto_https off\n\tservers {") ||
    fail_test("test harness could not disable automatic TLS in production Caddyfile")
  source.sub!(/\n\ttls \{\n(?:\t\t.*\n)+?\t\}\n/, "\n") ||
    fail_test("test harness could not remove the production certificate stanza")
  return source unless read_body_timeout

  replacement_count = source.scan(/read_body\s+15m\b/).length
  fail_test("test override expected exactly one production read_body 15m policy, found #{replacement_count}") unless replacement_count == 1
  source.gsub!(/(read_body\s+)15m\b/, "\\1#{read_body_timeout}")
  source
end

def start_caddy(caddy_port, upstream_port, read_body_timeout: nil)
  config = Tempfile.new(["t01-caddy", ".Caddyfile"], ROOT)
  config.write(read_caddy_config(read_body_timeout: read_body_timeout))
  config.flush
  name = "t01-caddy-#{Process.pid}-#{caddy_port}"
  output = Tempfile.new(["t01-caddy-log", ".log"], ROOT)
  command = [
    "docker", "run", "--rm", "--name", name,
    "--add-host", "host.docker.internal:host-gateway",
    "-e", "SITE_ADDRESS=http://127.0.0.1:#{caddy_port}",
    "-e", "SUB2API_ACTIVE_UPSTREAM=host.docker.internal:#{upstream_port}",
    "-p", "127.0.0.1:#{caddy_port}:#{caddy_port}",
    "-v", "#{config.path}:/etc/caddy/Caddyfile:ro",
    IMAGE, "caddy", "run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"
  ]
  pid = Process.spawn(*command, chdir: ROOT, out: output.path, err: output.path)
  [pid, name, config, output]
end

def stop_caddy(pid, name, config, output)
  system("docker", "stop", "--time", "2", name, out: File::NULL, err: File::NULL)
  Process.wait(pid) rescue nil
  logs = File.read(output.path)
  config.close!
  output.close!
  logs
end

def proxy_ready?(port)
  socket = TCPSocket.new("127.0.0.1", port)
  socket.write("GET /health HTTP/1.1\r\nHost: 127.0.0.1:#{port}\r\nConnection: close\r\n\r\n")
  socket.read.start_with?("HTTP/1.1 200")
rescue Errno::ECONNREFUSED, Errno::EHOSTUNREACH, Errno::ECONNRESET, EOFError
  false
ensure
  socket.close rescue nil
end

def wait_for_proxy(port, timeout_seconds: 20)
  deadline = Process.clock_gettime(Process::CLOCK_MONOTONIC) + timeout_seconds
  loop do
    return if proxy_ready?(port)
    fail_test("production Caddy policy did not proxy /health within #{timeout_seconds}s") if Process.clock_gettime(Process::CLOCK_MONOTONIC) >= deadline
    sleep 0.1
  end
end

def upload_request(port, body_bytes:, interval_seconds:, wait_timeout: 20)
  socket = TCPSocket.new("127.0.0.1", port)
  socket.write("POST /v1/responses HTTP/1.1\r\nHost: 127.0.0.1:#{port}\r\nContent-Length: #{body_bytes}\r\nConnection: close\r\n\r\n")
  body_bytes.times do
    socket.write("x")
    sleep interval_seconds if interval_seconds.positive?
  end
  Timeout.timeout(wait_timeout) { socket.read }
rescue Errno::EPIPE, Errno::ECONNRESET => error
  fail_test("continuous upload connection closed before request body completed: #{error.class}: #{error.message}")
ensure
  socket.close rescue nil
end

def incomplete_upload_result(port, wait_seconds:)
  socket = TCPSocket.new("127.0.0.1", port)
  socket.write("POST /v1/responses HTTP/1.1\r\nHost: 127.0.0.1:#{port}\r\nContent-Length: 8\r\nConnection: close\r\n\r\nx")
  sleep wait_seconds
  [:released, Timeout.timeout(10) { socket.read }]
rescue EOFError, Errno::ECONNRESET, Errno::EPIPE
  [:released, ""]
rescue Timeout::Error
  [:still_open, ""]
ensure
  socket.close rescue nil
end

def with_caddy(upstream_port, read_body_timeout: nil)
  caddy_port = free_port
  pid, name, config, output = start_caddy(caddy_port, upstream_port, read_body_timeout: read_body_timeout)
  wait_for_proxy(caddy_port)
  yield caddy_port
ensure
  logs = stop_caddy(pid, name, config, output) if pid
  if $!
    warn "Caddy logs:\n#{logs}" unless logs.nil? || logs.empty?
  end
end

unless system("docker", "image", "inspect", IMAGE, out: File::NULL, err: File::NULL)
  fail_test("required Caddy image is not available: #{IMAGE}")
end

slow_seconds = Integer(ENV.fetch("T01_SLOW_UPLOAD_DURATION_SECONDS", "4"))
short_repeats = Integer(ENV.fetch("T01_SHORT_UPLOAD_REPEATS", "1"))
fail_test("T01_SLOW_UPLOAD_DURATION_SECONDS must be positive") unless slow_seconds.positive?
fail_test("T01_SHORT_UPLOAD_REPEATS must be positive") unless short_repeats.positive?

upstream_port = free_port
upstream_server, upstream_thread, stop_upstream = start_upstream(upstream_port)

begin
  # This starts Caddy from the real production Caddyfile. Production values are
  # preserved; only SITE_ADDRESS and the otherwise external Sub2API upstream
  # are injected for the local harness.
  short_repeats.times do |index|
    with_caddy(upstream_port) do |caddy_port|
      response = upload_request(caddy_port, body_bytes: slow_seconds, interval_seconds: 1.0, wait_timeout: 30)
      fail_test("slow upload did not complete with upstream success: #{response.inspect}") unless response.start_with?("HTTP/1.1 200") && response.include?(%("ok":true))
    end
    puts "PASS: production-policy slow upload completed (#{slow_seconds}s, run #{index + 1}/#{short_repeats})"
  end

  # Contract evidence: the production policy is 15 minutes (validated by the
  # Caddy config test). This derived configuration changes only that one value
  # to make released-connection behavior observable in seconds; it is not a
  # substitute for the production timeout value.
  with_caddy(upstream_port, read_body_timeout: "2s") do |caddy_port|
    state, response = incomplete_upload_result(caddy_port, wait_seconds: 3)
    fail_test("incomplete upload was not released by the short derived policy") unless state == :released
    fail_test("incomplete upload unexpectedly reached upstream success: #{response.inspect}") if response.include?(%("ok":true))
  end
  puts "PASS: derived short-timeout incomplete upload released"
ensure
  stop_upstream.call
  upstream_server.close rescue nil
  upstream_thread.join(2)
end
