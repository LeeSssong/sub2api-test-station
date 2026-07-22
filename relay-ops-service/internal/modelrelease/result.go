package modelrelease

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

const maxResultBytes = 2 << 20

var (
	lowercaseSHA256 = regexp.MustCompile(`\A[0-9a-f]{64}\z`)
	modelIDPattern  = regexp.MustCompile(`\A[a-z0-9][a-z0-9._-]{0,127}\z`)
	forbiddenKey    = regexp.MustCompile(`(?i)\A(?:api[_-]?key|token|cookie|authorization|password|secret|credentials?|model[_-]?output)\z`)
)

type Result struct {
	SchemaVersion    int       `json:"schema_version"`
	ProposalID       string    `json:"proposal_id"`
	SnapshotID       string    `json:"snapshot_id"`
	EvaluatedAt      time.Time `json:"evaluated_at"`
	Status           string    `json:"status"`
	AccountSetSHA256 string    `json:"account_set_sha256"`
	BaseConfigSHA256 string    `json:"base_config_sha256"`
	Published        Catalog   `json:"published"`
	Candidate        Candidate `json:"candidate"`
	Groups           []Group   `json:"groups"`
	Accounts         []Account `json:"accounts"`
	Evidence         Evidence  `json:"evidence"`
	Blockers         []string  `json:"blockers"`
}

type Catalog struct {
	Families []string `json:"families"`
	Models   []string `json:"models"`
}

type Candidate struct {
	Family       *string  `json:"family"`
	Families     []string `json:"families"`
	Models       []string `json:"models"`
	ReviewModels []string `json:"review_models"`
}

type Group struct {
	GroupID       int64    `json:"group_id"`
	Name          string   `json:"name"`
	Covered       bool     `json:"covered"`
	CoveredModels []string `json:"covered_models"`
	MissingModels []string `json:"missing_models"`
}

type Account struct {
	AccountID       int64    `json:"account_id"`
	GroupIDs        []int64  `json:"group_ids"`
	QualifiedModels []string `json:"qualified_models"`
}

type Evidence struct {
	CapturedAt        time.Time `json:"captured_at"`
	FreshnessMinutes  int64     `json:"freshness_minutes"`
	BalanceMinUSD     float64   `json:"balance_min_usd"`
	QualitySamplesMin int64     `json:"quality_samples_min"`
}

type View struct {
	Available         bool
	Stale             bool
	Status            string
	ProposalID        string
	EvaluatedAt       string
	PublishedFamilies string
	PublishedModels   string
	CandidateFamilies string
	CandidateModels   string
	ReviewModels      string
	AccountSetSHA256  string
	BaseConfigSHA256  string
	Groups            []GroupView
	Accounts          []AccountView
	Blockers          []string
}

type GroupView struct {
	GroupID, Name, Coverage, MissingModels string
}

type AccountView struct {
	AccountID, Groups, QualifiedModels string
}

