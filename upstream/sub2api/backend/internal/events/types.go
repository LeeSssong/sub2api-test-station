// Package events owns persistence and delivery of integration events. The
// envelope itself is defined in internal/integration so producers and
// consumers compile against the same contract.
package events

import "github.com/Wei-Shaw/sub2api/internal/integration"

type Event = integration.Event

const (
	ContractVersion                 = integration.ContractVersion
	EventTypeRequestCompleted       = integration.EventTypeRequestCompleted
	EventTypeAccountHealthChanged   = integration.EventTypeAccountHealthChanged
	EventTypeAccountBalanceSnapshot = integration.EventTypeAccountBalanceSnapshot
)

type ErrorClass string

const (
	ErrorClassTransient ErrorClass = "transient"
	ErrorClassPermanent ErrorClass = "permanent"
	ErrorClassContract  ErrorClass = "contract"
	ErrorClassUnknown   ErrorClass = "unknown"
)

func ClassifyError(err error) ErrorClass {
	if err == nil {
		return ""
	}
	if classified, ok := err.(interface{ ErrorClass() ErrorClass }); ok {
		return classified.ErrorClass()
	}
	return ErrorClassUnknown
}
