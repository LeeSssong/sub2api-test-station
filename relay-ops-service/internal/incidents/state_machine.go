package incidents

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrOccurrenceConflict = errors.New("incident occurrence conflicts with current state")
	ErrNotActive          = errors.New("incident is not active")
)

type Acknowledgement struct {
	Key          string
	OccurrenceNo int64
	ActorUserID  int64
	At           time.Time
}

type Observation struct {
	Key                 string
	Severity            string
	Failing             bool
	EvidenceHash        string
	CurrentValue        string
	ConfirmationWindows int
}

type Record struct {
	Key          string
	Severity     string
	State        string
	SampleCount  int
	OccurrenceNo int64
	EvidenceHash string
	CurrentValue string
}

type Repository interface {
	Get(context.Context, string) (Record, bool, error)
	Put(context.Context, Record) error
}

type Policy struct {
	ConfirmWindows map[string]int
}

func DefaultPolicy() Policy {
	return Policy{ConfirmWindows: map[string]int{"P0": 1, "P1": 2, "P2": 3}}
}

type Machine struct {
	Repository Repository
	Policy     Policy
}

type Transition struct {
	State        string
	Kind         string
	Notify       bool
	RelatedKey   string
	OccurrenceNo int64
}

func (m Machine) Observe(ctx context.Context, observation Observation) (Transition, error) {
	if m.Repository == nil || observation.Key == "" {
		return Transition{}, fmt.Errorf("incident repository and key are required")
	}
	required, ok := m.Policy.ConfirmWindows[observation.Severity]
	if !ok {
		return Transition{}, fmt.Errorf("incident severity is invalid")
	}
	if observation.ConfirmationWindows > 0 {
		required = observation.ConfirmationWindows
	}
	record, exists, err := m.Repository.Get(ctx, observation.Key)
	if err != nil {
		return Transition{}, err
	}
	if exists && record.OccurrenceNo <= 0 {
		record.OccurrenceNo = 1
	}
	if !observation.Failing {
		if !exists || record.State == "recovered" {
			return Transition{State: "healthy", OccurrenceNo: record.OccurrenceNo}, nil
		}
		notify := record.State == "confirmed" || record.State == "escalated" || record.State == "degraded"
		record.State = "recovered"
		record.CurrentValue = observation.CurrentValue
		record.EvidenceHash = observation.EvidenceHash
		if err := m.Repository.Put(ctx, record); err != nil {
			return Transition{}, err
		}
		return Transition{State: "recovered", Kind: "recovered", Notify: notify, RelatedKey: observation.Key, OccurrenceNo: record.OccurrenceNo}, nil
	}

	if !exists || record.State == "recovered" {
		occurrenceNo := int64(1)
		if exists {
			occurrenceNo = record.OccurrenceNo + 1
		}
		record = Record{Key: observation.Key, Severity: observation.Severity, State: "observed", SampleCount: 1, OccurrenceNo: occurrenceNo, EvidenceHash: observation.EvidenceHash, CurrentValue: observation.CurrentValue}
		if required == 1 {
			record.State = "confirmed"
		}
		if err := m.Repository.Put(ctx, record); err != nil {
			return Transition{}, err
		}
		if record.State == "confirmed" {
			return Transition{State: record.State, Kind: "confirmed", Notify: true, RelatedKey: observation.Key, OccurrenceNo: record.OccurrenceNo}, nil
		}
		return Transition{State: record.State, OccurrenceNo: record.OccurrenceNo}, nil
	}

	previousEvidence := record.EvidenceHash
	previousSeverity := record.Severity
	record.SampleCount++
	record.CurrentValue = observation.CurrentValue
	record.EvidenceHash = observation.EvidenceHash
	if severityRank(observation.Severity) < severityRank(previousSeverity) {
		record.Severity = observation.Severity
		record.State = "escalated"
		if err := m.Repository.Put(ctx, record); err != nil {
			return Transition{}, err
		}
		return Transition{State: record.State, Kind: "escalated", Notify: true, RelatedKey: observation.Key, OccurrenceNo: record.OccurrenceNo}, nil
	}
	if record.State == "observed" && record.SampleCount >= required {
		record.State = "confirmed"
		if err := m.Repository.Put(ctx, record); err != nil {
			return Transition{}, err
		}
		return Transition{State: record.State, Kind: "confirmed", Notify: true, RelatedKey: observation.Key, OccurrenceNo: record.OccurrenceNo}, nil
	}
	if err := m.Repository.Put(ctx, record); err != nil {
		return Transition{}, err
	}
	if (record.State == "confirmed" || record.State == "escalated") && observation.EvidenceHash != "" && observation.EvidenceHash != previousEvidence {
		return Transition{State: record.State, Kind: "new_evidence", Notify: true, RelatedKey: observation.Key, OccurrenceNo: record.OccurrenceNo}, nil
	}
	return Transition{State: record.State, OccurrenceNo: record.OccurrenceNo}, nil
}

func severityRank(severity string) int {
	switch severity {
	case "P0":
		return 0
	case "P1":
		return 1
	case "P2":
		return 2
	default:
		return 99
	}
}
