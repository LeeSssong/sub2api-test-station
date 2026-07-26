package accounthealth

type ProfitInput struct {
	StandardCost float64
	UserCost     float64
	Multiplier   *float64
}

type Profit struct {
	Revenue      float64
	UpstreamCost float64
	Gross        float64
	Margin       *float64
	Computable   bool
}

// ComputeProfit derives upstream cost from the official standard cost and the
// trustworthy schema v2 multiplier. It never falls back to 1: the deprecated
// accounts.rate_multiplier is fixed at 1 in production and using it inflates
// upstream cost by 4x-20x.
func ComputeProfit(in ProfitInput) Profit {
	if in.Multiplier == nil {
		return Profit{Computable: false}
	}
	profit := Profit{
		Revenue:      in.UserCost,
		UpstreamCost: in.StandardCost * *in.Multiplier,
		Computable:   true,
	}
	profit.Gross = profit.Revenue - profit.UpstreamCost
	profit.Margin = marginOf(profit.Revenue, profit.Gross)
	return profit
}

func SumProfit(inputs []ProfitInput) (Profit, int) {
	total := Profit{}
	excluded := 0
	for _, in := range inputs {
		one := ComputeProfit(in)
		if !one.Computable {
			excluded++
			continue
		}
		total.Computable = true
		total.Revenue += one.Revenue
		total.UpstreamCost += one.UpstreamCost
	}
	if !total.Computable {
		return Profit{Computable: false}, excluded
	}
	total.Gross = total.Revenue - total.UpstreamCost
	total.Margin = marginOf(total.Revenue, total.Gross)
	return total, excluded
}

func marginOf(revenue, gross float64) *float64 {
	if revenue == 0 {
		return nil
	}
	margin := gross / revenue
	return &margin
}
