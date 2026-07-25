package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
)

func TestNewOfficialHTTPClientDialsLoopbackAndPreservesHost(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "api.xingqiaolab.top" {
			t.Fatalf("host = %q", r.Host)
		}
		_, _ = w.Write([]byte("ok"))
	})}
	defer server.Close()
	go func() {
		_ = server.Serve(listener)
	}()

	client, err := newOfficialHTTPClient(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.Get("http://api.xingqiaolab.top/api/v1/auth/me")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); got != "ok" {
		t.Fatalf("body = %q", got)
	}
}

func TestNewOfficialHTTPClientRejectsNonLoopbackDialAddress(t *testing.T) {
	for _, address := range []string{"10.0.0.2:443", "api.xingqiaolab.top:443", "missing-port"} {
		t.Run(fmt.Sprintf("address=%s", address), func(t *testing.T) {
			if _, err := newOfficialHTTPClient(address); err == nil {
				t.Fatalf("accepted %q", address)
			}
		})
	}
}
