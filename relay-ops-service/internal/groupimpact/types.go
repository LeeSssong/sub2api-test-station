package groupimpact

import "time"

type RuntimeEvidence struct {
	Requests          int64
	Successes         int64
	ErrorRate         float64
	TTFTP95MS         float64
	TTFTBaselineP95MS float64
	AffectedScope     []string
}

type UnavailableAccount struct {
	Name   string
	Reason string
}

type CapacityEvidence struct {
	Available   int
	Total       int
	Unavailable []UnavailableAccount
	ObservedAt  time.Time
}

type NativeMonitorEvidence struct {
	Model      string
	Status     string
	ObservedAt time.Time
}

type Snapshot struct {
	GroupID       int64
	GroupName     string
	ObservedAt    time.Time
	Runtime       *RuntimeEvidence
	Capacity      *CapacityEvidence
	NativeMonitor []NativeMonitorEvidence
}

type Fact struct {
	Label     string
	Value     string
	Confirmed bool
}

type Impact struct {
	Failing      bool
	Severity     string
	Primary      string
	EvidenceHash string
	MaterialHash string
	Headline     string
	Summary      string
	UserImpact   string
	Current      []Fact
	Clues        []Fact
	Action       string
	ObservedAt   time.Time
}

type ReminderSnapshot struct {
	GroupName  string    `json:"group_name"`
	Headline   string    `json:"headline"`
	LatestFact string    `json:"latest_fact"`
	Capacity   string    `json:"capacity,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
}
