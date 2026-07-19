package agent

import "context"

type Analyzer interface {
	Analyze(context.Context, IncidentContractV1) (Analysis, error)
}

type AnalysisRepository interface {
	Find(context.Context, string) (Analysis, bool, error)
	Save(context.Context, string, Analysis, bool) error
}

type Service struct {
	Analyzer   Analyzer
	Repository AnalysisRepository
}

func (s Service) AnalyzeOnce(ctx context.Context, contract IncidentContractV1) (Analysis, error) {
	if err := validateContract(contract); err != nil {
		return Analysis{}, err
	}
	if s.Repository != nil {
		if existing, found, err := s.Repository.Find(ctx, contract.IncidentID); err != nil {
			return Analysis{}, err
		} else if found {
			return existing, nil
		}
	}
	analysis := Fallback(contract)
	fallback := true
	if s.Analyzer != nil {
		if generated, err := s.Analyzer.Analyze(ctx, contract); err == nil {
			analysis = generated
			fallback = false
		}
	}
	if s.Repository != nil {
		if err := s.Repository.Save(ctx, contract.IncidentID, analysis, fallback); err != nil {
			return Analysis{}, err
		}
	}
	return analysis, nil
}
