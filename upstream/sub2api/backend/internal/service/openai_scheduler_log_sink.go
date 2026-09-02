package service

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

const openAISchedulerLogDefaultQueueCapacity = 4096

// OpenAISchedulerLogSinkHealth is intentionally limited to completeness
// signals. The request path never waits for this best-effort sink.
type OpenAISchedulerLogSinkHealth struct {
	QueueDepth    int       `json:"queue_depth"`
	QueueCapacity int       `json:"queue_capacity"`
	DroppedCount  uint64    `json:"dropped_count"`
	LastDropAt    time.Time `json:"last_drop_at"`
	WriteFailed   uint64    `json:"write_failed_count"`
	WrittenCount  uint64    `json:"written_count"`
	LastErrorAt   time.Time `json:"last_error_at"`
}

// OpenAISchedulerLogSink owns a bounded queue of immutable scheduler event
// snapshots. Persistence is deliberately injected separately so that choosing
// an account can never block on database availability.
type OpenAISchedulerLogSink struct {
	queue             chan OpenAIResilienceEvent
	droppedCount      atomic.Uint64
	lastDropUnixNano  atomic.Int64
	writeFailed       atomic.Uint64
	writtenCount      atomic.Uint64
	lastErrorUnixNano atomic.Int64
	writer            OpenAISchedulerLogWriter
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
	startOnce         sync.Once
	stopOnce          sync.Once
}

type OpenAISchedulerLogInsert struct {
	EventAt          time.Time
	Platform         string
	GroupID          *int64
	LogicalRequestID string
	AttemptID        string
	AttemptNumber    int
	EventName        string
	AccountID        int64
	CanonicalModel   string
	Outcome          string
	FinalOutcome     string
	SelectionLayer   string
	AlgorithmVersion string
	DecisionJSON     string
}

type OpenAISchedulerLogWriter interface {
	BatchInsertOpenAISchedulerLogs(context.Context, []OpenAISchedulerLogInsert) (int, error)
}

type openAISchedulerLogCleaner interface {
	DeleteOpenAISchedulerLogsBefore(context.Context, time.Time, int) (int64, error)
}

func NewOpenAISchedulerLogSink(capacity int) *OpenAISchedulerLogSink {
	if capacity <= 0 {
		capacity = openAISchedulerLogDefaultQueueCapacity
	}
	return &OpenAISchedulerLogSink{queue: make(chan OpenAIResilienceEvent, capacity)}
}

func NewOpenAISchedulerLogSinkWithWriter(writer OpenAISchedulerLogWriter, capacity int) *OpenAISchedulerLogSink {
	sink := NewOpenAISchedulerLogSink(capacity)
	sink.writer = writer
	return sink
}

// Start owns asynchronous batch persistence. Enqueue stays non-blocking even
// while the database is unavailable.
func (s *OpenAISchedulerLogSink) Start() {
	if s == nil || s.writer == nil {
		return
	}
	s.startOnce.Do(func() {
		s.ctx, s.cancel = context.WithCancel(context.Background())
		s.wg.Add(1)
		go s.run()
	})
}

func (s *OpenAISchedulerLogSink) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.wg.Wait()
	})
}

func (s *OpenAISchedulerLogSink) run() {
	defer s.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	cleanupTicker := time.NewTicker(time.Hour)
	defer cleanupTicker.Stop()
	s.cleanupWithTimeout(context.Background())
	for {
		select {
		case <-s.ctx.Done():
			s.flushWithTimeout(context.Background())
			return
		case <-ticker.C:
			s.flushWithTimeout(s.ctx)
		case <-cleanupTicker.C:
			s.cleanupWithTimeout(s.ctx)
		}
	}
}

func (s *OpenAISchedulerLogSink) flushWithTimeout(base context.Context) {
	if base == nil || base.Err() != nil {
		base = context.Background()
	}
	ctx, cancel := context.WithTimeout(base, 5*time.Second)
	defer cancel()
	if err := s.Flush(ctx); err != nil {
		s.writeFailed.Add(1)
		s.lastErrorUnixNano.Store(time.Now().UTC().UnixNano())
	}
}

func (s *OpenAISchedulerLogSink) cleanupWithTimeout(base context.Context) {
	cleaner, ok := s.writer.(openAISchedulerLogCleaner)
	if !ok {
		return
	}
	if base == nil || base.Err() != nil {
		base = context.Background()
	}
	ctx, cancel := context.WithTimeout(base, 5*time.Second)
	defer cancel()
	if _, err := cleaner.DeleteOpenAISchedulerLogsBefore(ctx, OpenAISchedulerLogRetentionCutoff(time.Now()), 0); err != nil {
		s.writeFailed.Add(1)
		s.lastErrorUnixNano.Store(time.Now().UTC().UnixNano())
	}
}

func isOpenAISchedulerLogEvent(name string) bool {
	switch name {
	case OpenAIEventSchedulerSelection, OpenAIEventSchedulerRequestOutcome,
		OpenAIEventAccountModelSoftFailure, OpenAIEventAccountModelCooldownStarted,
		OpenAIEventAccountModelCooldownSkippedCache, OpenAIEventAccountModelPostFailureSelected,
		OpenAIEventStreamUpstreamFailure, OpenAIEventFailoverAfterStreamFailure,
		OpenAIEventResponsesFailoverDecision:
		return true
	default:
		return false
	}
}

