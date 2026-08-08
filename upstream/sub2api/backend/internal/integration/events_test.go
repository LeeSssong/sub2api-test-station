package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestContractRequestCompletedUsesDecimalStringsAndNoCredentials(t *testing.T) {
	event, err := NewEvent(EventTypeRequestCompleted, "sub2api-test", time.Now(), RequestCompleted{
		RequestID: "req-1", AccountID: 42, Model: "gpt-test", PromptTokens: 10,
		CompletionTokens: 5, UserCharge: "0.0010", ActualCost: "0.0004", Currency: "USD",
	})
	require.NoError(t, err)
	require.Equal(t, ContractVersion, event.ContractVersion)
	require.NotEmpty(t, event.EventID)
	require.Equal(t, "UTC", event.OccurredAt.Location().String(), "constructor normalizes timestamps to UTC")
	require.NotContains(t, string(event.Payload), "api_key")
	require.NoError(t, event.Validate())
}

func TestContractRejectsCredentialFieldsAndUnsupportedVersions(t *testing.T) {
	event, err := NewEvent(EventTypeAccountHealthChanged, "sub2api-test", time.Now(), map[string]any{
		"account_id": 1, "status": "healthy", "api_key": "must-not-cross-boundary",
	})
	require.Error(t, err)
	require.Nil(t, event.Payload)

	event = Event{EventID: "550e8400-e29b-41d4-a716-446655440000", Type: EventTypeAccountHealthChanged,
		OccurredAt: time.Now(), SourceVersion: "test", ContractVersion: 99, Payload: []byte(`{"account_id":1}`)}
	require.Error(t, event.Validate())
}

func TestContractAccountUpdateCommandIsWhitelisted(t *testing.T) {
	command, err := NewCommand(CommandAccountUpdate, "admin-1", AccountUpdate{AccountID: 7, Fields: map[string]any{"status": "active"}})
	require.NoError(t, err)
	require.Equal(t, ContractVersion, command.ContractVersion)
	require.NoError(t, command.Validate())

	_, err = NewCommand("accounts.delete", "admin-1", map[string]any{"account_id": 7})
	require.Error(t, err)
}

func TestContractHealthAndBalancePayloadsRequireFacts(t *testing.T) {
	_, err := NewEvent(EventTypeAccountHealthChanged, "sub2api-test", time.Now(), AccountHealthChanged{
		AccountID: 7, Status: "healthy", CheckedAt: time.Now(),
	})
	require.NoError(t, err)

	event, err := NewEvent(EventTypeAccountBalanceSnapshot, "sub2api-test", time.Now(), AccountBalanceSnapshot{
		AccountID: 7, Balance: "12.3400", Currency: "USD", CapturedAt: time.Now(),
	})
	require.NoError(t, err)
	require.NoError(t, event.Validate())

	_, err = NewEvent(EventTypeAccountBalanceSnapshot, "sub2api-test", time.Now(), AccountBalanceSnapshot{
		AccountID: 7, Balance: "not-a-decimal", Currency: "USD", CapturedAt: time.Now(),
	})
	require.Error(t, err)
}