func Load(path string, now time.Time) (Result, error) {
	if strings.TrimSpace(path) == "" {
		return Result{}, fmt.Errorf("model release evidence is unavailable")
	}
	file, err := os.Open(path)
	if err != nil {
		return Result{}, fmt.Errorf("model release evidence is unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxResultBytes {
		return Result{}, fmt.Errorf("model release evidence is invalid")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxResultBytes+1))
	if err != nil || len(data) > maxResultBytes {
		return Result{}, fmt.Errorf("model release evidence is invalid")
	}

	generic, err := decodeGeneric(data)
	if err != nil || containsForbiddenKey(generic) || !proposalHashMatches(generic) {
		return Result{}, fmt.Errorf("model release evidence is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var result Result
	if err := decoder.Decode(&result); err != nil || ensureEOF(decoder) != nil || validate(result, now.UTC()) != nil {
		return Result{}, fmt.Errorf("model release evidence is invalid")
	}
	return result, nil
}

func (r Result) View(now time.Time) View {
	groups := make([]GroupView, 0, len(r.Groups))
	for _, group := range r.Groups {
		coverage := "完整"
		if !group.Covered {
			coverage = "缺失"
		}
		groups = append(groups, GroupView{
			GroupID: fmt.Sprintf("%d", group.GroupID), Name: group.Name,
			Coverage: coverage, MissingModels: strings.Join(group.MissingModels, ", "),
		})
	}
	accounts := make([]AccountView, 0, len(r.Accounts))
	for _, account := range r.Accounts {
		groupIDs := make([]string, 0, len(account.GroupIDs))
		for _, groupID := range account.GroupIDs {
			groupIDs = append(groupIDs, fmt.Sprintf("%d", groupID))
		}
		accounts = append(accounts, AccountView{
			AccountID: fmt.Sprintf("%d", account.AccountID), Groups: strings.Join(groupIDs, ", "),
			QualifiedModels: strings.Join(account.QualifiedModels, ", "),
		})
	}
	return View{
		Available: true, Stale: now.UTC().Sub(r.EvaluatedAt) > 20*time.Minute,
		Status: r.Status, ProposalID: r.ProposalID, EvaluatedAt: r.EvaluatedAt.UTC().Format(time.RFC3339),
		PublishedFamilies: strings.Join(r.Published.Families, ", "), PublishedModels: strings.Join(r.Published.Models, ", "),
		CandidateFamilies: strings.Join(r.Candidate.Families, ", "), CandidateModels: strings.Join(r.Candidate.Models, ", "),
		ReviewModels: strings.Join(r.Candidate.ReviewModels, ", "), AccountSetSHA256: r.AccountSetSHA256,
		BaseConfigSHA256: r.BaseConfigSHA256, Groups: groups, Accounts: accounts, Blockers: append([]string(nil), r.Blockers...),
	}
}

func validate(result Result, now time.Time) error {
	if result.SchemaVersion != 1 || strings.TrimSpace(result.SnapshotID) == "" ||
		!lowercaseSHA256.MatchString(result.ProposalID) || !lowercaseSHA256.MatchString(result.AccountSetSHA256) ||
		!lowercaseSHA256.MatchString(result.BaseConfigSHA256) || result.EvaluatedAt.IsZero() || result.EvaluatedAt.After(now) ||
		result.Evidence.CapturedAt.IsZero() || result.Evidence.CapturedAt.After(now) ||
		!allowedStatus(result.Status) || result.Evidence.FreshnessMinutes != 20 || result.Evidence.BalanceMinUSD != 5 ||
		result.Evidence.QualitySamplesMin != 20 {
		return fmt.Errorf("metadata is invalid")
	}
	if !validUniqueStrings(result.Published.Families, true, false) || !validUniqueStrings(result.Published.Models, true, true) ||
		!validUniqueStrings(result.Candidate.Families, true, false) || !validUniqueStrings(result.Candidate.Models, true, true) ||
		!validUniqueStrings(result.Candidate.ReviewModels, true, true) || !validUniqueStrings(result.Blockers, true, false) {
		return fmt.Errorf("model lists are invalid")
	}
	if result.Status == "可升级" && (len(result.Candidate.Models) == 0 || len(result.Blockers) != 0) {
		return fmt.Errorf("ready result is incomplete")
	}
	if result.Candidate.Family != nil && strings.TrimSpace(*result.Candidate.Family) == "" {
		return fmt.Errorf("candidate family is invalid")
	}
	previousGroup := int64(0)
	for _, group := range result.Groups {
		if group.GroupID <= previousGroup || strings.TrimSpace(group.Name) == "" ||
			!validUniqueStrings(group.CoveredModels, true, true) || !validUniqueStrings(group.MissingModels, true, true) ||
			(group.Covered && len(group.MissingModels) != 0) {
			return fmt.Errorf("group is invalid")
		}
		previousGroup = group.GroupID
	}
	previousAccount := int64(0)
	for _, account := range result.Accounts {
		if account.AccountID <= previousAccount || !validPositiveIDs(account.GroupIDs) ||
			!validUniqueStrings(account.QualifiedModels, true, true) {
			return fmt.Errorf("account is invalid")
		}
		previousAccount = account.AccountID
	}
	return nil
}

func allowedStatus(value string) bool {
	switch value {
	case "未发现更新", "待确认", "待测试", "测试未通过", "可升级", "已发布":
		return true
	default:
		return false
	}
}

func validUniqueStrings(values []string, allowEmpty bool, models bool) bool {
	if !allowEmpty && len(values) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" || (models && !modelIDPattern.MatchString(value)) {
			return false
		}
		if _, duplicate := seen[value]; duplicate {
			return false
		}
		seen[value] = struct{}{}
	}
	return sort.StringsAreSorted(values)
}

func validPositiveIDs(values []int64) bool {
	if len(values) == 0 {
		return false
	}
	previous := int64(0)
	for _, value := range values {
		if value <= previous {
			return false
		}
		previous = value
	}
	return true
}

func decodeGeneric(data []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil || ensureEOF(decoder) != nil {
		return nil, fmt.Errorf("invalid JSON")
	}
	return document, nil
}

func proposalHashMatches(document map[string]any) bool {
	provided, ok := document["proposal_id"].(string)
	if !ok || !lowercaseSHA256.MatchString(provided) {
		return false
	}
	delete(document, "proposal_id")
	canonical, err := json.Marshal(document)
	if err != nil {
		return false
	}
	digest := sha256.Sum256(canonical)
	expected := hex.EncodeToString(digest[:])
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func containsForbiddenKey(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if forbiddenKey.MatchString(key) || containsForbiddenKey(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsForbiddenKey(child) {
				return true
			}
		}
	}
	return false
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}
