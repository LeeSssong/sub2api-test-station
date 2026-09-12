package handler

import (
	"context"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
)

func advanceOpenAIWSCyberBlockState(blocked, pending, marked bool, turnErr error) (bool, bool) {
	var failoverErr *service.UpstreamFailoverError
	isFailover := errors.As(turnErr, &failoverErr)
	if marked {
		if isFailover {
			return false, true
		}
		return true, false
	}
	if pending && !isFailover {
		return true, false
	}
	return blocked, pending
}

func openAIWSIngressEndedByClient(err error) bool {
	if err == nil {
		return true
	}
	var closeErr *service.OpenAIWSClientCloseError
	if errors.As(err, &closeErr) && closeErr.StatusCode() == coderws.StatusNormalClosure {
		return true
	}
	if coderws.CloseStatus(err) == coderws.StatusNormalClosure {
		return true
	}
	return errors.Is(err, context.Canceled)
}