func cloneOpenAIResilienceEvent(event OpenAIResilienceEvent) OpenAIResilienceEvent {
	if event.GroupID != nil {
		groupID := *event.GroupID
		event.GroupID = &groupID
	}
	event.CandidateAccountIDs = append([]int64(nil), event.CandidateAccountIDs...)
	event.ExcludedAccountIDs = append([]int64(nil), event.ExcludedAccountIDs...)
	if event.ExcludeReasons != nil {
		reasons := make(map[string]int, len(event.ExcludeReasons))
		for key, value := range event.ExcludeReasons {
			reasons[key] = value
		}
		event.ExcludeReasons = reasons
	}
	return event
}

// Enqueue is non-blocking. A false return means either that the event is not
// part of the scheduler decision contract or that the bounded queue is full.
func (s *OpenAISchedulerLogSink) Enqueue(event OpenAIResilienceEvent) bool {
	if s == nil || !isOpenAISchedulerLogEvent(event.Name) {
		return false
	}
	event = cloneOpenAIResilienceEvent(event)
	select {
	case s.queue <- event:
		return true
	default:
		s.droppedCount.Add(1)
		s.lastDropUnixNano.Store(time.Now().UTC().UnixNano())
		return false
	}
}

func (s *OpenAISchedulerLogSink) Health() OpenAISchedulerLogSinkHealth {
	if s == nil {
		return OpenAISchedulerLogSinkHealth{}
	}
	health := OpenAISchedulerLogSinkHealth{
		QueueDepth: len(s.queue), QueueCapacity: cap(s.queue), DroppedCount: s.droppedCount.Load(),
		WriteFailed: s.writeFailed.Load(), WrittenCount: s.writtenCount.Load(),
	}
	if unixNano := s.lastDropUnixNano.Load(); unixNano > 0 {
		health.LastDropAt = time.Unix(0, unixNano).UTC()
	}
	if unixNano := s.lastErrorUnixNano.Load(); unixNano > 0 {
		health.LastErrorAt = time.Unix(0, unixNano).UTC()
	}
	return health
}

// Flush persists the currently queued batch. It is invoked only by the
// background writer (and tests), never by the request path.
func (s *OpenAISchedulerLogSink) Flush(ctx context.Context) error {
	if s == nil || s.writer == nil {
		return nil
	}
	inputs := make([]OpenAISchedulerLogInsert, 0, len(s.queue))
	for {
		select {
		case event := <-s.queue:
			if event.CorrelationID == "" {
				continue
			}
			decision, err := json.Marshal(map[string]any{
				"candidate_account_ids": event.CandidateAccountIDs,
				"excluded_account_ids":  event.ExcludedAccountIDs,
				"exclude_reasons":       event.ExcludeReasons,
				"candidate_count":       event.CandidateCount, "eligible_count": event.EligibleCount,
				"effective_top_k": event.EffectiveTopK, "selected_rank": event.SelectedRank,
				"quality_score": event.QualityScore, "quality_score_gap": event.QualityScoreGap,
				"retry_budget_exhausted": event.RetryBudgetExhausted, "extra_used": event.ExtraUsed,
				"switch_count": event.SwitchCount, "safe_to_replay": event.SafeToReplay,
				"switch_reason": event.SwitchReason, "switch_block_reason": event.SwitchBlockReason,
				"stop_reason": event.StopReason, "status_code": event.StatusCode,
			})
			if err != nil {
				continue
			}
			at := event.At.UTC()
			if at.IsZero() {
				at = time.Now().UTC()
			}
			platform := event.Platform
			if platform == "" {
				platform = PlatformOpenAI
			}
			inputs = append(inputs, OpenAISchedulerLogInsert{
				EventAt: at, Platform: platform, GroupID: event.GroupID, LogicalRequestID: event.CorrelationID,
				AttemptID: event.AttemptID, AttemptNumber: event.AttemptNumber, EventName: event.Name,
				AccountID: event.AccountID, CanonicalModel: event.CanonicalModel, Outcome: event.Outcome,
				FinalOutcome: event.FinalOutcome, SelectionLayer: event.SelectionLayer,
				AlgorithmVersion: OpenAISchedulerAlgorithmVersion, DecisionJSON: string(decision),
			})
		default:
			if len(inputs) == 0 {
				return nil
			}
			inserted, err := s.writer.BatchInsertOpenAISchedulerLogs(ctx, inputs)
			if err == nil {
				s.writtenCount.Add(uint64(inserted))
			}
			return err
		}
	}
}

func OpenAISchedulerLogRetentionCutoff(now time.Time) time.Time {
	return now.UTC().Add(-7 * 24 * time.Hour)
}

var defaultOpenAISchedulerLogSink = NewOpenAISchedulerLogSink(openAISchedulerLogDefaultQueueCapacity)

// ConfigureDefaultOpenAISchedulerLogSink replaces the unstarted in-memory
// sink during application wiring. It is called once during startup only.
func ConfigureDefaultOpenAISchedulerLogSink(writer OpenAISchedulerLogWriter) *OpenAISchedulerLogSink {
	sink := NewOpenAISchedulerLogSinkWithWriter(writer, openAISchedulerLogDefaultQueueCapacity)
	sink.Start()
	defaultOpenAISchedulerLogSink = sink
	return sink
}

func DefaultOpenAISchedulerLogSinkHealth() OpenAISchedulerLogSinkHealth {
	return defaultOpenAISchedulerLogSink.Health()
}
