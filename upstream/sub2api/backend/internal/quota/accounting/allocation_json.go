package accounting

import (
	"encoding/json"
	"fmt"
)

type allocationJSONEntry struct {
	GrantID  int64  `json:"grant_id"`
	QuotaUSD string `json:"quota_usd"`
}

// AllocationJSON returns the two persisted allocation arrays. Consumption is
// represented as a signed negative delta; the domain allocation itself remains
// positive for refund/deduction callers.
func AllocationJSON(result AllocationResult) (paid, gift []byte, err error) {
	paidEntries := make([]allocationJSONEntry, 0)
	giftEntries := make([]allocationJSONEntry, 0)
	for _, allocation := range result.Allocations {
		if allocation.GrantID <= 0 || allocation.Quota.IsNegative() || allocation.Quota.IsZero() {
			return nil, nil, fmt.Errorf("invalid allocation")
		}
		entry := allocationJSONEntry{GrantID: allocation.GrantID, QuotaUSD: allocation.Quota.Neg().StringFixed(8)}
		switch allocation.Bucket {
		case "paid":
			paidEntries = append(paidEntries, entry)
		case "gift":
			giftEntries = append(giftEntries, entry)
		default:
			return nil, nil, fmt.Errorf("invalid allocation bucket %q", allocation.Bucket)
		}
	}
	paid, err = json.Marshal(paidEntries)
	if err != nil {
		return nil, nil, err
	}
	gift, err = json.Marshal(giftEntries)
	if err != nil {
		return nil, nil, err
	}
	return paid, gift, nil
}
