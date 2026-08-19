package handler

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func projectNativeUserErrorForContext(c *gin.Context, status int, errType, code, message string) service.NativeUserErrorProjection {
	selected := false
	stage := ""
	owner := ""
	if c != nil {
		if value, ok := c.Get(opsAccountIDKey); ok {
			switch accountID := value.(type) {
			case int64:
				selected = accountID > 0
			case int:
				selected = accountID > 0
			}
		}
		if selected {
			stage, owner = "upstream", "provider"
		} else if strings.EqualFold(errType, localCapacityExhaustedErrorCode) {
			stage, owner = "routing", "platform"
		}
	}
	return service.ProjectNativeUserError(service.NativeUserErrorInput{
		Status: status, Type: errType, Code: code, Message: message,
		Stage: stage, Ownership: owner, AccountSelected: selected,
	})
}
