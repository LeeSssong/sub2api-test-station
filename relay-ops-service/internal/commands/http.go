package commands

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"example.invalid/relay-ops-service/internal/feishuevents"
)

const maxCallbackBodyBytes = int64(256 << 10)

type EventDecoder interface {
	Decode(*http.Request, int64) (feishuevents.Envelope, error)
}

type EventRepository interface {
	InsertFeishuEvent(context.Context, Record) (bool, error)
}

type HTTPHandler struct {
	decoder    EventDecoder
	repository EventRepository
	now        func() time.Time
}

func NewHTTPHandler(decoder EventDecoder, repository EventRepository, now func() time.Time) http.Handler {
	return &HTTPHandler{decoder: decoder, repository: repository, now: now}
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.decoder == nil || h.repository == nil || h.now == nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	envelope, err := h.decoder.Decode(req, maxCallbackBodyBytes)
	if err != nil {
		switch {
		case errors.Is(err, feishuevents.ErrUnauthorized), errors.Is(err, feishuevents.ErrExpired):
			w.WriteHeader(http.StatusUnauthorized)
		case errors.Is(err, feishuevents.ErrTooLarge):
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
		return
	}
	if envelope.Challenge != "" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"challenge": envelope.Challenge})
		return
	}
	if envelope.Event == nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	decision := Parse(*envelope.Event)
	if decision.Ignore {
		w.WriteHeader(http.StatusOK)
		return
	}
	record := Record{
		EventID:      envelope.Event.EventID,
		MessageID:    envelope.Event.MessageID,
		ChatID:       envelope.Event.ChatID,
		SenderOpenID: envelope.Event.SenderOpenID,
		Command:      decision.Command,
		ActionKind:   decision.Action.Kind,
		GroupName:    decision.Action.GroupName,
		TargetRole:   decision.Action.TargetRole,
		Status:       StatusReceived,
		ErrorCode:    decision.ErrorCode,
		ReceivedAt:   h.now().UTC(),
	}
	if _, err := h.repository.InsertFeishuEvent(req.Context(), record); err != nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}
