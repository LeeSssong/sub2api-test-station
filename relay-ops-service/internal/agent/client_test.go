package agent

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentAnalyzesVersionedContractAndValidatesOutput(t *testing.T) {
	t.Parallel()
	keyFile := writeAgentKey(t, "agent-secret-key")
	var captured string
	client := Client{BaseURL: "https://agent.example/v1", APIKeyFile: keyFile, Model: "ops-model", HTTP: &http.Client{Transport: agentTransport(func(request *http.Request) *http.Response {
		data, _ := io.ReadAll(request.Body)
		captured = string(data)
		return agentResponse(http.StatusOK, `{"choices":[{"message":{"content":"{\"summary\":\"倍率上升\",\"what_was_done\":[\"读取价格页\"],\"result\":[\"0.10x\"],\"change\":\"从0.07x上升\",\"focus\":\"人工核对\",\"hypotheses\":[],\"recommended_action\":\"request_human_review\",\"requires_human_approval\":true,\"confidence\":0.9}"}}]}`)
	})}}
	analysis, err := client.Analyze(context.Background(), validContract())
	if err != nil || analysis.Summary != "倍率上升" || !analysis.RequiresHumanApproval {
		t.Fatalf("analysis=%#v err=%v", analysis, err)
	}
	if strings.Contains(captured, "agent-secret-key") || !strings.Contains(captured, "relay-ops-incident-v1") {
		t.Fatalf("captured request=%s", captured)
	}
}

func TestAgentRejectsSecretsPIIAndInvalidOutput(t *testing.T) {
	t.Parallel()
	contract := validContract()
	contract.CurrentValue = "Bearer sk-upstream-secret"
	if _, err := (Client{}).Analyze(context.Background(), contract); !IsUnsafeContract(err) {
		t.Fatalf("unsafe error=%v", err)
	}
	contract = validContract()
	contract.Upstream = "owner@example.com"
	if _, err := (Client{}).Analyze(context.Background(), contract); !IsUnsafeContract(err) {
		t.Fatalf("PII error=%v", err)
	}
	if _, err := ValidateAgentOutput([]byte(`{"summary":"missing fields"}`)); err == nil {
		t.Fatal("invalid output accepted")
	}
}

func TestDeterministicFallbackNeedsNoModel(t *testing.T) {
	t.Parallel()
	analysis := Fallback(validContract())
	if analysis.Summary == "" || analysis.RecommendedAction != "observe" || analysis.Confidence != 0 {
		t.Fatalf("fallback=%#v", analysis)
	}
}

func TestServiceAnalyzesEachEventOnceAndPersistsFallback(t *testing.T) {
	t.Parallel()
	repository := &memoryAnalysisRepository{}
	analyzer := &fakeAnalyzer{err: context.DeadlineExceeded}
	service := Service{Analyzer: analyzer, Repository: repository}
	first, err := service.AnalyzeOnce(context.Background(), validContract())
	if err != nil || first.Summary == "" || analyzer.calls != 1 {
		t.Fatalf("first=%#v calls=%d err=%v", first, analyzer.calls, err)
	}
	second, err := service.AnalyzeOnce(context.Background(), validContract())
	if err != nil || second.Summary != first.Summary || analyzer.calls != 1 {
		t.Fatalf("second=%#v calls=%d err=%v", second, analyzer.calls, err)
	}
}

func validContract() IncidentContractV1 {
	return IncidentContractV1{ContractVersion: "relay-ops-incident-v1", IncidentID: "inc-1", Severity: "P1", Upstream: "Neko", PublicGroup: "GPT-Pro", Model: "gpt-5.6-sol", MetricName: "multiplier_bps", BaselineValue: "700", CurrentValue: "1000", Samples: 2, EvidenceRefs: []string{"pricing_snapshot:1"}, AllowedActions: []string{"observe", "request_human_review"}}
}

func writeAgentKey(t *testing.T, value string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent-key")
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

type agentTransport func(*http.Request) *http.Response

func (fn agentTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request), nil
}
func agentResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

type fakeAnalyzer struct {
	calls int
	err   error
}

func (a *fakeAnalyzer) Analyze(context.Context, IncidentContractV1) (Analysis, error) {
	a.calls++
	return Analysis{}, a.err
}

type memoryAnalysisRepository struct {
	value  Analysis
	exists bool
}

func (r *memoryAnalysisRepository) Find(context.Context, string) (Analysis, bool, error) {
	return r.value, r.exists, nil
}
func (r *memoryAnalysisRepository) Save(_ context.Context, _ string, value Analysis, _ bool) error {
	r.value = value
	r.exists = true
	return nil
}
