package execution

import "time"

type PositionSide string

const (
	PositionSideLong  PositionSide = "long"
	PositionSideShort PositionSide = "short"
)

type RiskLimit struct {
	MaxPositionSize  float64
	MaxLeverage      float64
	MaxDailyLoss     float64
	StopLossRequired bool
}

type ExecutionRequest struct {
	Symbol    string
	Side      string
	Quantity  float64
	Price     float64
	Type      string
	Timestamp time.Time
}

type Portfolio struct {
	TotalValue      float64
	AvailableMargin float64
	UsedMargin      float64
	Positions       []Position
	LastUpdated     time.Time
}

const (
	PositionLong  = "long"
	PositionShort = "short"
	OrderTypeStop = "stop"
)

