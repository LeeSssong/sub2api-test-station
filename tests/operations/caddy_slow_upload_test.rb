#!/usr/bin/env ruby
# frozen_string_literal: true

require "json"
require "socket"
require "tempfile"
require "timeout"

IMAGE = "caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d"
ROOT = File.expand_path("../..", __dir__)

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

def wait_for_port(port, timeout_seconds: 15)
  deadline = Process.clock_gettime(Process::CLOCK_MONOTONIC) + timeout_seconds
  loop do
    begin
      socket = TCPSocket.new("127.0.0.1", port)
      socket.close
      return
    rescue Errno::ECONNREFUSED, Errno::EHOSTUNREACH
      fail_test("port #{port} did not become ready") if Process.clock_gettime(Process::CLOCK_MONOTONIC) >= deadline
      sleep 0.1
    end
  end
end

def start_upstream(port)
  server = TCPServer.new("127.0.0.1", port)
  stop = false
  thread = Thread.new do
    until stop
      begin
        socket = server.accept_nonblock
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

      Thread.new(socket) do |client|
        begin
          header = +""
          header << client.readpartial(4096) until header.include?("\r\n\r\n")
          content_length = header[/\r\nContent-Length:\s*(\d+)/i, 1].to_i
          body = header.split("\r\n\r\n", 2).fetch(1).dup
          body << client.readpartial([content_length - body.bytesize, 4096].min) while body.bytesize < content_length
          response = {"ok" => true, "bytes" => body.bytesize}.to_json
          client.write("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: #{response.bytesize}\r\nConnection: close\r\n\r\n#{response}")
        rescue EOFError, Errno::ECONNRESET, IOError
          # The incomplete-upload case is expected to close before the body ends.
        ensure
          client.close rescue nil
        end
      end
    end
  end
  [server, thread, -> { stop = true; server.close rescue nil; thread.join(2) }]
end

def run_caddy(caddy_port, upstream_port, read_body_timeout:, response_header_timeout:)
  config = Tempfile.new(["t01-caddy", ".Caddyfile"], ROOT)
  config.write(<<~CADDY)
    {
      auto_https off
      servers {
        timeouts {
          read_body #{read_body_timeout}
        }
      }
    }

    :#{caddy_port} {
      reverse_proxy host.docker.internal:#{upstream_port} {
        transport http {
          response_header_timeout #{response_header_timeout}
        }
      }
      log {
        output stdout
        format json
      }
    }
  CADDY
  config.flush
  command = [
    "docker", "run", "--rm", "--name", "t01-caddy-#{Process.pid}-#{caddy_port}",
    "--add-host", "host.docker.internal:host-gateway",
    "-p", "127.0.0.1:#{caddy_port}:#{caddy_port}",
    "-v", "#{config.path}:/etc/caddy/Caddyfile:ro",
    IMAGE, "caddy", "run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"
  ]
  output = Tempfile.new(["t01-caddy-log", ".log"], ROOT)
  pid = Process.spawn(*command, chdir: ROOT, out: output.path, err: output.path)
  begin
    wait_for_port(caddy_port)
  rescue StandardError => error
    Process.wait(pid, Process::WNOHANG) rescue nil
    output.flush
    fail_test("Caddy did not start: #{error.message}\n#{File.read(output.path)}")
  end
  [pid, config, output]
rescue StandardError
  config.close! if config
  raise
end

def stop_caddy(pid, config, output)
  Process.kill("TERM", pid) rescue nil
  Process.wait(pid) rescue nil
  logs = File.read(output.path)
  config.close!
  output.close!
  logs
end

def upload_request(port, body_bytes:, interval_seconds:, wait_timeout: 20)
  socket = TCPSocket.new("127.0.0.1", port)
  socket.write("POST /v1/responses HTTP/1.1\r\nHost: sub2api.example.test\r\nContent-Length: #{body_bytes}\r\nConnection: close\r\n\r\n")
  body_bytes.times do
    socket.write("x")
    sleep interval_seconds if interval_seconds.positive?
  end
  response = Timeout.timeout(wait_timeout) { socket.read }
  response
ensure
  socket.close if socket
end

def incomplete_upload(port, wait_seconds:)
  socket = TCPSocket.new("127.0.0.1", port)
  socket.write("POST /v1/responses HTTP/1.1\r\nHost: sub2api.example.test\r\nContent-Length: 8\r\nConnection: close\r\n\r\nx")
  sleep wait_seconds
  Timeout.timeout(10) { socket.read }
rescue EOFError, Errno::ECONNRESET, Errno::EPIPE
  ""
ensure
  socket.close if socket
end

unless system("docker", "image", "inspect", IMAGE, out: File::NULL, err: File::NULL)
  fail_test("required Caddy image is not available: #{IMAGE}")
end

upstream_port = free_port
upstream_server, upstream_thread, stop_upstream = start_upstream(upstream_port)

begin
  # Contract mode is intentionally short. Set T01_SLOW_UPLOAD_DURATION_SECONDS=301
  # for the full >300-second acceptance run against the same 15m policy.
  slow_seconds = Integer(ENV.fetch("T01_SLOW_UPLOAD_DURATION_SECONDS", "4"))
  interval = slow_seconds.zero? ? 0 : 1.0
  caddy_port = free_port
  pid, config, output = run_caddy(caddy_port, upstream_port, read_body_timeout: "15m", response_header_timeout: "15m")
  begin
    response = upload_request(caddy_port, body_bytes: slow_seconds, interval_seconds: interval, wait_timeout: 30)
  ensure
    logs = stop_caddy(pid, config, output)
  end
  fail_test("slow upload did not complete with upstream success: #{response.inspect}") unless response.start_with?("HTTP/1.1 200") && response.include?(%("ok":true))
  fail_test("slow upload access log did not contain the request") unless logs.include?("/v1/responses")
  puts "PASS: controlled slow upload completed (#{slow_seconds}s)"

  # Use a short override only to make abandoned-body cleanup testable without
  # waiting 15 minutes. This is not a production value.
  caddy_port = free_port
  pid, config, output = run_caddy(caddy_port, upstream_port, read_body_timeout: "2s", response_header_timeout: "10s")
  begin
    response = incomplete_upload(caddy_port, wait_seconds: 3)
  ensure
    logs = stop_caddy(pid, config, output)
  end
  fail_test("incomplete upload unexpectedly reached upstream success: #{response.inspect}") if response.include?(%("ok":true))
  fail_test("incomplete upload access log did not contain the request") unless logs.include?("/v1/responses")
  puts "PASS: controlled incomplete upload was released"
ensure
  stop_upstream.call
  upstream_server.close rescue nil
  upstream_thread.join(2)
end
