package commands

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/feishuevents"
)

func TestHTTPHandlerReturnsChallengeWithoutPersistence(t *testing.T) {
	repository := &fakeEventRepository{}
	handler := NewHTTPHandler(fakeDecoder{envelope: feishuevents.Envelope{Challenge: "challenge-value"}}, repository, fixedNow)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/relay-ops/api/feishu/events", strings.NewReader(`{"encrypt":"value"}`)))
	if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != `{"challenge":"challenge-value"}` || len(repository.records) != 0 {
		t.Fatalf("response=%d %q records=%#v", recorder.Code, recorder.Body.String(), repository.records)
	}
}

func TestHTTPHandlerPersistsAcceptedAndUnknownGroupMessagesOnce(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		command   string
		errorCode string
	}{
		{"accepted", "切换 GPT-Pro 到灾备", "切换 GPT-Pro 到灾备", ""},
		{"unknown", "切换到某个分组", "", ErrorUnknownCommand},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := validEvent(tt.text)
			repository := &fakeEventRepository{}
			handler := NewHTTPHandler(fakeDecoder{envelope: feishuevents.Envelope{Event: &event}}, repository, fixedNow)
			for i := 0; i < 2; i++ {
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/relay-ops/api/feishu/events", nil))
				if recorder.Code != http.StatusOK {
					t.Fatalf("call %d status = %d", i, recorder.Code)
				}
			}
			if len(repository.records) != 1 || repository.records[0].Command != tt.command || repository.records[0].ErrorCode != tt.errorCode || repository.records[0].Status != StatusReceived {
				t.Fatalf("records = %#v", repository.records)
			}
		})
	}
}

func TestHTTPHandlerIgnoresNonCommandEventsAndMapsFailures(t *testing.T) {
	t.Run("private is ignored", func(t *testing.T) {
		event := validEvent("查询当前分组状态")
		event.ChatType = "p2p"
		repository := &fakeEventRepository{}
		recorder := httptest.NewRecorder()
		NewHTTPHandler(fakeDecoder{envelope: feishuevents.Envelope{Event: &event}}, repository, fixedNow).ServeHTTP(
			recorder, httptest.NewRequest(http.MethodPost, "/relay-ops/api/feishu/events", nil),
		)
		if recorder.Code != http.StatusOK || len(repository.records) != 0 {
			t.Fatalf("status=%d records=%#v", recorder.Code, repository.records)
		}
	})

	for _, tt := range []struct {
		name string
		err  error
		want int
	}{
		{"unauthorized", feishuevents.ErrUnauthorized, http.StatusUnauthorized},
		{"expired", feishuevents.ErrExpired, http.StatusUnauthorized},
		{"large", feishuevents.ErrTooLarge, http.StatusRequestEntityTooLarge},
		{"malformed", feishuevents.ErrMalformed, http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			NewHTTPHandler(fakeDecoder{err: tt.err}, &fakeEventRepository{}, fixedNow).ServeHTTP(
				recorder, httptest.NewRequest(http.MethodPost, "/relay-ops/api/feishu/events", nil),
			)
			if recorder.Code != tt.want {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.want)
			}
		})
	}

	t.Run("database unavailable", func(t *testing.T) {
		event := validEvent("查询当前分组状态")
		recorder := httptest.NewRecorder()
		NewHTTPHandler(fakeDecoder{envelope: feishuevents.Envelope{Event: &event}}, &fakeEventRepository{err: errors.New("database down")}, fixedNow).ServeHTTP(
			recorder, httptest.NewRequest(http.MethodPost, "/relay-ops/api/feishu/events", nil),
		)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d", recorder.Code)
		}
	})
}

type fakeDecoder struct {
	envelope feishuevents.Envelope
	err      error
}

func (f fakeDecoder) Decode(*http.Request, int64) (feishuevents.Envelope, error) {
	return f.envelope, f.err
}

type fakeEventRepository struct {
	records []Record
	err     error
}

func (f *fakeEventRepository) InsertFeishuEvent(_ context.Context, record Record) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	for _, existing := range f.records {
		if existing.EventID == record.EventID {
			return false, nil
		}
	}
	f.records = append(f.records, record)
	return true, nil
}

func fixedNow() time.Time {
	return time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
}
