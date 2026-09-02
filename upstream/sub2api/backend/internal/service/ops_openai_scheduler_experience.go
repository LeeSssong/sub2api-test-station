package service

import (
	"context"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	OpsSchedulerMetricStatusOK               = "ok"
	OpsSchedulerMetricStatusNoData           = "no_data"
	OpsSchedulerMetricStatusInsufficientData = "insufficient_data"
)

type OpsSchedulerRateMetric struct {
	Numerator   int64    `json:"numerator"`
	Denominator int64    `json:"denominator"`
	Value       *float64 `json:"value"`
	Status      string   `json:"status"`
}

type OpsSchedulerAttemptsMetric struct {
	SampleSize int64    `json:"sample_size"`
	Value      *float64 `json:"value"`
	P95        *int     `json:"p95"`
	Status     string   `json:"status"`
}

type OpsOpenAISchedulerExperienceMetrics struct {
	AutoRecoveryRate         OpsSchedulerRateMetric     `json:"auto_recovery_rate"`
	AverageAttempts          OpsSchedulerAttemptsMetric `json:"average_attempts"`
	RepeatedBadAccountRate   OpsSchedulerRateMetric     `json:"repeated_bad_account_rate"`
	RetryBudgetExhaustedRate OpsSchedulerRateMetric     `json:"retry_budget_exhausted_rate"`
	StickyKeptRate           OpsSchedulerRateMetric     `json:"sticky_kept_rate"`
	StickyEscapeRate         OpsSchedulerRateMetric     `json:"sticky_escape_rate"`
	TopKFilteredRate         OpsSchedulerRateMetric     `json:"top_k_filtered_rate"`
	TTFTReportEligibleRate   OpsSchedulerRateMetric     `json:"ttft_report_eligible_rate"`
	FirstOutputSlowCount     int64                      `json:"first_output_slow_count"`
}

type OpsOpenAISchedulerExperienceResponse struct {
	StartTime     time.Time                           `json:"start_time"`
	EndTime       time.Time                           `json:"end_time"`
	GeneratedAt   time.Time                           `json:"generated_at"`
	LatestEventAt *time.Time                          `json:"latest_event_at"`
	SampleSize    int64                               `json:"sample_size"`
	Metrics       OpsOpenAISchedulerExperienceMetrics `json:"metrics"`
}

type opsSchedulerLogicalRequest struct {
	attempts             int
	hadFailure           bool
	finalOutcome         string
	retryBudgetExhausted bool
	failedAccountModels  map[string]struct{}
}

func (s *OpsService) GetOpenAISchedulerExperience(ctx context.Context, filter *OpsDashboardFilter) (*OpsOpenAISchedulerExperienceResponse, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if filter == nil {
		return nil, infraerrors.BadRequest("OPS_FILTER_REQUIRED", "filter is required")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_REQUIRED", "start_time/end_time are required")
	}
	if filter.StartTime.After(filter.EndTime) {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_INVALID", "start_time must be <= end_time")
	}
	if filter.GroupID != nil && *filter.GroupID <= 0 {
		return nil, infraerrors.BadRequest("OPS_GROUP_ID_INVALID", "group_id must be > 0")
	}

	events := openAIResilienceEventsForWindow(
		filter.StartTime,
		filter.EndTime,
		strings.TrimSpace(filter.Platform),
		filter.GroupID,
	)
	return aggregateOpenAISchedulerExperience(events, filter.StartTime, filter.EndTime), nil
}

