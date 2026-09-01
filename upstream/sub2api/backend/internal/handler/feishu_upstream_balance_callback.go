package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type FeishuUpstreamBalanceCallbackHandler struct {
	service *service.UpstreamBalanceNotificationService
}

func NewFeishuUpstreamBalanceCallbackHandler(svc *service.UpstreamBalanceNotificationService) *FeishuUpstreamBalanceCallbackHandler {
	return &FeishuUpstreamBalanceCallbackHandler{service: svc}
}

func (h *FeishuUpstreamBalanceCallbackHandler) Handle(c *gin.Context) {
	if h == nil || h.service == nil {
		c.Status(http.StatusNotFound)
		return
	}
	var payload struct {
		Type      string `json:"type"`
		Challenge string `json:"challenge"`
		Token     string `json:"token"`
		OpenID    string `json:"open_id"`
		Action    struct {
			Value map[string]string `json:"value"`
		} `json:"action"`
		Header struct {
			Token string `json:"token"`
		} `json:"header"`
		Event struct {
			Operator struct {
				OpenID     string `json:"open_id"`
				OperatorID struct {
					OpenID string `json:"open_id"`
				} `json:"operator_id"`
			} `json:"operator"`
			Action struct {
				Value map[string]string `json:"value"`
			} `json:"action"`
		} `json:"event"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request"})
		return
	}
	callbackToken := firstNonEmptyValue(c.GetHeader("X-Feishu-Callback-Token"), payload.Header.Token, payload.Token)
	if payload.Type == "url_verification" {
		if !h.service.ValidateFeishuCallbackToken(callbackToken) {
			c.JSON(http.StatusForbidden, gin.H{"code": "forbidden"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"challenge": payload.Challenge})
		return
	}
	value := payload.Event.Action.Value
	if value == nil {
		value = payload.Action.Value
	}
	operatorOpenID := firstNonEmptyValue(payload.Event.Operator.OperatorID.OpenID, payload.Event.Operator.OpenID, payload.OpenID)
	message, err := h.service.SilenceFromFeishu(c.Request.Context(),
		callbackToken, operatorOpenID,
		strings.TrimSpace(value["token"]), strings.TrimSpace(value["duration"]))
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"code": "forbidden"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": message})
}

func firstNonEmptyValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
