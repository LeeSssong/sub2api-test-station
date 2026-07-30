package incidents

import (
	"context"
	"fmt"
)

type Observation struct {
	Key                 string
	Family              string
	PolicyVersion       int
	SourceKind          string
	Severity            string
	Failing             bool
	EvidenceHash        string
	MaterialHash        string
	CurrentValue        string
	LatestPayload       []byte
	ConfirmationWindows int
	RecoveryWindows     int
}

type Record struct {
	Key           string
	Family        string
	PolicyVersion int
	SourceKind    string
	Severity      string
	State         string
	SampleCount   int
	OccurrenceNo  int64
	RecoveryCount int
	EvidenceHash  string
	MaterialHash  string
	CurrentValue  string
	LatestPayload []byte
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
		if !exists || record.State == "recovered" || record.State == "closed" {
			return Transition{State: "healthy", OccurrenceNo: record.OccurrenceNo}, nil
		}
		recoveryWindows := observation.RecoveryWindows
		if recoveryWindows <= 0 {
			recoveryWindows = 1
		}
		notify := record.State == "confirmed" || record.State == "escalated" || record.State == "degraded"
		record.RecoveryCount++
		record.CurrentValue = observation.CurrentValue
		record.EvidenceHash = observation.EvidenceHash
		record.LatestPayload = append([]byte(nil), observation.LatestPayload...)
		applyObservationMetadata(&record, observation)
		if record.RecoveryCount < recoveryWindows {
			if err := m.Repository.Put(ctx, record); err != nil {
				return Transition{}, err
			}
			return Transition{State: record.State, OccurrenceNo: record.OccurrenceNo}, nil
		}
		record.State = "recovered"
		record.MaterialHash = observation.MaterialHash
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
		record = Record{
			Key: observation.Key, Family: observation.Family, PolicyVersion: observation.PolicyVersion,
			SourceKind: observation.SourceKind, Severity: observation.Severity, State: "observed",
			SampleCount: 1, OccurrenceNo: occurrenceNo, EvidenceHash: observation.EvidenceHash,
			MaterialHash: observation.MaterialHash, CurrentValue: observation.CurrentValue,
			LatestPayload: append([]byte(nil), observation.LatestPayload...),
		}
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

	if record.State == "suppressed" {
		record.State = "confirmed"
		record.RecoveryCount = 0
		record.SampleCount++
		record.CurrentValue = observation.CurrentValue
		record.EvidenceHash = observation.EvidenceHash
		record.MaterialHash = observation.MaterialHash
		record.LatestPayload = append([]byte(nil), observation.LatestPayload...)
		applyObservationMetadata(&record, observation)
		if err := m.Repository.Put(ctx, record); err != nil {
			return Transition{}, err
		}
		return Transition{State: "confirmed", Kind: "resumed", Notify: true, RelatedKey: observation.Key, OccurrenceNo: record.OccurrenceNo}, nil
	}

	previousMaterial := record.MaterialHash
	previousSeverity := record.Severity
	record.SampleCount++
	record.RecoveryCount = 0
	record.CurrentValue = observation.CurrentValue
	record.EvidenceHash = observation.EvidenceHash
	record.MaterialHash = observation.MaterialHash
	record.LatestPayload = append([]byte(nil), observation.LatestPayload...)
	applyObservationMetadata(&record, observation)
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
	if (record.State == "confirmed" || record.State == "escalated" || record.State == "degraded") &&
		observation.MaterialHash != "" && observation.MaterialHash != previousMaterial {
		return Transition{State: record.State, Kind: "progressed", Notify: true, RelatedKey: observation.Key, OccurrenceNo: record.OccurrenceNo}, nil
	}
	return Transition{State: record.State, OccurrenceNo: record.OccurrenceNo}, nil
}

func (m Machine) Suppress(ctx context.Context, key string) error {
	if m.Repository == nil || key == "" {
		return fmt.Errorf("incident repository and key are required")
	}
	record, exists, err := m.Repository.Get(ctx, key)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("incident not found")
	}
	if record.State == "recovered" || record.State == "closed" {
		return nil
	}
	record.State = "suppressed"
	record.RecoveryCount = 0
	return m.Repository.Put(ctx, record)
}

func (m Machine) Close(ctx context.Context, key, reason string) (Transition, error) {
	if m.Repository == nil || key == "" {
		return Transition{}, fmt.Errorf("incident repository and key are required")
	}
	if reason != "resolved" && reason != "manual_resolved" {
		return Transition{}, fmt.Errorf("incident close reason is invalid")
	}
	record, exists, err := m.Repository.Get(ctx, key)
	if err != nil {
		return Transition{}, err
	}
	if !exists {
		return Transition{}, fmt.Errorf("incident not found")
	}
	if record.State == "recovered" || record.State == "closed" {
		return Transition{State: record.State, OccurrenceNo: record.OccurrenceNo}, nil
	}
	notify := record.State == "confirmed" || record.State == "escalated" || record.State == "degraded"
	state, kind := "recovered", "recovered"
	if reason == "manual_resolved" {
		state, kind = "closed", "manual_resolved"
	}
	record.State = state
	record.RecoveryCount = 0
	if err := m.Repository.Put(ctx, record); err != nil {
		return Transition{}, err
	}
	return Transition{State: state, Kind: kind, Notify: notify, RelatedKey: key, OccurrenceNo: record.OccurrenceNo}, nil
}

func applyObservationMetadata(record *Record, observation Observation) {
	if observation.Family != "" {
		record.Family = observation.Family
	}
	if observation.PolicyVersion > 0 {
		record.PolicyVersion = observation.PolicyVersion
	}
	if observation.SourceKind != "" {
		record.SourceKind = observation.SourceKind
	}
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
