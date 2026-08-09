package adapter

import (
	"context"
	"testing"

	"example.invalid/relay-ops-service/internal/billing"
)

func TestCommandAdapterUsesOfficialAPIForAccountUpdates(t *testing.T) {
	client := &officialClientStub{}
	command := billing.AccountUpdateCommand{CommandID: "command-1", ActorID: 8, AccountID: 4, IdempotencyKey: "account:4:priority:1", Fields: map[string]any{"priority": 3}}
	if err := (Sub2API{Client: client}).SendAccountUpdateCommand(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	if client.command.AccountID != 4 || client.command.Fields["priority"] != 3 {
		t.Fatalf("official command=%+v", client.command)
	}
}

type officialClientStub struct{ command billing.AccountUpdateCommand }

func (*officialClientStub) RefreshAccount(context.Context, int64, string) error { return nil }
func (c *officialClientStub) UpdateAccount(_ context.Context, command billing.AccountUpdateCommand) error {
	c.command = command
	return nil
}
