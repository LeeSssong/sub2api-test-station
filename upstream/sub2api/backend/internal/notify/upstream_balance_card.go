package notify

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

const (
	UpstreamBalanceCardStateLow  = "low"
	UpstreamBalanceCardStateZero = "zero"

	CardErrorInvalid  = "card_invalid"
	CardErrorTooLarge = "card_too_large"

	maxCardBytes = 30 << 10
)

type cardRenderError struct{ code string }

func (e *cardRenderError) Error() string { return "upstream balance card cannot be rendered" }

func CardErrorCode(err error) string {
	var target *cardRenderError
	if errors.As(err, &target) {
		return target.code
	}
	return ""
}

type UpstreamBalanceCardRank struct {
	GroupName string
	Rank      *int
}

type UpstreamBalanceCardAccount struct {
	ID         int64
	Name       string
	BalanceUSD *float64
	Ranks      []UpstreamBalanceCardRank
}

type UpstreamBalanceCardInput struct {
	State            string
	ValueUSD         float64
	BaseURL          string
	LoginAccount     string
	LoginPassword    string
	RecipientOpenIDs []string
	Accounts         []UpstreamBalanceCardAccount
	SilenceToken     string
}

type interactiveCardText struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

type interactiveCardConfig struct {
	WideScreenMode bool `json:"wide_screen_mode"`
}

type interactiveCardHeader struct {
	Title    interactiveCardText `json:"title"`
	Template string              `json:"template"`
}

type interactiveCardElement struct {
	Tag     string                  `json:"tag"`
	Text    *interactiveCardText    `json:"text,omitempty"`
	Actions []interactiveCardAction `json:"actions,omitempty"`
}

type interactiveCardAction struct {
	Tag   string              `json:"tag"`
	Text  interactiveCardText `json:"text"`
	Type  string              `json:"type"`
	Value map[string]string   `json:"value"`
}

type interactiveCard struct {
	Config   interactiveCardConfig    `json:"config"`
	Header   interactiveCardHeader    `json:"header"`
	Elements []interactiveCardElement `json:"elements"`
}

func RenderUpstreamBalanceCard(input UpstreamBalanceCardInput) ([]byte, error) {
	if (input.State != UpstreamBalanceCardStateLow && input.State != UpstreamBalanceCardStateZero) ||
		math.IsNaN(input.ValueUSD) || math.IsInf(input.ValueUSD, 0) || input.ValueUSD < 0 || strings.TrimSpace(input.BaseURL) == "" || len(input.Accounts) == 0 {
		return nil, &cardRenderError{code: CardErrorInvalid}
	}
	title, template := "上游账号余额不足", "orange"
	if input.State == UpstreamBalanceCardStateZero {
		title, template = "上游账号余额为 0", "red"
	}
	loginAccount := strings.TrimSpace(input.LoginAccount)
	loginPassword := strings.TrimSpace(input.LoginPassword)
	if loginAccount == "" {
		loginAccount = UnregisteredValue
	}
	if loginPassword == "" {
		loginPassword = UnregisteredValue
	}
	elements := make([]interactiveCardElement, 0, len(input.Accounts)+2)
	if input.State == UpstreamBalanceCardStateZero {
		if mentions := renderMentions(input.RecipientOpenIDs); mentions != "" {
			elements = append(elements, markdownElement(mentions))
		}
	}
	wallet := strings.Join([]string{
		"**当前余额**：USD " + formatBalanceUSD(input.ValueUSD),
		"**BaseURL**：" + cardValue(input.BaseURL),
		"**上游登录账号**：" + cardValue(loginAccount),
		"**上游登录密码**：" + cardValue(loginPassword),
	}, "\n")
	elements = append(elements, markdownElement(wallet))
	if strings.TrimSpace(input.SilenceToken) != "" {
		elements = append(elements, actionElement([]interactiveCardAction{
			cardAction("静默 1 小时", "default", "1h", input.SilenceToken),
			cardAction("静默 6 小时", "default", "6h", input.SilenceToken),
			cardAction("静默 24 小时", "primary", "24h", input.SilenceToken),
		}))
	}
	elements = append(elements, markdownElement("**关联活跃账号**"))
	accounts := append([]UpstreamBalanceCardAccount(nil), input.Accounts...)
	sort.SliceStable(accounts, func(i, j int) bool {
		left, right := bestCardRank(accounts[i]), bestCardRank(accounts[j])
		if left != right {
			return left < right
		}
		return accounts[i].ID < accounts[j].ID
	})
	for _, account := range accounts {
		name := strings.TrimSpace(account.Name)
		if name == "" {
			name = "未命名账号"
		}
		lines := []string{"**" + cardValue(name) + "**", "账号 ID：" + strconv.FormatInt(account.ID, 10)}
		if account.BalanceUSD != nil && !math.IsNaN(*account.BalanceUSD) && !math.IsInf(*account.BalanceUSD, 0) && *account.BalanceUSD >= 0 {
			lines = append(lines, "余额：USD "+formatBalanceUSD(*account.BalanceUSD))
		}
		ranks := append([]UpstreamBalanceCardRank(nil), account.Ranks...)
		sort.SliceStable(ranks, func(i, j int) bool {
			left, right := rankSortValue(ranks[i].Rank), rankSortValue(ranks[j].Rank)
			if left != right {
				return left < right
			}
			return ranks[i].GroupName < ranks[j].GroupName
		})
		if len(ranks) == 0 {
			lines = append(lines, "分组排名：未排名")
		} else {
			for _, rank := range ranks {
				label := "未排名"
				if rank.Rank != nil && *rank.Rank > 0 {
					label = fmt.Sprintf("第 %d 名", *rank.Rank)
				}
				groupName := strings.TrimSpace(rank.GroupName)
				if groupName == "" {
					groupName = "未命名分组"
				}
				lines = append(lines, cardValue(groupName)+"："+label)
			}
		}
		elements = append(elements, markdownElement(strings.Join(lines, "\n")))
	}
	card := interactiveCard{
		Config:   interactiveCardConfig{WideScreenMode: true},
		Header:   interactiveCardHeader{Title: interactiveCardText{Tag: "plain_text", Content: title}, Template: template},
		Elements: elements,
	}
	payload, err := json.Marshal(card)
	if err != nil {
		return nil, &cardRenderError{code: CardErrorInvalid}
	}
	if len(payload) > maxCardBytes {
		return nil, &cardRenderError{code: CardErrorTooLarge}
	}
	return payload, nil
}