func aggregateOpenAISchedulerExperience(events []OpenAIResilienceEvent, start, end time.Time) *OpsOpenAISchedulerExperienceResponse {
	response := &OpsOpenAISchedulerExperienceResponse{
		StartTime:   start,
		EndTime:     end,
		GeneratedAt: time.Now().UTC(),
	}

	requests := make(map[string]*opsSchedulerLogicalRequest)
	var repeatedBadNumerator, repeatedBadDenominator int64
	var stickyKept, stickyEscaped, stickyDenominator int64
	var topKFiltered, topKEligible int64
	var ttftEligible, ttftDenominator int64
	var firstOutputSlowCount int64

	for _, event := range events {
		if response.LatestEventAt == nil || event.At.After(*response.LatestEventAt) {
			at := event.At
			response.LatestEventAt = &at
		}

		correlationID := strings.TrimSpace(event.CorrelationID)
		if correlationID == "" {
			continue
		}
		request := requests[correlationID]
		if request == nil {
			request = &opsSchedulerLogicalRequest{failedAccountModels: make(map[string]struct{})}
			requests[correlationID] = request
		}

		switch event.Name {
		case OpenAIEventFirstOutputSlow:
			firstOutputSlowCount++
		case OpenAIEventSchedulerSelection:
			request.attempts++
			if request.hadFailure && event.SelectionLayer != "half_open_probe" {
				repeatedBadDenominator++
				if _, failed := request.failedAccountModels[opsSchedulerAccountModelKey(event.AccountID, event.CanonicalModel)]; failed {
					repeatedBadNumerator++
				}
			}
			if event.StickyKept {
				stickyKept++
				stickyDenominator++
			} else if reason := strings.TrimSpace(event.StickyEscapeReason); reason != "" && reason != "none" {
				stickyEscaped++
				stickyDenominator++
			}
			if event.EligibleCount > 0 {
				topKEligible += int64(event.EligibleCount)
				if filtered := event.EligibleCount - event.EffectiveTopK; filtered > 0 {
					topKFiltered += int64(filtered)
				}
			}
			if !event.OutputStarted {
				ttftDenominator++
				if event.TTFTReportEligible {
					ttftEligible++
				}
			}
		case OpenAIEventAccountModelSoftFailure, OpenAIEventStreamUpstreamFailure:
			request.hadFailure = true
			request.failedAccountModels[opsSchedulerAccountModelKey(event.AccountID, event.CanonicalModel)] = struct{}{}
		case OpenAIEventSchedulerRequestOutcome:
			request.finalOutcome = strings.TrimSpace(event.FinalOutcome)
			request.retryBudgetExhausted = event.RetryBudgetExhausted
		}
	}

	attempts := make([]int, 0, len(requests))
	var autoRecoveryNumerator, autoRecoveryDenominator int64
	var retryBudgetNumerator, retryBudgetDenominator int64
	for _, request := range requests {
		if request.attempts == 0 {
			continue
		}
		attempts = append(attempts, request.attempts)
		if request.hadFailure {
			autoRecoveryDenominator++
			if request.finalOutcome == "success" {
				autoRecoveryNumerator++
			}
		}
		if request.attempts > 1 {
			retryBudgetDenominator++
			if request.retryBudgetExhausted {
				retryBudgetNumerator++
			}
		}
	}

	response.SampleSize = int64(len(attempts))
	response.Metrics = OpsOpenAISchedulerExperienceMetrics{
		AutoRecoveryRate:         newOpsSchedulerRateMetric(autoRecoveryNumerator, autoRecoveryDenominator),
		AverageAttempts:          newOpsSchedulerAttemptsMetric(attempts),
		RepeatedBadAccountRate:   newOpsSchedulerRateMetric(repeatedBadNumerator, repeatedBadDenominator),
		RetryBudgetExhaustedRate: newOpsSchedulerRateMetric(retryBudgetNumerator, retryBudgetDenominator),
		StickyKeptRate:           newOpsSchedulerRateMetric(stickyKept, stickyDenominator),
		StickyEscapeRate:         newOpsSchedulerRateMetric(stickyEscaped, stickyDenominator),
		TopKFilteredRate:         newOpsSchedulerRateMetric(topKFiltered, topKEligible),
		TTFTReportEligibleRate:   newOpsSchedulerRateMetric(ttftEligible, ttftDenominator),
		FirstOutputSlowCount:     firstOutputSlowCount,
	}
	return response
}

func opsSchedulerAccountModelKey(accountID int64, model string) string {
	return strings.Join([]string{strconv.FormatInt(accountID, 10), normalizeOpenAIAccountModelTransientModel(model)}, "\x00")
}

func newOpsSchedulerRateMetric(numerator, denominator int64) OpsSchedulerRateMetric {
	metric := OpsSchedulerRateMetric{Numerator: numerator, Denominator: denominator}
	switch {
	case denominator == 0:
		metric.Status = OpsSchedulerMetricStatusNoData
	case denominator < 5:
		metric.Status = OpsSchedulerMetricStatusInsufficientData
	default:
		value := float64(numerator) / float64(denominator)
		metric.Value = &value
		metric.Status = OpsSchedulerMetricStatusOK
	}
	return metric
}

func newOpsSchedulerAttemptsMetric(attempts []int) OpsSchedulerAttemptsMetric {
	metric := OpsSchedulerAttemptsMetric{SampleSize: int64(len(attempts))}
	if len(attempts) == 0 {
		metric.Status = OpsSchedulerMetricStatusNoData
		return metric
	}

	sorted := append([]int(nil), attempts...)
	sort.Ints(sorted)
	var total int
	for _, attemptCount := range sorted {
		total += attemptCount
	}
	average := float64(total) / float64(len(sorted))
	p95 := sorted[int(math.Ceil(0.95*float64(len(sorted))))-1]
	metric.Value = &average
	metric.P95 = &p95
	metric.Status = OpsSchedulerMetricStatusOK
	return metric
}
