package accounting

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestAllocationJSONSeparatesPaidAndGiftWithSignedDeltas(t *testing.T) {
	result := AllocationResult{Paid: decimal.NewFromInt(2), Gift: decimal.NewFromInt(1), Allocations: []Allocation{
		{GrantID: 4, Bucket: "paid", Quota: decimal.NewFromInt(2)},
		{GrantID: 5, Bucket: "gift", Quota: decimal.NewFromInt(1)},
	}}
	paid, gift, err := AllocationJSON(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(paid) != `[{"grant_id":4,"quota_usd":"-2.00000000"}]` {
		t.Fatalf("paid allocation=%s", paid)
	}
	if string(gift) != `[{"grant_id":5,"quota_usd":"-1.00000000"}]` {
		t.Fatalf("gift allocation=%s", gift)
	}
}
