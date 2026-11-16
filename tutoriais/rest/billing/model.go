package main

import "time"

type InstanceCostBreakdown struct {
	InstanceID string  `json:"instance_id"`
	Name       string  `json:"name"`
	Flavor     string  `json:"flavor"`
	HourlyRate float64 `json:"hourly_rate"`
	CostUSD    float64 `json:"cost_usd"`
}

type BudgetInfo struct {
	AccountID      string                  `json:"account_id"`
	CurrentCost    float64                 `json:"current_cost"`
	BudgetLimit    float64                 `json:"budget_limit"`
	Currency       string                  `json:"currency"`
	Exceeded       bool                    `json:"exceeded"`
	ForecastedCost float64                 `json:"forecasted_cost"`
	Instances      []InstanceCostBreakdown `json:"instances_breakdown"`
	LastUpdated    time.Time               `json:"last_updated"`
}

type SimulateChargeRequest struct {
	Amount float64 `json:"amount"`
	Reason string  `json:"reason"`
}

type UpdateBudgetLimitRequest struct {
	BudgetLimit float64 `json:"budget_limit"`
}
