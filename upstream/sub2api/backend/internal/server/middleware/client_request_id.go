package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const clientRequestIDHeader = "X-Client-Request-ID"

const (
	threadIDHeader         = "X-Codex-Thread-Id"
	windowIDHeader         = "X-Codex-Window-Id"
	sessionIDHeader        = "X-Session-Id"
	logicalRequestIDHeader = "X-Logical-Request-Id"
)

func setOptionalCorrelationHeaders(req *http.Request, ctx context.Context) context.Context {
	if req == nil {
		return ctx
	}
	for _, item := range []struct {
		header string
		key    ctxkey.Key
	}{
		{threadIDHeader, ctxkey.ThreadID},
		{windowIDHeader, ctxkey.WindowID},
		{sessionIDHeader, ctxkey.SessionID},
		{logicalRequestIDHeader, ctxkey.LogicalRequestID},
	} {
		if value, valid := normalizeCorrelationID(req.Header.Get(item.header)); valid {
			ctx = context.WithValue(ctx, item.key, value)
		}
	}
	return ctx
}

// ClientRequestID ensures every request has a unique client_request_id in request.Context().
//
// This is used by the Ops monitoring module for end-to-end request correlation.
func ClientRequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request == nil {
			c.Next()
			return
		}

		if v, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(v) != "" {
			var valid bool
			v, valid = normalizeCorrelationID(v)
			if !valid {
				v = uuid.New().String()
			}
			c.Header(clientRequestIDHeader, v)
			ctx := context.WithValue(c.Request.Context(), ctxkey.ClientRequestID, v)
			ctx = setOptionalCorrelationHeaders(c.Request, ctx)
			c.Request = c.Request.WithContext(ctx)
			c.Next()
			return
		}

		id := uuid.New().String()
		c.Header(clientRequestIDHeader, id)
		ctx := context.WithValue(c.Request.Context(), ctxkey.ClientRequestID, id)
		ctx = setOptionalCorrelationHeaders(c.Request, ctx)
		requestLogger := logger.FromContext(ctx).With(zap.String("client_request_id", strings.TrimSpace(id)))
		ctx = logger.IntoContext(ctx, requestLogger)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
