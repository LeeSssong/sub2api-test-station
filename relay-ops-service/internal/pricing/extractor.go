package pricing

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/domain"
	"github.com/PuerkitoBio/goquery"
)

var errUnparseable = errors.New("pricing evidence is unparseable")

const EvidenceSchemaVersion = "pricing-evidence-v2"

var multiplierPattern = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*(?:x|倍)`)
var labeledMultiplierPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:倍率|multiplier|(?:cost\s+)?rate)\s*[:：=]?\s*([0-9]+(?:\.[0-9]+)?)\s*(?:x|倍)`),
	regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*(?:x|倍)\s*(?:倍率|multiplier|(?:cost\s+)?rate)`),
}
var decimalPattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)

type ModelPrice struct {
	ModelID    string `json:"model_id"`
	Tier       string `json:"tier,omitempty"`
	Input      string `json:"input,omitempty"`
	Output     string `json:"output,omitempty"`
	CacheRead  string `json:"cache_read,omitempty"`
	CacheWrite string `json:"cache_write,omitempty"`
}

type Evidence struct {
	Models               []ModelPrice          `json:"models"`
	AdvertisedMultiplier *domain.MultiplierBPS `json:"advertised_multiplier_bps,omitempty"`
	SourceURL            string                `json:"source_url"`
	Confidence           string                `json:"confidence"`
	SchemaVersion        string                `json:"schema_version"`
}

type Extractor interface {
	Match(FetchResult) bool
	Extract(FetchResult) (Evidence, error)
}

type CompositeExtractor struct{}

func (CompositeExtractor) Match(FetchResult) bool { return true }

func (CompositeExtractor) Extract(result FetchResult) (Evidence, error) {
	contentType := strings.ToLower(result.ContentType)
	trimmed := bytes.TrimSpace(result.Body)
	if strings.Contains(contentType, "json") || (len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')) {
		if evidence, err := extractJSON(result); err == nil {
			return evidence, nil
		}
	}
	return extractHTML(result)
}

func IsUnparseable(err error) bool { return errors.Is(err, errUnparseable) }

func extractJSON(result FetchResult) (Evidence, error) {
	decoder := json.NewDecoder(bytes.NewReader(result.Body))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return Evidence{}, errUnparseable
	}
	evidence := Evidence{SourceURL: result.URL, Confidence: "structured_json", SchemaVersion: EvidenceSchemaVersion}
	evidence.AdvertisedMultiplier = findMultiplier(document)
	evidence.Models = findJSONModels(document)
	if evidence.AdvertisedMultiplier == nil && len(evidence.Models) == 0 {
		return Evidence{}, errUnparseable
	}
	sortModelPrices(evidence.Models)
	return evidence, nil
}

func findMultiplier(value any) *domain.MultiplierBPS {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "multiplier") || lower == "rate" || strings.Contains(lower, "倍率") {
				if parsed := parseMultiplier(fmt.Sprint(child)); parsed != nil {
					return parsed
				}
			}
		}
		for _, child := range current {
			if parsed := findMultiplier(child); parsed != nil {
				return parsed
			}
		}
	case []any:
		for _, child := range current {
			if parsed := findMultiplier(child); parsed != nil {
				return parsed
			}
		}
	}
	return nil
}

func parseMultiplier(raw string) *domain.MultiplierBPS {
	raw = strings.TrimSpace(raw)
	match := multiplierPattern.FindStringSubmatch(raw)
	value := raw
	if len(match) == 2 {
		value = match[1]
	}
	parsed, err := domain.ParseMultiplierBPS(value)
	if err != nil || parsed <= 0 {
		return nil
	}
	return &parsed
}

func findJSONModels(document any) []ModelPrice {
	object, ok := document.(map[string]any)
	if !ok {
		return nil
	}
	for key, value := range object {
		if strings.EqualFold(key, "models") || strings.EqualFold(key, "model_pricing") {
			if models := parseJSONModelCollection(value); len(models) > 0 {
				return models
			}
		}
	}
	for _, value := range object {
		if models := findJSONModels(value); len(models) > 0 {
			return models
		}
	}
	return nil
}

func parseJSONModelCollection(value any) []ModelPrice {
	result := make([]ModelPrice, 0)
	switch collection := value.(type) {
	case []any:
		for _, item := range collection {
			if object, ok := item.(map[string]any); ok {
				if model, ok := modelFromMap("", object); ok {
					result = append(result, model)
				}
			}
		}
	case map[string]any:
		for id, item := range collection {
			if object, ok := item.(map[string]any); ok {
				if model, ok := modelFromMap(id, object); ok {
					result = append(result, model)
				}
			}
		}
	}
	return result
}

func modelFromMap(fallbackID string, object map[string]any) (ModelPrice, bool) {
	model := ModelPrice{ModelID: fallbackID}
	for key, value := range object {
		normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
		switch normalized {
		case "model", "model_id", "id", "name":
			if model.ModelID == "" {
				model.ModelID = strings.TrimSpace(fmt.Sprint(value))
			}
		case "tier", "context_tier":
			model.Tier = strings.TrimSpace(fmt.Sprint(value))
		case "input", "input_price":
			model.Input = normalizeDecimal(value)
		case "output", "output_price":
			model.Output = normalizeDecimal(value)
		case "cache_read", "cache_read_price":
			model.CacheRead = normalizeDecimal(value)
		case "cache_write", "cache_write_price":
			model.CacheWrite = normalizeDecimal(value)
		}
	}
	return model, model.ModelID != "" && (model.Input != "" || model.Output != "" || model.CacheRead != "" || model.CacheWrite != "")
}

func extractHTML(result FetchResult) (Evidence, error) {
	document, err := goquery.NewDocumentFromReader(bytes.NewReader(result.Body))
	if err != nil {
		return Evidence{}, errUnparseable
	}
	document.Find("script,style,noscript,template").Remove()
	evidence := Evidence{SourceURL: result.URL, Confidence: "common_html", SchemaVersion: EvidenceSchemaVersion}
	evidence.AdvertisedMultiplier = extractHTMLMultiplier(document)
	document.Find("[data-model]").Each(func(_ int, selection *goquery.Selection) {
		model := ModelPrice{ModelID: strings.TrimSpace(selection.AttrOr("data-model", "")), Tier: strings.TrimSpace(selection.AttrOr("data-tier", ""))}
		model.Input = normalizeDecimal(selection.AttrOr("data-input-price", ""))
		model.Output = normalizeDecimal(selection.AttrOr("data-output-price", ""))
		model.CacheRead = normalizeDecimal(selection.AttrOr("data-cache-read-price", ""))
		model.CacheWrite = normalizeDecimal(selection.AttrOr("data-cache-write-price", ""))
		if model.ModelID != "" && (model.Input != "" || model.Output != "") {
			evidence.Models = append(evidence.Models, model)
		}
	})
	document.Find("table").Each(func(_ int, table *goquery.Selection) {
		headers := make(map[string]int)
		table.Find("thead th").Each(func(index int, header *goquery.Selection) {
			headers[normalizeHeader(header.Text())] = index
		})
		table.Find("tbody tr").Each(func(_ int, row *goquery.Selection) {
			cells := row.Find("td")
			model := ModelPrice{
				ModelID:    cell(cells, headerIndex(headers, "model")),
				Tier:       cell(cells, headerIndex(headers, "tier")),
				Input:      normalizeDecimal(cell(cells, headerIndex(headers, "input"))),
				Output:     normalizeDecimal(cell(cells, headerIndex(headers, "output"))),
				CacheRead:  normalizeDecimal(cell(cells, headerIndex(headers, "cache_read"))),
				CacheWrite: normalizeDecimal(cell(cells, headerIndex(headers, "cache_write"))),
			}
			if model.ModelID != "" && (model.Input != "" || model.Output != "") {
				evidence.Models = append(evidence.Models, model)
			}
		})
	})
	if evidence.AdvertisedMultiplier == nil && len(evidence.Models) == 0 {
		return Evidence{}, errUnparseable
	}
	sortModelPrices(evidence.Models)
	return evidence, nil
}

func NewUnparseableEvidence(sourceURL string) Evidence {
	return Evidence{SourceURL: sourceURL, Confidence: "unparseable", SchemaVersion: EvidenceSchemaVersion}
}

func extractHTMLMultiplier(document *goquery.Document) *domain.MultiplierBPS {
	var result *domain.MultiplierBPS
	document.Find("[data-multiplier], [data-rate]").EachWithBreak(func(_ int, selection *goquery.Selection) bool {
		for _, attribute := range []string{"data-multiplier", "data-rate"} {
			if value, exists := selection.Attr(attribute); exists {
				if parsed := parseMultiplier(value); parsed != nil {
					result = parsed
					return false
				}
			}
		}
		return true
	})
	if result != nil {
		return result
	}
	text := strings.Join(strings.Fields(document.Text()), " ")
	for _, pattern := range labeledMultiplierPatterns {
		match := pattern.FindStringSubmatch(text)
		if len(match) == 2 {
			if parsed := parseMultiplier(match[1]); parsed != nil {
				return parsed
			}
		}
	}
	return nil
}

func normalizeHeader(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(lower, "模型") || strings.Contains(lower, "model"):
		return "model"
	case strings.Contains(lower, "缓存写") || strings.Contains(lower, "cache write"):
		return "cache_write"
	case strings.Contains(lower, "缓存读") || strings.Contains(lower, "cache read"):
		return "cache_read"
	case strings.Contains(lower, "输入") || strings.Contains(lower, "input"):
		return "input"
	case strings.Contains(lower, "输出") || strings.Contains(lower, "output"):
		return "output"
	case strings.Contains(lower, "阶梯") || strings.Contains(lower, "tier"):
		return "tier"
	default:
		return lower
	}
}

func headerIndex(headers map[string]int, key string) int {
	if index, ok := headers[key]; ok {
		return index
	}
	return -1
}

func cell(cells *goquery.Selection, index int) string {
	if index < 0 || index >= cells.Length() {
		return ""
	}
	return strings.TrimSpace(cells.Eq(index).Text())
}

func normalizeDecimal(value any) string {
	raw := strings.TrimSpace(fmt.Sprint(value))
	raw = strings.TrimPrefix(raw, "$")
	raw = strings.ReplaceAll(raw, ",", "")
	if !decimalPattern.MatchString(raw) {
		return ""
	}
	return raw
}

func sortModelPrices(models []ModelPrice) {
	sort.Slice(models, func(left, right int) bool {
		return modelKey(models[left]) < modelKey(models[right])
	})
}

type MultiplierChange struct {
	Before        domain.MultiplierBPS `json:"before_bps"`
	After         domain.MultiplierBPS `json:"after_bps"`
	BeforePresent bool                 `json:"before_present"`
	AfterPresent  bool                 `json:"after_present"`
}

type PriceChange struct {
	ModelID string     `json:"model_id"`
	Tier    string     `json:"tier,omitempty"`
	Before  ModelPrice `json:"before"`
	After   ModelPrice `json:"after"`
}

type SemanticDiff struct {
	AddedModels       []string          `json:"added_models,omitempty"`
	RemovedModels     []string          `json:"removed_models,omitempty"`
	PriceChanges      []PriceChange     `json:"price_changes,omitempty"`
	Multiplier        *MultiplierChange `json:"multiplier,omitempty"`
	UnparseableFields []string          `json:"unparseable_fields,omitempty"`
}

func Diff(previous, current Evidence) SemanticDiff {
	diff := SemanticDiff{}
	beforeMultiplier, beforePresent := optionalMultiplier(previous.AdvertisedMultiplier)
	afterMultiplier, afterPresent := optionalMultiplier(current.AdvertisedMultiplier)
	if beforePresent != afterPresent || (beforePresent && beforeMultiplier != afterMultiplier) {
		diff.Multiplier = &MultiplierChange{
			Before: beforeMultiplier, After: afterMultiplier,
			BeforePresent: beforePresent, AfterPresent: afterPresent,
		}
	}
	if current.Confidence == "unparseable" && hasPricingEvidence(previous) {
		diff.UnparseableFields = append(diff.UnparseableFields, "pricing_evidence")
	}
	before := indexModels(previous.Models)
	after := indexModels(current.Models)
	for key, model := range after {
		old, exists := before[key]
		if !exists {
			diff.AddedModels = append(diff.AddedModels, model.ModelID)
		} else if old != model {
			diff.PriceChanges = append(diff.PriceChanges, PriceChange{ModelID: model.ModelID, Tier: model.Tier, Before: old, After: model})
		}
	}
	for key, model := range before {
		if _, exists := after[key]; !exists {
			diff.RemovedModels = append(diff.RemovedModels, model.ModelID)
		}
	}
	sort.Strings(diff.AddedModels)
	sort.Strings(diff.RemovedModels)
	sort.Slice(diff.PriceChanges, func(left, right int) bool {
		return modelKey(diff.PriceChanges[left].After) < modelKey(diff.PriceChanges[right].After)
	})
	return diff
}

func optionalMultiplier(value *domain.MultiplierBPS) (domain.MultiplierBPS, bool) {
	if value == nil {
		return 0, false
	}
	return *value, true
}

func hasPricingEvidence(evidence Evidence) bool {
	return evidence.AdvertisedMultiplier != nil || len(evidence.Models) > 0
}

func (diff SemanticDiff) SemanticChange() bool {
	return diff.Multiplier != nil || len(diff.AddedModels) > 0 || len(diff.RemovedModels) > 0 || len(diff.PriceChanges) > 0 || len(diff.UnparseableFields) > 0
}

func indexModels(models []ModelPrice) map[string]ModelPrice {
	result := make(map[string]ModelPrice, len(models))
	for _, model := range models {
		result[modelKey(model)] = model
	}
	return result
}

func modelKey(model ModelPrice) string { return model.ModelID + "\x00" + model.Tier }

func IsStale(observedAt, now time.Time, maximumAge time.Duration) bool {
	return maximumAge > 0 && now.Sub(observedAt) > maximumAge
}
