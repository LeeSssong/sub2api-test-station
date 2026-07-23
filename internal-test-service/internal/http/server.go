package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"example.invalid/internal-test-service/internal/authproxy"
	"example.invalid/internal-test-service/internal/publicsettings"
	"example.invalid/internal-test-service/internal/registration"
)

const maxAuthBodyBytes = 1 << 20

type Server struct {
	Registration    *registration.Service
	Auth            *authproxy.Service
	PublicSettings  *publicsettings.Service
	SchedulerStatus func() (time.Time, bool)
}

func NewServer(reg *registration.Service, auth *authproxy.Service, settings *publicsettings.Service) (*Server, error) {
	if reg == nil || auth == nil || settings == nil {
		return nil, errors.New("D04 HTTP dependencies are required")
	}
	return &Server{Registration: reg, Auth: auth, PublicSettings: settings}, nil
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc(authproxy.RegisterEndpoint, s.register)
	mux.HandleFunc(authproxy.LoginEndpoint, s.authenticate)
	mux.HandleFunc(authproxy.Login2FAEndpoint, s.authenticate)
	mux.HandleFunc("/api/v1/settings/public", s.publicSettings)
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	status := "pending"
	lastTick := ""
	if s.SchedulerStatus != nil {
		at, ok := s.SchedulerStatus()
		if !at.IsZero() {
			lastTick = at.UTC().Format(time.RFC3339)
			if ok {
				status = "ok"
			} else {
				status = "error"
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok", "service": "internal-test",
		"scheduler_status": status, "scheduler_last_tick": lastTick,
	})
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !requireSameOrigin(w, r) {
		return
	}
	body, ok := readBoundedBody(w, r)
	if !ok {
		return
	}
	response, err := s.Registration.Register(r.Context(), body, r.Header)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"code": "REGISTRATION_FORWARD_FAILED", "message": "注册服务暂时不可用"})
		return
	}
	writeProxyResponse(w, response)
}

func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !requireSameOrigin(w, r) {
		return
	}
	body, ok := readBoundedBody(w, r)
	if !ok {
		return
	}
	response, err := s.Auth.Authenticate(r.Context(), r.URL.Path, body, r.Header)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"code": "AUTH_FORWARD_FAILED", "message": "登录服务暂时不可用"})
		return
	}
	writeProxyResponse(w, response)
}

func (s *Server) publicSettings(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	response, err := s.PublicSettings.Get(r.Context(), r.Header)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"code": "PUBLIC_SETTINGS_FORWARD_FAILED", "message": "公开设置暂时不可用"})
		return
	}
	writeProxyResponse(w, response)
}

func readBoundedBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	if r.ContentLength > maxAuthBodyBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"code": "REQUEST_TOO_LARGE", "message": "请求内容过大"})
		return nil, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"code": "REQUEST_TOO_LARGE", "message": "请求内容过大"})
			return nil, false
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "INVALID_BODY", "message": "请求读取失败"})
		return nil, false
	}
	return body, true
}

func writeProxyResponse(w http.ResponseWriter, response authproxy.Response) {
	for _, key := range []string{"Content-Type", "Set-Cookie", "Authorization"} {
		for _, value := range response.Header.Values(key) {
			w.Header().Add(key, value)
		}
	}
	status := response.Status
	if status == 0 {
		status = http.StatusBadGateway
	}
	w.WriteHeader(status)
	_, _ = w.Write(response.Body)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Routes().ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"code": "METHOD_NOT_ALLOWED", "message": "请求方法不支持"})
	return false
}

func requireSameOrigin(w http.ResponseWriter, r *http.Request) bool {
	origin, err := url.Parse(r.Header.Get("Origin"))
	if err != nil || r.Header.Get("Origin") == "" || origin.Scheme == "" || origin.Host == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"code": "ORIGIN_REJECTED", "message": "请求来源不受信任"})
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]); forwarded == "http" || forwarded == "https" {
		scheme = forwarded
	}
	if origin.Scheme != scheme || !strings.EqualFold(origin.Host, r.Host) {
		writeJSON(w, http.StatusForbidden, map[string]string{"code": "ORIGIN_REJECTED", "message": "请求来源不受信任"})
		return false
	}
	return true
}