func markdownElement(content string) interactiveCardElement {
	return interactiveCardElement{Tag: "div", Text: &interactiveCardText{Tag: "lark_md", Content: content}}
}

func actionElement(actions []interactiveCardAction) interactiveCardElement {
	return interactiveCardElement{Tag: "action", Actions: actions}
}

func cardAction(label, actionType, duration, token string) interactiveCardAction {
	return interactiveCardAction{
		Tag: "button", Text: interactiveCardText{Tag: "plain_text", Content: label}, Type: actionType,
		Value: map[string]string{"action": "silence", "duration": duration, "token": token},
	}
}

func renderedCardText(card interactiveCard) string {
	parts := []string{card.Header.Title.Content}
	for _, element := range card.Elements {
		if element.Text != nil {
			parts = append(parts, element.Text.Content)
		}
	}
	return strings.Join(parts, "\n")
}

func renderMentions(openIDs []string) string {
	seen := make(map[string]struct{}, len(openIDs))
	values := make([]string, 0, len(openIDs))
	for _, openID := range openIDs {
		openID = strings.TrimSpace(openID)
		if openID == "" {
			continue
		}
		if _, exists := seen[openID]; exists {
			continue
		}
		seen[openID] = struct{}{}
		values = append(values, openID)
	}
	sort.Strings(values)
	for i := range values {
		values[i] = "<at id=" + cardAttribute(values[i]) + "></at>"
	}
	return strings.Join(values, " ")
}

func cardValue(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\r", " ", "\n", " ")
	return replacer.Replace(strings.TrimSpace(value))
}

func cardAttribute(value string) string {
	return strings.NewReplacer("&", "", "<", "", ">", "", "\"", "", "'", "", " ", "").Replace(value)
}

func formatBalanceUSD(value float64) string {
	formatted := strconv.FormatFloat(value, 'f', 6, 64)
	rounded, err := strconv.ParseFloat(formatted, 64)
	if err != nil || balanceDisplayClass(rounded) != balanceDisplayClass(value) {
		formatted = strconv.FormatFloat(value, 'f', -1, 64)
	}
	formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
	if !strings.Contains(formatted, ".") {
		formatted += ".00"
	} else if len(formatted)-strings.IndexByte(formatted, '.')-1 == 1 {
		formatted += "0"
	}
	return formatted
}

func balanceDisplayClass(value float64) string {
	switch {
	case value == 0:
		return UpstreamBalanceCardStateZero
	case value > 0 && value < 5:
		return UpstreamBalanceCardStateLow
	default:
		return "healthy"
	}
}

func bestCardRank(account UpstreamBalanceCardAccount) int {
	best := int(^uint(0) >> 1)
	for _, rank := range account.Ranks {
		if rank.Rank != nil && *rank.Rank > 0 && *rank.Rank < best {
			best = *rank.Rank
		}
	}
	return best
}

func rankSortValue(rank *int) int {
	if rank == nil || *rank <= 0 {
		return int(^uint(0) >> 1)
	}
	return *rank
}
