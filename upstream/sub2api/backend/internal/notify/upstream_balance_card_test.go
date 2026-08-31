package notify

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderUpstreamBalanceCardP2ShowsOneWalletAndAllAccounts(t *testing.T) {
	payload, err := RenderUpstreamBalanceCard(UpstreamBalanceCardInput{
		State: UpstreamBalanceCardStateLow, ValueUSD: 4.25, BaseURL: "https://upstream.example",
		LoginAccount: "registry-user.invalid", LoginPassword: "fake-password-value",
		RecipientOpenIDs: []string{"ou-fake-a"},
		Accounts: []UpstreamBalanceCardAccount{
			{ID: 12, Name: "second", Ranks: []UpstreamBalanceCardRank{{GroupName: "GPT-Pro", Rank: intPointer(2)}}},
			{ID: 11, Name: "first", Ranks: []UpstreamBalanceCardRank{{GroupName: "GPT-Plus", Rank: intPointer(1)}, {GroupName: "Unranked"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var card interactiveCard
	if err := json.Unmarshal(payload, &card); err != nil {
		t.Fatal(err)
	}
	if card.Header.Title.Content != "上游账号余额不足" || card.Header.Template != "orange" || !card.Config.WideScreenMode {
		t.Fatalf("card header = %#v", card.Header)
	}
	text := renderedCardText(card)
	for _, value := range []string{"USD 4.25", "https://upstream.example", "registry-user.invalid", "fake-password-value", "first", "账号 ID：11", "GPT-Plus：第 1 名", "Unranked：未排名", "second", "账号 ID：12", "GPT-Pro：第 2 名"} {
		if !strings.Contains(text, value) {
			t.Fatalf("card missing %q: %s", value, text)
		}
	}
	for _, once := range []string{"USD 4.25", "https://upstream.example", "registry-user.invalid", "fake-password-value"} {
		if strings.Count(text, once) != 1 {
			t.Fatalf("%q appears %d times, want 1", once, strings.Count(text, once))
		}
	}
	if strings.Contains(text, "<at id=") {
		t.Fatalf("P2 card contains recipient mention: %s", text)
	}
	if strings.Index(text, "first") > strings.Index(text, "second") {
		t.Fatalf("accounts not ordered by best rank then ID: %s", text)
	}
}

func TestRenderUpstreamBalanceCardP1MentionsWithoutUrgency(t *testing.T) {
	payload, err := RenderUpstreamBalanceCard(UpstreamBalanceCardInput{
		State: UpstreamBalanceCardStateZero, ValueUSD: 0, BaseURL: "https://zero.example",
		LoginAccount: UnregisteredValue, LoginPassword: UnregisteredValue,
		RecipientOpenIDs: []string{"ou-fake-b", "ou-fake-a", "ou-fake-b"},
		Accounts:         []UpstreamBalanceCardAccount{{ID: 7, Name: "zero-account"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var card interactiveCard
	if err := json.Unmarshal(payload, &card); err != nil {
		t.Fatal(err)
	}
	if card.Header.Title.Content != "上游账号余额为 0" || card.Header.Template != "red" {
		t.Fatalf("card header = %#v", card.Header)
	}
	text := renderedCardText(card)
	if strings.Count(text, "<at id=ou-fake-a></at>") != 1 || strings.Count(text, "<at id=ou-fake-b></at>") != 1 {
		t.Fatalf("P1 mentions are not unique: %s", text)
	}
}

func TestRenderUpstreamBalanceCardPreservesTinyPositiveClassification(t *testing.T) {
	payload, err := RenderUpstreamBalanceCard(UpstreamBalanceCardInput{
		State: UpstreamBalanceCardStateLow, ValueUSD: 0.0000001, BaseURL: "https://tiny.example",
		LoginAccount: UnregisteredValue, LoginPassword: UnregisteredValue,
		Accounts: []UpstreamBalanceCardAccount{{ID: 1, Name: "tiny"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var card interactiveCard
	if err := json.Unmarshal(payload, &card); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(renderedCardText(card), "**当前余额**：USD 0.0000001\n") {
		t.Fatalf("tiny positive balance crossed zero display boundary: %s", payload)
	}
}

func TestRenderUpstreamBalanceCardRejectsOversizeWithoutTruncatingAccounts(t *testing.T) {
	accounts := make([]UpstreamBalanceCardAccount, 0, 500)
	for i := 0; i < 500; i++ {
		accounts = append(accounts, UpstreamBalanceCardAccount{ID: int64(i + 1), Name: strings.Repeat("long-account-name", 8)})
	}
	_, err := RenderUpstreamBalanceCard(UpstreamBalanceCardInput{
		State: UpstreamBalanceCardStateLow, ValueUSD: 1, BaseURL: "https://large.example",
		LoginAccount: "registry-user.invalid", LoginPassword: "fake-password-value", Accounts: accounts,
	})
	if err == nil || CardErrorCode(err) != CardErrorTooLarge {
		t.Fatalf("oversize error = %v code=%q", err, CardErrorCode(err))
	}
}

func intPointer(value int) *int { return &value }
