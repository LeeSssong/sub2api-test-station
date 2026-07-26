package accounthealth

import "sort"

type GroupAvailability struct {
	GroupName string
	Total     int
	Available int
	Alerting  bool
	Down      []AccountVerdict
}

// ShouldAlert encodes capacity-tiered thresholds. A group with three or more
// accounts alerts once it loses redundancy; smaller groups only alert when
// nothing is left, so single-account groups never alert while they work.
func ShouldAlert(total, available int) bool {
	if total <= 0 {
		return false
	}
	if total >= 3 {
		return available <= 1
	}
	return available == 0
}

func GroupAvailabilities(verdicts []AccountVerdict) []GroupAvailability {
	byGroup := map[string]*GroupAvailability{}
	for _, verdict := range verdicts {
		if verdict.Tier == TierUnknown {
			continue
		}
		for _, name := range verdict.GroupNames {
			if name == "" {
				continue
			}
			group, ok := byGroup[name]
			if !ok {
				group = &GroupAvailability{GroupName: name}
				byGroup[name] = group
			}
			group.Total++
			switch verdict.Tier {
			case TierHealthy, TierDegraded:
				group.Available++
			case TierUnavailable:
				group.Down = append(group.Down, verdict)
			}
		}
	}
	groups := make([]GroupAvailability, 0, len(byGroup))
	for _, group := range byGroup {
		group.Alerting = ShouldAlert(group.Total, group.Available)
		groups = append(groups, *group)
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].GroupName < groups[j].GroupName })
	return groups
}
