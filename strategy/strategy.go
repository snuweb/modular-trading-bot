package strategy

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"modular-trading-bot/indicators"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// =============================================================================
// SEGMENT 1: CORE TYPES AND CONFIGURATION
// =============================================================================

// HyperliquidConfig contains all Hyperliquid-specific configuration
type HyperliquidConfig struct {
	// Network Configuration
	ChainID      int64  `json:"chain_id"`      // Hyperliquid chain ID
	RPCURL       string `json:"rpc_url"`       // Hyperliquid RPC endpoint
	APIURL       string `json:"api_url"`       // Hyperliquid API endpoint
	WebSocketURL string `json:"websocket_url"` // WebSocket endpoint
	PrivateKey   string `json:"private_key"`   // Wallet private key
	Address      string `json:"address"`       // Wallet address

	// Trading Configuration
	DefaultLeverage decimal.Decimal `json:"default_leverage"` // Default leverage (1-50x)
	MaxLeverage     decimal.Decimal `json:"max_leverage"`     // Maximum allowed leverage
	CrossMargin     bool            `json:"cross_margin"`     // Use cross-margin mode
	IsolatedMargin  bool            `json:"isolated_margin"`  // Use isolated margin mode

	// Risk Management
	MaxPositionSize   decimal.Decimal `json:"max_position_size"`   // Max position size in USD
	LiquidationBuffer decimal.Decimal `json:"liquidation_buffer"`  // Buffer from liquidation (15%)
	StopLossPercent   decimal.Decimal `json:"stop_loss_percent"`   // Stop loss percentage
	TakeProfitPercent decimal.Decimal `json:"take_profit_percent"` // Take profit percentage
	MaxDailyLoss      decimal.Decimal `json:"max_daily_loss"`      // Max daily loss limit

	// Gas and Fees
	GasPrice        *big.Int        `json:"gas_price"`        // Gas price in wei
	GasLimit        uint64          `json:"gas_limit"`        // Gas limit
	ReservedBalance decimal.Decimal `json:"reserved_balance"` // Reserved balance for gas
	MaxSlippage     decimal.Decimal `json:"max_slippage"`     // Max slippage tolerance

	// Strategy Settings
	TradingPairs     []string        `json:"trading_pairs"`     // Supported trading pairs
	MinTradeSize     decimal.Decimal `json:"min_trade_size"`    // Minimum trade size
	UpdateInterval   time.Duration   `json:"update_interval"`   // Strategy update interval
	FundingThreshold decimal.Decimal `json:"funding_threshold"` // Funding rate threshold

	// Advanced Features
	EnableMEVProtection bool `json:"enable_mev_protection"` // MEV protection
	EnableArbitrage     bool `json:"enable_arbitrage"`      // Arbitrage opportunities
	EnableGridTrading   bool `json:"enable_grid_trading"`   // Grid trading
	EnableDCA           bool `json:"enable_dca"`            // Dollar cost averaging

	// Bot Operation
	InitialBalance    decimal.Decimal `json:"initial_balance"`    // Initial balance for the strategy
	EnabledStrategies []StrategyType  `json:"enabled_strategies"` // List of strategies to run
	EnableLogging     bool            `json:"enable_logging"`      // Enable detailed logging for the strategy
}

// DefaultHyperliquidConfig returns a default configuration for Hyperliquid strategies.
func DefaultHyperliquidConfig() HyperliquidConfig {
	return HyperliquidConfig{
		// Network Configuration
		ChainID:      0, // To be set by user
		RPCURL:       "", // To be set by user
		APIURL:       "", // To be set by user
		WebSocketURL: "", // To be set by user
		PrivateKey:   "", // To be set by user
		Address:      "", // To be set by user

		// Trading Configuration
		DefaultLeverage: decimal.NewFromFloat(10.0), // Default 10x
		MaxLeverage:     decimal.NewFromFloat(20.0), // Max 20x
		CrossMargin:     true,
		IsolatedMargin:  false,

		// Risk Management
		MaxPositionSize:   decimal.NewFromFloat(10000.0), // Max position $10,000 USD
		LiquidationBuffer: decimal.NewFromFloat(0.15),  // 15% buffer
		StopLossPercent:   decimal.NewFromFloat(0.05),  // 5% stop loss
		TakeProfitPercent: decimal.NewFromFloat(0.10),  // 10% take profit
		MaxDailyLoss:      decimal.NewFromFloat(0.05),  // Max 5% daily loss of initial balance

		// Gas and Fees
		GasPrice:        big.NewInt(20000000000),       // 20 Gwei example
		GasLimit:        300000,
		ReservedBalance: decimal.NewFromFloat(0.1),   // 0.1 ETH/Token for gas
		MaxSlippage:     decimal.NewFromFloat(0.005), // 0.5% slippage

		// Strategy Settings
		TradingPairs:     []string{"BTC-USD", "ETH-USD"}, // Default pairs
		MinTradeSize:     decimal.NewFromFloat(10.0),    // Min trade $10 USD
		UpdateInterval:   1 * time.Minute,               // Update every 1 minute
		FundingThreshold: decimal.NewFromFloat(0.0001),  // 0.01% funding rate threshold

		// Advanced Features
		EnableMEVProtection: false,
		EnableArbitrage:     false,
		EnableGridTrading:   false,
		EnableDCA:           false,

		// Bot Operation
		InitialBalance:    decimal.NewFromFloat(1000.0),                       // Default $1000 initial balance
		EnabledStrategies: []StrategyType{StrategyMomentumBreakout, StrategyMeanReversion}, // Default strategies
		EnableLogging:     true, // Enable logging by default
	}
}

// StrategyType defines different Hyperliquid trading strategies
type StrategyType string

const (
	StrategyPerpetualFutures   StrategyType = "PERPETUAL_FUTURES"
	StrategyFundingArbitrage   StrategyType = "FUNDING_ARBITRAGE"
	StrategyLiquidationHunting StrategyType = "LIQUIDATION_HUNTING"
	StrategyGridTrading        StrategyType = "GRID_TRADING"
	StrategyMomentumBreakout   StrategyType = "MOMENTUM_BREAKOUT"
	StrategyMeanReversion      StrategyType = "MEAN_REVERSION"
	StrategyDeltaNeutral       StrategyType = "DELTA_NEUTRAL"
)

// OrderType represents Hyperliquid order types
type OrderType string

const (
	OrderTypeMarket     OrderType = "MARKET"
	OrderTypeLimit      OrderType = "LIMIT"
	OrderTypeStopMarket OrderType = "STOP_MARKET"
	OrderTypeStopLimit  OrderType = "STOP_LIMIT"
	OrderTypeTakeProfit OrderType = "TAKE_PROFIT"
	OrderTypeTrailing   OrderType = "TRAILING_STOP"
)

// PositionSide represents position direction
type PositionSide string

const (
	PositionLong  PositionSide = "LONG"
	PositionShort PositionSide = "SHORT"
)

// MarginMode represents margin trading mode
type MarginMode string

const (
	MarginCross    MarginMode = "CROSS"
	MarginIsolated MarginMode = "ISOLATED"
)

// TradingSignal represents a trading signal with Hyperliquid-specific data
type TradingSignal struct {
	Timestamp   time.Time              `json:"timestamp"`
	Symbol      string                 `json:"symbol"`
	Signal      string                 `json:"signal"`     // BUY, SELL, HOLD
	Confidence  decimal.Decimal        `json:"confidence"` // 0-1
	Price       decimal.Decimal        `json:"price"`
	Size        decimal.Decimal        `json:"size"`
	Leverage    decimal.Decimal        `json:"leverage"`
	Side        PositionSide           `json:"side"`
	OrderType   OrderType              `json:"order_type"`
	StopLoss    decimal.Decimal        `json:"stop_loss"`
	TakeProfit  decimal.Decimal        `json:"take_profit"`
	FundingRate decimal.Decimal        `json:"funding_rate"`
	Liquidation decimal.Decimal        `json:"liquidation_price"`
	Margin      decimal.Decimal        `json:"margin_required"`
	Indicators  map[string]interface{} `json:"indicators"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// Position represents a Hyperliquid position
type Position struct {
	ID               string          `json:"id"`
	Symbol           string          `json:"symbol"`
	Side             PositionSide    `json:"side"`
	Size             decimal.Decimal `json:"size"`
	EntryPrice       decimal.Decimal `json:"entry_price"`
	CurrentPrice     decimal.Decimal `json:"current_price"`
	UnrealizedPnL    decimal.Decimal `json:"unrealized_pnl"`
	RealizedPnL      decimal.Decimal `json:"realized_pnl"`
	Leverage         decimal.Decimal `json:"leverage"`
	Margin           decimal.Decimal `json:"margin"`
	LiquidationPrice decimal.Decimal `json:"liquidation_price"`
	FundingPnL       decimal.Decimal `json:"funding_pnl"`
	MarginMode       MarginMode      `json:"margin_mode"`
	OpenTime         time.Time       `json:"open_time"`
	LastUpdate       time.Time       `json:"last_update"`
}

// Order represents a Hyperliquid order
type Order struct {
	ID            string          `json:"id"`
	ClientOrderID string          `json:"client_order_id"`
	Symbol        string          `json:"symbol"`
	Side          PositionSide    `json:"side"`
	Type          OrderType       `json:"type"`
	Size          decimal.Decimal `json:"size"`
	Price         decimal.Decimal `json:"price"`
	StopPrice     decimal.Decimal `json:"stop_price"`
	Status        string          `json:"status"`
	FilledSize    decimal.Decimal `json:"filled_size"`
	RemainingSize decimal.Decimal `json:"remaining_size"`
	AveragePrice  decimal.Decimal `json:"average_price"`
	Fee           decimal.Decimal `json:"fee"`
	CreateTime    time.Time       `json:"create_time"`
	UpdateTime    time.Time       `json:"update_time"`
	TimeInForce   string          `json:"time_in_force"`
	ReduceOnly    bool            `json:"reduce_only"`
	PostOnly      bool            `json:"post_only"`
}

// MarketData represents real-time market data
type MarketData struct {
	Symbol       string          `json:"symbol"`
	Price        decimal.Decimal `json:"price"`
	BidPrice     decimal.Decimal `json:"bid_price"`
	AskPrice     decimal.Decimal `json:"ask_price"`
	BidSize      decimal.Decimal `json:"bid_size"`
	AskSize      decimal.Decimal `json:"ask_size"`
	Volume24h    decimal.Decimal `json:"volume_24h"`
	Change24h    decimal.Decimal `json:"change_24h"`
	High24h      decimal.Decimal `json:"high_24h"`
	Low24h       decimal.Decimal `json:"low_24h"`
	FundingRate  decimal.Decimal `json:"funding_rate"`
	NextFunding  time.Time       `json:"next_funding"`
	OpenInterest decimal.Decimal `json:"open_interest"`
	IndexPrice   decimal.Decimal `json:"index_price"`
	MarkPrice    decimal.Decimal `json:"mark_price"`
	LastUpdate   time.Time       `json:"last_update"`
}

// AccountInfo represents account information
type AccountInfo struct {
	Address            string          `json:"address"`
	TotalBalance       decimal.Decimal `json:"total_balance"`
	AvailableBalance   decimal.Decimal `json:"available_balance"`
	UsedMargin         decimal.Decimal `json:"used_margin"`
	FreeMargin         decimal.Decimal `json:"free_margin"`
	TotalUnrealizedPnL decimal.Decimal `json:"total_unrealized_pnl"`
	TotalRealizedPnL   decimal.Decimal `json:"total_realized_pnl"`
	TotalFunding       decimal.Decimal `json:"total_funding"`
	PositionCount      int             `json:"position_count"`
	OrderCount         int             `json:"order_count"`
	Leverage           decimal.Decimal `json:"leverage"`
	MarginRatio        decimal.Decimal `json:"margin_ratio"`
	MaintenanceMargin  decimal.Decimal `json:"maintenance_margin"`
	LastUpdate         time.Time       `json:"last_update"`
}

// RiskMetrics represents risk management metrics
type RiskMetrics struct {
	CurrentLeverage     decimal.Decimal `json:"current_leverage"`
	MaxLeverage         decimal.Decimal `json:"max_leverage"`
	LeverageUtilization decimal.Decimal `json:"leverage_utilization"`
	LiquidationRisk     decimal.Decimal `json:"liquidation_risk"`
	VaR                 decimal.Decimal `json:"var"` // Value at Risk
	MaxDrawdown         decimal.Decimal `json:"max_drawdown"`
	CurrentDrawdown     decimal.Decimal `json:"current_drawdown"`
	SharpeRatio         decimal.Decimal `json:"sharpe_ratio"`
	WinRate             decimal.Decimal `json:"win_rate"`
	ProfitFactor        decimal.Decimal `json:"profit_factor"`
	PortfolioHeat       decimal.Decimal `json:"portfolio_heat"` // Total risk exposure
	DailyPnL            decimal.Decimal `json:"daily_pnl"`
	DailyLossLimit      decimal.Decimal `json:"daily_loss_limit"`
	RemainingRisk       decimal.Decimal `json:"remaining_risk"`
}

// FundingInfo represents funding rate information
type FundingInfo struct {
	Symbol          string            `json:"symbol"`
	CurrentRate     decimal.Decimal   `json:"current_rate"`
	PredictedRate   decimal.Decimal   `json:"predicted_rate"`
	NextFundingTime time.Time         `json:"next_funding_time"`
	TimeToFunding   time.Duration     `json:"time_to_funding"`
	HistoricalRates []decimal.Decimal `json:"historical_rates"`
	AverageRate     decimal.Decimal   `json:"average_rate"`
	Trend           string            `json:"trend"`           // INCREASING, DECREASING, STABLE
	ArbitrageScore  decimal.Decimal   `json:"arbitrage_score"` // 0-1 opportunity score
}

// GridConfig represents grid trading configuration
type GridConfig struct {
	Symbol       string          `json:"symbol"`
	GridLevels   int             `json:"grid_levels"`
	GridSpacing  decimal.Decimal `json:"grid_spacing"` // Percentage spacing between levels
	BasePrice    decimal.Decimal `json:"base_price"`
	UpperBound   decimal.Decimal `json:"upper_bound"`
	LowerBound   decimal.Decimal `json:"lower_bound"`
	OrderSize    decimal.Decimal `json:"order_size"`
	TotalCapital decimal.Decimal `json:"total_capital"`
	MaxPositions int             `json:"max_positions"`
	Enabled      bool            `json:"enabled"`
}

// StrategyState represents the current state of the strategy
type StrategyState struct {
	mu              sync.RWMutex
	IsRunning       bool                   `json:"is_running"`
	StartTime       time.Time              `json:"start_time"`
	LastUpdate      time.Time              `json:"last_update"`
	TotalTrades     int64                  `json:"total_trades"`
	WinningTrades   int64                  `json:"winning_trades"`
	LosingTrades    int64                  `json:"losing_trades"`
	TotalPnL        decimal.Decimal        `json:"total_pnl"`
	TotalFees       decimal.Decimal        `json:"total_fees"`
	MaxDrawdown     decimal.Decimal        `json:"max_drawdown"`
	CurrentDrawdown decimal.Decimal        `json:"current_drawdown"`
	Positions       map[string]*Position   `json:"positions"`
	Orders          map[string]*Order      `json:"orders"`
	Signals         []TradingSignal        `json:"signals"`
	Performance     map[string]interface{} `json:"performance"`
	Errors          []string               `json:"errors"`
	Warnings        []string               `json:"warnings"`
}

// Trade represents a completed trade
type Trade struct {
	ID         string          `json:"id"`
	Symbol     string          `json:"symbol"`
	Side       PositionSide    `json:"side"`
	Size       decimal.Decimal `json:"size"`
	EntryPrice decimal.Decimal `json:"entry_price"`
	ExitPrice  decimal.Decimal `json:"exit_price"`
	PnL        decimal.Decimal `json:"pnl"`
	Fee        decimal.Decimal `json:"fee"`
	Duration   time.Duration   `json:"duration"`
	Strategy   StrategyType    `json:"strategy"`
	OpenTime   time.Time       `json:"open_time"`
	CloseTime  time.Time       `json:"close_time"`
	Reason     string          `json:"reason"` // Signal, StopLoss, TakeProfit, etc.
}

// PerformanceSnapshot represents performance at a point in time
// NotificationSeverity defines the severity levels for strategy notifications.
type NotificationSeverity string

// Defines the various severity levels for notifications.
const (
	SeverityInfo     NotificationSeverity = "INFO"
	SeverityWarning  NotificationSeverity = "WARNING"
	SeverityError    NotificationSeverity = "ERROR"
	SeverityCritical NotificationSeverity = "CRITICAL"
)

// StrategyNotification is used to send notifications from the strategy engine.
type StrategyNotification struct {
	Timestamp time.Time              `json:"timestamp"`
	Type      string                 `json:"type"`    // e.g., "ORDER_FILLED", "ERROR", "INFO_UPDATE"
	Message   string                 `json:"message"`
	Severity  NotificationSeverity   `json:"severity"`
	Details   map[string]interface{} `json:"details,omitempty"` // Optional additional details
}

// PerformanceSnapshot represents performance at a point in time
type PerformanceSnapshot struct {
	Timestamp       time.Time       `json:"timestamp"`
	PortfolioValue  decimal.Decimal `json:"portfolio_value"`
	TotalPnL        decimal.Decimal `json:"total_pnl"`
	DailyPnL        decimal.Decimal `json:"daily_pnl"`
	WinRate         decimal.Decimal `json:"win_rate"`
	SharpeRatio     decimal.Decimal `json:"sharpe_ratio"`
	MaxDrawdown     decimal.Decimal `json:"max_drawdown"`
	CurrentDrawdown decimal.Decimal `json:"current_drawdown"`
	ActivePositions int             `json:"active_positions"`
	TotalTrades     int             `json:"total_trades"`
}

// =============================================================================
// SEGMENT 2: STRATEGY INTERFACE AND CORE IMPLEMENTATION
// =============================================================================

// HyperliquidStrategy defines the main strategy interface
type HyperliquidStrategy interface {
	// Core Strategy Methods
	Initialize(config HyperliquidConfig) error
	Start(ctx context.Context) error
	Stop() error
	IsRunning() bool

	// Market Data Methods
	UpdateMarketData(symbol string, data MarketData) error
	GetMarketData(symbol string) (MarketData, error)

	// Indicator Integration (using YOUR indicators module)
	UpdateIndicators(symbol string, price decimal.Decimal) error
	GetTradingSignals(symbol string) (TradingSignal, error)
	GetIndicatorSnapshot(symbol string) (indicators.IndicatorSnapshot, error)

	// Trading Methods
	PlaceOrder(order *Order) error
	CancelOrder(orderID string) error
	GetPositions() map[string]*Position
	GetOrders() map[string]*Order
	ClosePosition(symbol string) error

	// Risk Management
	CalculatePositionSize(signal TradingSignal) (decimal.Decimal, error)
	CheckRiskLimits(signal TradingSignal) error
	GetRiskMetrics() RiskMetrics

	// Portfolio Management
	GetAccountInfo() AccountInfo
	GetPortfolioValue() decimal.Decimal
	RebalancePortfolio() error

	// Strategy-Specific Methods
	ExecutePerpetualFuturesStrategy(symbol string) error
	ExecuteFundingArbitrageStrategy() error
	ExecuteLiquidationHuntingStrategy() error
	ExecuteGridTradingStrategy(symbol string) error

	// Configuration and State
	UpdateConfig(config HyperliquidConfig) error
	GetState() StrategyState
	Reset() error
}

// hyperliquidStrategy implements the HyperliquidStrategy interface
type hyperliquidStrategy struct {
	// Configuration
	config HyperliquidConfig
	logger *zap.Logger

	// Core Components - YOUR indicators for each symbol
	indicatorEngines map[string]indicators.IndicatorEngine
	state            *StrategyState

	// Market Data
	marketData  map[string]MarketData
	fundingData map[string]FundingInfo

	// Trading State
	positions map[string]*Position
	orders    map[string]*Order
	account   AccountInfo

	// Risk Management
	riskMetrics RiskMetrics
	dailyStats  map[string]decimal.Decimal

	// Grid Trading
	gridConfigs map[string]*GridConfig
	gridOrders  map[string][]string // symbol -> order IDs

	// Synchronization
	mu           sync.RWMutex
	stopChan     chan struct{}
	updateTicker *time.Ticker

	// Performance Tracking
	tradeHistory   []Trade
	performanceLog []PerformanceSnapshot
}

// NewHyperliquidStrategy creates a new Hyperliquid strategy instance
func NewHyperliquidStrategy(logger *zap.Logger) HyperliquidStrategy {
	return &hyperliquidStrategy{
		logger:           logger,
		indicatorEngines: make(map[string]indicators.IndicatorEngine),
		marketData:       make(map[string]MarketData),
		fundingData:      make(map[string]FundingInfo),
		positions:        make(map[string]*Position),
		orders:           make(map[string]*Order),
		gridConfigs:      make(map[string]*GridConfig),
		gridOrders:       make(map[string][]string),
		dailyStats:       make(map[string]decimal.Decimal),
		stopChan:         make(chan struct{}),
		tradeHistory:     make([]Trade, 0),
		performanceLog:   make([]PerformanceSnapshot, 0),
		state: &StrategyState{
			Positions:   make(map[string]*Position),
			Orders:      make(map[string]*Order),
			Signals:     make([]TradingSignal, 0),
			Performance: make(map[string]interface{}),
			Errors:      make([]string, 0),
			Warnings:    make([]string, 0),
		},
	}
}

// Initialize implements HyperliquidStrategy.Initialize
func (hs *hyperliquidStrategy) Initialize(config HyperliquidConfig) error {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	hs.config = config

	// Initialize indicator engines for each trading pair using YOUR indicators
	for _, symbol := range config.TradingPairs {
		indicatorConfig := indicators.DefaultConfig()

		// Customize indicator config for Hyperliquid
		indicatorConfig.MaxHistorySize = 1000
		indicatorConfig.RSIPeriod = 14
		indicatorConfig.MACDConfig.FastPeriod = 12
		indicatorConfig.MACDConfig.SlowPeriod = 26
		indicatorConfig.MACDConfig.SignalPeriod = 9
		indicatorConfig.EnableLogging = true

		// Set signal thresholds optimized for crypto
		indicatorConfig.SignalThresholds.RSIOverbought = 75
		indicatorConfig.SignalThresholds.RSIOversold = 25
		indicatorConfig.SignalThresholds.StochOverbought = 85
		indicatorConfig.SignalThresholds.StochOversold = 15

		engine := indicators.NewIndicatorEngine(indicatorConfig)
		hs.indicatorEngines[symbol] = engine

		hs.logger.Info("Initialized indicator engine for symbol", zap.String("symbol", symbol))
	}

	// Initialize grid trading configs if enabled
	if config.EnableGridTrading {
		for _, symbol := range config.TradingPairs {
			hs.initializeGridConfig(symbol)
		}
	}

	// Initialize state
	hs.state.StartTime = time.Now()
	hs.state.IsRunning = false

	hs.logger.Info("Hyperliquid strategy initialized successfully",
		zap.Int("trading_pairs", len(config.TradingPairs)),
		zap.Bool("grid_trading", config.EnableGridTrading),
		zap.Bool("arbitrage", config.EnableArbitrage))

	return nil
}

// Start implements HyperliquidStrategy.Start
func (hs *hyperliquidStrategy) Start(ctx context.Context) error {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	if hs.state.IsRunning {
		return errors.New("strategy is already running")
	}

	hs.state.IsRunning = true
	hs.state.StartTime = time.Now()
	hs.updateTicker = time.NewTicker(hs.config.UpdateInterval)

	// Start the main strategy loop
	go hs.runStrategyLoop(ctx)

	hs.logger.Info("Hyperliquid strategy started")
	return nil
}

// Stop implements HyperliquidStrategy.Stop
func (hs *hyperliquidStrategy) Stop() error {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	if !hs.state.IsRunning {
		return errors.New("strategy is not running")
	}

	close(hs.stopChan)
	if hs.updateTicker != nil {
		hs.updateTicker.Stop()
	}

	hs.state.IsRunning = false

	hs.logger.Info("Hyperliquid strategy stopped")
	return nil
}

// IsRunning implements HyperliquidStrategy.IsRunning
func (hs *hyperliquidStrategy) IsRunning() bool {
	hs.mu.RLock()
	defer hs.mu.RUnlock()
	return hs.state.IsRunning
}

// UpdateMarketData implements HyperliquidStrategy.UpdateMarketData
func (hs *hyperliquidStrategy) UpdateMarketData(symbol string, data MarketData) error {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	hs.marketData[symbol] = data

	// Update indicators with new price using YOUR indicators module
	if engine, exists := hs.indicatorEngines[symbol]; exists {
		price, _ := data.Price.Float64()
		if err := engine.UpdatePrice(price); err != nil {
			hs.logger.Error("Failed to update indicators",
				zap.String("symbol", symbol),
				zap.Error(err))
			return err
		}
	}

	return nil
}

// GetMarketData implements HyperliquidStrategy.GetMarketData
func (hs *hyperliquidStrategy) GetMarketData(symbol string) (MarketData, error) {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	data, exists := hs.marketData[symbol]
	if !exists {
		return MarketData{}, errors.New("market data not found for symbol: " + symbol)
	}

	return data, nil
}

// UpdateIndicators implements HyperliquidStrategy.UpdateIndicators
func (hs *hyperliquidStrategy) UpdateIndicators(symbol string, price decimal.Decimal) error {
	hs.mu.RLock()
	engine, exists := hs.indicatorEngines[symbol]
	hs.mu.RUnlock()

	if !exists {
		return errors.New("indicator engine not found for symbol: " + symbol)
	}

	// Convert decimal to float64 for YOUR indicators
	priceFloat, _ := price.Float64()
	return engine.UpdatePrice(priceFloat)
}

// GetTradingSignals implements HyperliquidStrategy.GetTradingSignals
func (hs *hyperliquidStrategy) GetTradingSignals(symbol string) (TradingSignal, error) {
	hs.mu.RLock()
	engine, exists := hs.indicatorEngines[symbol]
	marketData, dataExists := hs.marketData[symbol]
	hs.mu.RUnlock()

	if !exists {
		return TradingSignal{}, errors.New("indicator engine not found for symbol: " + symbol)
	}

	if !dataExists {
		return TradingSignal{}, errors.New("market data not found for symbol: " + symbol)
	}

	// Get signals from YOUR indicators module
	indicatorSignals := engine.GetSignals()

	// Convert YOUR indicator signals to Hyperliquid trading signals
	signal := hs.convertToHyperliquidSignal(symbol, indicatorSignals, marketData)

	return signal, nil
}

// GetIndicatorSnapshot implements HyperliquidStrategy.GetIndicatorSnapshot
func (hs *hyperliquidStrategy) GetIndicatorSnapshot(symbol string) (indicators.IndicatorSnapshot, error) {
	hs.mu.RLock()
	engine, exists := hs.indicatorEngines[symbol]
	hs.mu.RUnlock()

	if !exists {
		return indicators.IndicatorSnapshot{}, errors.New("indicator engine not found for symbol: " + symbol)
	}

	return engine.GetIndicatorValues(), nil
}

// convertToHyperliquidSignal converts YOUR indicator signals to Hyperliquid-specific signals
func (hs *hyperliquidStrategy) convertToHyperliquidSignal(symbol string, indicatorSignals indicators.TradingSignals, marketData MarketData) TradingSignal {
	signal := TradingSignal{
		Timestamp:  indicatorSignals.Timestamp,
		Symbol:     symbol,
		Price:      marketData.Price,
		Indicators: make(map[string]interface{}),
		Metadata:   make(map[string]interface{}),
	}

	// Convert YOUR indicator signals to Hyperliquid format
	switch indicatorSignals.Overall {
	case indicators.SignalBuy:
		signal.Signal = "BUY"
		signal.Side = PositionLong
	case indicators.SignalSell:
		signal.Signal = "SELL"
		signal.Side = PositionShort
	default:
		signal.Signal = "HOLD"
	}

	// Set confidence from YOUR indicators
	signal.Confidence = decimal.NewFromFloat(indicatorSignals.Confidence)

	// Add funding rate consideration for Hyperliquid
	if fundingInfo, exists := hs.fundingData[symbol]; exists {
		signal.FundingRate = fundingInfo.CurrentRate
		signal.Metadata["funding_rate"] = fundingInfo.CurrentRate
		signal.Metadata["next_funding"] = fundingInfo.NextFundingTime
	}

	// Calculate position size based on risk management
	if positionSize, err := hs.CalculatePositionSize(signal); err == nil {
		signal.Size = positionSize
	}

	// Set leverage based on configuration and market conditions
	signal.Leverage = hs.calculateOptimalLeverage(symbol, signal)

	// Set order type based on market conditions
	signal.OrderType = hs.determineOrderType(signal)

	// Calculate stop loss and take profit for Hyperliquid
	signal.StopLoss = hs.calculateStopLoss(signal)
	signal.TakeProfit = hs.calculateTakeProfit(signal)

	// Copy indicator data from YOUR indicators
	signal.Indicators["trend"] = string(indicatorSignals.Trend)
	signal.Indicators["momentum"] = string(indicatorSignals.Momentum)
	signal.Indicators["volatility"] = string(indicatorSignals.Volatility)

	return signal
}

// runStrategyLoop is the main strategy execution loop
func (hs *hyperliquidStrategy) runStrategyLoop(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			hs.logger.Error("Strategy loop panic recovered", zap.Any("panic", r))
		}
	}()

	for {
		select {
		case <-ctx.Done():
			hs.logger.Info("Strategy loop stopped by context")
			return
		case <-hs.stopChan:
			hs.logger.Info("Strategy loop stopped by stop channel")
			return
		case <-hs.updateTicker.C:
			hs.executeStrategyUpdate()
		}
	}
}

// executeStrategyUpdate performs one strategy update cycle
func (hs *hyperliquidStrategy) executeStrategyUpdate() {
	defer func() {
		if r := recover(); r != nil {
			hs.logger.Error("Strategy update panic recovered", zap.Any("panic", r))
		}
	}()

	// Update account info and risk metrics
	if err := hs.updateAccountInfo(); err != nil {
		hs.logger.Error("Failed to update account info", zap.Error(err))
	}

	// Check risk limits before executing any trades
	if err := hs.checkGlobalRiskLimits(); err != nil {
		hs.logger.Warn("Risk limits exceeded, skipping trade execution", zap.Error(err))
		return
	}

	// Execute strategies for each trading pair
	for _, symbol := range hs.config.TradingPairs {
		if err := hs.executeSymbolStrategy(symbol); err != nil {
			hs.logger.Error("Failed to execute strategy for symbol",
				zap.String("symbol", symbol),
				zap.Error(err))
		}
	}

	// Execute funding arbitrage if enabled
	if hs.config.EnableArbitrage {
		if err := hs.ExecuteFundingArbitrageStrategy(); err != nil {
			hs.logger.Error("Failed to execute funding arbitrage", zap.Error(err))
		}
	}

	// Update performance metrics
	hs.updatePerformanceMetrics()

	// Update state timestamp
	hs.mu.Lock()
	hs.state.LastUpdate = time.Now()
	hs.mu.Unlock()
}

// =============================================================================
// SEGMENT 3: TRADING EXECUTION AND RISK MANAGEMENT
// =============================================================================

// executeSymbolStrategy executes trading strategy for a specific symbol
func (hs *hyperliquidStrategy) executeSymbolStrategy(symbol string) error {
	// Get trading signal from YOUR indicators
	signal, err := hs.GetTradingSignals(symbol)
	if err != nil {
		return err
	}

	// Check if we should trade based on signal confidence
	if signal.Confidence.LessThan(decimal.NewFromFloat(0.6)) {
		return nil // Skip low confidence signals
	}

	// Execute different strategies based on configuration
	switch {
	case hs.config.EnableGridTrading:
		return hs.ExecuteGridTradingStrategy(symbol)
	case signal.Signal == "BUY" || signal.Signal == "SELL":
		return hs.executeTrendFollowingStrategy(symbol, signal)
	default:
		return hs.executePositionManagement(symbol)
	}
}

// executeTrendFollowingStrategy executes trend-following trades based on YOUR indicators
func (hs *hyperliquidStrategy) executeTrendFollowingStrategy(symbol string, signal TradingSignal) error {
	// Check if we already have a position in this symbol
	existingPosition := hs.getPosition(symbol)

	// If we have an opposite position, close it first
	if existingPosition != nil &&
		((signal.Side == PositionLong && existingPosition.Side == PositionShort) ||
			(signal.Side == PositionShort && existingPosition.Side == PositionLong)) {

		if err := hs.ClosePosition(symbol); err != nil {
			return err
		}
	}

	// Don't open new position if we already have one in the same direction
	if existingPosition != nil && existingPosition.Side == signal.Side {
		return nil
	}

	// Create and place the order
	order := hs.createOrderFromSignal(signal)
	return hs.PlaceOrder(order)
}

// PlaceOrder implements HyperliquidStrategy.PlaceOrder
func (hs *hyperliquidStrategy) PlaceOrder(order *Order) error {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	// Validate order before placing
	if err := hs.validateOrder(order); err != nil {
		return err
	}

	// Check risk limits for this order
	signal := TradingSignal{
		Symbol: order.Symbol,
		Size:   order.Size,
		Side:   order.Side,
		Price:  order.Price,
	}

	if err := hs.CheckRiskLimits(signal); err != nil {
		return err
	}

	// Generate unique order ID
	order.ID = hs.generateOrderID()
	order.CreateTime = time.Now()
	order.Status = "PENDING"

	// Store order in memory (in real implementation, this would call Hyperliquid API)
	hs.orders[order.ID] = order
	hs.state.Orders[order.ID] = order

	hs.logger.Info("Order placed successfully",
		zap.String("id", order.ID),
		zap.String("symbol", order.Symbol),
		zap.String("side", string(order.Side)),
		zap.String("size", order.Size.String()),
		zap.String("price", order.Price.String()))

	// Simulate order execution (in real implementation, this would be handled by exchange callbacks)
	go hs.simulateOrderExecution(order)

	return nil
}

// CalculatePositionSize implements HyperliquidStrategy.CalculatePositionSize
func (hs *hyperliquidStrategy) CalculatePositionSize(signal TradingSignal) (decimal.Decimal, error) {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	// Get available balance
	availableBalance := hs.account.AvailableBalance
	if availableBalance.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, errors.New("insufficient balance")
	}

	// Calculate base position size (percentage of available balance)
	basePercent := decimal.NewFromFloat(0.02) // 2% per trade
	if signal.Confidence.GreaterThan(decimal.NewFromFloat(0.8)) {
		basePercent = decimal.NewFromFloat(0.05) // 5% for high confidence
	}

	// Calculate position size in USD
	positionValueUSD := availableBalance.Mul(basePercent)

	// Apply leverage
	leverage := signal.Leverage
	if leverage.LessThanOrEqual(decimal.Zero) {
		leverage = hs.config.DefaultLeverage
	}

	// Calculate position size in units
	positionSize := positionValueUSD.Mul(leverage).Div(signal.Price)

	// Apply maximum position size limit
	if positionSize.GreaterThan(hs.config.MaxPositionSize.Div(signal.Price)) {
		positionSize = hs.config.MaxPositionSize.Div(signal.Price)
	}

	// Apply minimum trade size
	if positionSize.LessThan(hs.config.MinTradeSize.Div(signal.Price)) {
		return decimal.Zero, errors.New("position size below minimum")
	}

	return positionSize, nil
}

// CheckRiskLimits implements HyperliquidStrategy.CheckRiskLimits
func (hs *hyperliquidStrategy) CheckRiskLimits(signal TradingSignal) error {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	// Check daily loss limit
	if hs.riskMetrics.DailyPnL.LessThan(hs.config.MaxDailyLoss.Neg()) {
		return errors.New("daily loss limit exceeded")
	}

	// Check maximum leverage
	if signal.Leverage.GreaterThan(hs.config.MaxLeverage) {
		return errors.New("leverage exceeds maximum allowed")
	}

	// Check if adding this position would exceed portfolio heat
	newPositionValue := signal.Size.Mul(signal.Price)
	currentPortfolioValue := hs.GetPortfolioValue()
	newHeat := hs.riskMetrics.PortfolioHeat.Add(newPositionValue.Div(currentPortfolioValue))

	if newHeat.GreaterThan(decimal.NewFromFloat(0.95)) { // 95% max portfolio utilization
		return errors.New("position would exceed portfolio heat limit")
	}

	// Check liquidation risk
	if hs.riskMetrics.LiquidationRisk.GreaterThan(decimal.NewFromFloat(0.85)) {
		return errors.New("liquidation risk too high")
	}

	return nil
}

// checkGlobalRiskLimits checks overall portfolio risk limits
func (hs *hyperliquidStrategy) checkGlobalRiskLimits() error {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	// Check if strategy should be paused due to high drawdown
	if hs.riskMetrics.CurrentDrawdown.GreaterThan(decimal.NewFromFloat(0.15)) { // 15% drawdown
		return errors.New("maximum drawdown exceeded, strategy paused")
	}

	// Check margin ratio
	if hs.account.MarginRatio.LessThan(decimal.NewFromFloat(0.2)) { // 20% margin ratio
		return errors.New("margin ratio too low")
	}

	// Check if too many positions are open
	if len(hs.positions) >= 10 { // Max 10 concurrent positions
		return errors.New("maximum concurrent positions reached")
	}

	return nil
}

// GetRiskMetrics implements HyperliquidStrategy.GetRiskMetrics
func (hs *hyperliquidStrategy) GetRiskMetrics() RiskMetrics {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	// Calculate current metrics
	hs.calculateRiskMetrics()

	return hs.riskMetrics
}

// calculateRiskMetrics calculates current risk metrics
func (hs *hyperliquidStrategy) calculateRiskMetrics() {
	// Calculate current leverage
	totalPositionValue := decimal.Zero
	totalMargin := decimal.Zero

	for _, position := range hs.positions {
		positionValue := position.Size.Mul(position.CurrentPrice)
		totalPositionValue = totalPositionValue.Add(positionValue)
		totalMargin = totalMargin.Add(position.Margin)
	}

	if totalMargin.GreaterThan(decimal.Zero) {
		hs.riskMetrics.CurrentLeverage = totalPositionValue.Div(totalMargin)
	}

	// Calculate leverage utilization
	if hs.config.MaxLeverage.GreaterThan(decimal.Zero) {
		hs.riskMetrics.LeverageUtilization = hs.riskMetrics.CurrentLeverage.Div(hs.config.MaxLeverage)
	}

	// Calculate portfolio heat (percentage of portfolio at risk)
	portfolioValue := hs.GetPortfolioValue()
	if portfolioValue.GreaterThan(decimal.Zero) {
		hs.riskMetrics.PortfolioHeat = totalPositionValue.Div(portfolioValue)
	}

	// Calculate liquidation risk
	hs.riskMetrics.LiquidationRisk = hs.calculateLiquidationRisk()

	// Calculate daily PnL
	hs.riskMetrics.DailyPnL = hs.calculateDailyPnL()

	// Calculate win rate
	hs.riskMetrics.WinRate = hs.calculateWinRate()
}

// calculateLiquidationRisk calculates the risk of liquidation
func (hs *hyperliquidStrategy) calculateLiquidationRisk() decimal.Decimal {
	if len(hs.positions) == 0 {
		return decimal.Zero
	}

	totalRisk := decimal.Zero
	totalValue := decimal.Zero

	for _, position := range hs.positions {
		if position.LiquidationPrice.GreaterThan(decimal.Zero) {
			currentPrice := position.CurrentPrice
			liquidationPrice := position.LiquidationPrice

			var riskPercent decimal.Decimal
			if position.Side == PositionLong {
				// For long positions, risk is how close current price is to liquidation price
				riskPercent = liquidationPrice.Div(currentPrice)
			} else {
				// For short positions, risk is how close liquidation price is to current price
				riskPercent = currentPrice.Div(liquidationPrice)
			}

			positionValue := position.Size.Mul(currentPrice)
			totalRisk = totalRisk.Add(riskPercent.Mul(positionValue))
			totalValue = totalValue.Add(positionValue)
		}
	}

	if totalValue.GreaterThan(decimal.Zero) {
		return totalRisk.Div(totalValue)
	}

	return decimal.Zero
}

// calculateOptimalLeverage calculates optimal leverage based on market conditions
func (hs *hyperliquidStrategy) calculateOptimalLeverage(symbol string, signal TradingSignal) decimal.Decimal {
	baseLeverage := hs.config.DefaultLeverage

	// Reduce leverage in high volatility using YOUR indicators
	if marketData, exists := hs.marketData[symbol]; exists {
		// Get volatility from YOUR indicators
		if snapshot, err := hs.GetIndicatorSnapshot(symbol); err == nil {
			atr := decimal.NewFromFloat(snapshot.ATR)
			currentPrice := marketData.Price

			if atr.GreaterThan(decimal.Zero) && currentPrice.GreaterThan(decimal.Zero) {
				volatilityPercent := atr.Div(currentPrice).Mul(decimal.NewFromInt(100))

				// Reduce leverage if volatility is high
				if volatilityPercent.GreaterThan(decimal.NewFromFloat(3)) { // 3% ATR
					baseLeverage = baseLeverage.Mul(decimal.NewFromFloat(0.5)) // 50% reduction
				} else if volatilityPercent.GreaterThan(decimal.NewFromFloat(2)) { // 2% ATR
					baseLeverage = baseLeverage.Mul(decimal.NewFromFloat(0.75)) // 25% reduction
				}
			}
		}
	}

	// Adjust leverage based on signal confidence
	confidenceMultiplier := signal.Confidence
	leverage := baseLeverage.Mul(confidenceMultiplier)

	// Ensure leverage doesn't exceed maximum
	if leverage.GreaterThan(hs.config.MaxLeverage) {
		leverage = hs.config.MaxLeverage
	}

	// Ensure minimum leverage of 1x
	if leverage.LessThan(decimal.NewFromInt(1)) {
		leverage = decimal.NewFromInt(1)
	}

	return leverage
}

// determineOrderType determines the best order type based on market conditions
func (hs *hyperliquidStrategy) determineOrderType(signal TradingSignal) OrderType {
	// Use market orders for strong signals with high confidence
	if signal.Confidence.GreaterThan(decimal.NewFromFloat(0.85)) {
		return OrderTypeMarket
	}

	// Use limit orders for regular signals to get better pricing
	return OrderTypeLimit
}

// calculateStopLoss calculates stop loss price based on YOUR indicators
func (hs *hyperliquidStrategy) calculateStopLoss(signal TradingSignal) decimal.Decimal {
	// Get ATR for dynamic stop loss from YOUR indicators
	if snapshot, err := hs.GetIndicatorSnapshot(signal.Symbol); err == nil {
		atr := decimal.NewFromFloat(snapshot.ATR)

		if atr.GreaterThan(decimal.Zero) {
			// Use 2x ATR for stop loss
			atrMultiplier := decimal.NewFromFloat(2.0)
			stopDistance := atr.Mul(atrMultiplier)

			if signal.Side == PositionLong {
				return signal.Price.Sub(stopDistance)
			} else {
				return signal.Price.Add(stopDistance)
			}
		}
	}

	// Fallback to percentage-based stop loss
	stopPercent := hs.config.StopLossPercent
	if signal.Side == PositionLong {
		return signal.Price.Mul(decimal.NewFromInt(1).Sub(stopPercent))
	} else {
		return signal.Price.Mul(decimal.NewFromInt(1).Add(stopPercent))
	}
}

// calculateTakeProfit calculates take profit price
func (hs *hyperliquidStrategy) calculateTakeProfit(signal TradingSignal) decimal.Decimal {
	// Use risk-reward ratio of 1:2 (risk 1 to make 2)
	stopLoss := hs.calculateStopLoss(signal)
	riskDistance := signal.Price.Sub(stopLoss).Abs()
	rewardDistance := riskDistance.Mul(decimal.NewFromFloat(2.0))

	if signal.Side == PositionLong {
		return signal.Price.Add(rewardDistance)
	} else {
		return signal.Price.Sub(rewardDistance)
	}
}

// createOrderFromSignal creates an order from a trading signal
func (hs *hyperliquidStrategy) createOrderFromSignal(signal TradingSignal) *Order {
	order := &Order{
		ClientOrderID: hs.generateClientOrderID(),
		Symbol:        signal.Symbol,
		Side:          signal.Side,
		Type:          signal.OrderType,
		Size:          signal.Size,
		Price:         signal.Price,
		TimeInForce:   "GTC", // Good Till Cancelled
		ReduceOnly:    false,
		PostOnly:      signal.OrderType == OrderTypeLimit,
	}

	return order
}

// validateOrder validates an order before placing
func (hs *hyperliquidStrategy) validateOrder(order *Order) error {
	if order.Size.LessThanOrEqual(decimal.Zero) {
		return errors.New("invalid order size")
	}

	if order.Price.LessThanOrEqual(decimal.Zero) {
		return errors.New("invalid order price")
	}

	if order.Symbol == "" {
		return errors.New("missing symbol")
	}

	// Check minimum trade size
	orderValue := order.Size.Mul(order.Price)
	if orderValue.LessThan(hs.config.MinTradeSize) {
		return errors.New("order value below minimum trade size")
	}

	return nil
}

// simulateOrderExecution simulates order execution (replace with real Hyperliquid API calls)
func (hs *hyperliquidStrategy) simulateOrderExecution(order *Order) {
	// Simulate execution delay
	time.Sleep(time.Millisecond * 100)

	hs.mu.Lock()
	defer hs.mu.Unlock()

	// Mark order as filled
	order.Status = "FILLED"
	order.FilledSize = order.Size
	order.RemainingSize = decimal.Zero
	order.AveragePrice = order.Price
	order.UpdateTime = time.Now()

	// Create or update position
	hs.updatePositionFromOrder(order)

	// Log execution
	hs.logger.Info("Order executed",
		zap.String("id", order.ID),
		zap.String("symbol", order.Symbol),
		zap.String("side", string(order.Side)),
		zap.String("size", order.FilledSize.String()),
		zap.String("price", order.AveragePrice.String()))
}

// updatePositionFromOrder updates position based on executed order
func (hs *hyperliquidStrategy) updatePositionFromOrder(order *Order) {
	existingPosition := hs.positions[order.Symbol]

	if existingPosition == nil {
		// Create new position
		position := &Position{
			ID:           hs.generatePositionID(),
			Symbol:       order.Symbol,
			Side:         order.Side,
			Size:         order.FilledSize,
			EntryPrice:   order.AveragePrice,
			CurrentPrice: order.AveragePrice,
			Leverage:     hs.config.DefaultLeverage,
			OpenTime:     order.UpdateTime,
			LastUpdate:   order.UpdateTime,
		}

		// Calculate margin and liquidation price
		position.Margin = position.Size.Mul(position.EntryPrice).Div(position.Leverage)
		position.LiquidationPrice = hs.calculateLiquidationPrice(position)

		hs.positions[order.Symbol] = position
		hs.state.Positions[order.Symbol] = position

	} else {
		// Update existing position (averaging or closing)
		if existingPosition.Side == order.Side {
			// Average the position
			totalSize := existingPosition.Size.Add(order.FilledSize)
			totalValue := existingPosition.Size.Mul(existingPosition.EntryPrice).Add(
				order.FilledSize.Mul(order.AveragePrice))

			existingPosition.EntryPrice = totalValue.Div(totalSize)
			existingPosition.Size = totalSize
			existingPosition.LastUpdate = order.UpdateTime

			// Recalculate margin and liquidation
			existingPosition.Margin = existingPosition.Size.Mul(existingPosition.EntryPrice).Div(existingPosition.Leverage)
			existingPosition.LiquidationPrice = hs.calculateLiquidationPrice(existingPosition)
		} else {
			// Opposite side - reduce or close position
			if order.FilledSize.GreaterThanOrEqual(existingPosition.Size) {
				// Close position completely
				delete(hs.positions, order.Symbol)
				delete(hs.state.Positions, order.Symbol)
			} else {
				// Reduce position
				existingPosition.Size = existingPosition.Size.Sub(order.FilledSize)
				existingPosition.LastUpdate = order.UpdateTime

				// Recalculate margin and liquidation
				existingPosition.Margin = existingPosition.Size.Mul(existingPosition.EntryPrice).Div(existingPosition.Leverage)
				existingPosition.LiquidationPrice = hs.calculateLiquidationPrice(existingPosition)
			}
		}
	}
}

// calculateLiquidationPrice calculates liquidation price for a position
func (hs *hyperliquidStrategy) calculateLiquidationPrice(position *Position) decimal.Decimal {
	// Simplified liquidation calculation for Hyperliquid
	// Real implementation would use actual Hyperliquid formulas

	maintenanceMarginRate := decimal.NewFromFloat(0.005) // 0.5%

	if position.Side == PositionLong {
		// Long liquidation = entry * (1 - 1/leverage + maintenance_rate)
		factor := decimal.NewFromInt(1).Sub(decimal.NewFromInt(1).Div(position.Leverage)).Add(maintenanceMarginRate)
		return position.EntryPrice.Mul(factor)
	} else {
		// Short liquidation = entry * (1 + 1/leverage - maintenance_rate)
		factor := decimal.NewFromInt(1).Add(decimal.NewFromInt(1).Div(position.Leverage)).Sub(maintenanceMarginRate)
		return position.EntryPrice.Mul(factor)
	}
}

// =============================================================================
// SEGMENT 4: HYPERLIQUID-SPECIFIC STRATEGIES AND UTILITIES (FINAL)
// =============================================================================

// ExecuteFundingArbitrageStrategy implements funding rate arbitrage
func (hs *hyperliquidStrategy) ExecuteFundingArbitrageStrategy() error {
	if !hs.config.EnableArbitrage {
		return nil
	}

	for _, symbol := range hs.config.TradingPairs {
		fundingInfo, exists := hs.fundingData[symbol]
		if !exists {
			continue
		}

		// Check if funding rate is attractive for arbitrage
		absRate := fundingInfo.CurrentRate.Abs()
		if absRate.LessThan(hs.config.FundingThreshold) {
			continue // Not profitable enough
		}

		// Get indicator sentiment from YOUR indicators
		indicatorSignals, err := hs.GetTradingSignals(symbol)
		if err != nil {
			continue
		}

		// Execute funding arbitrage based on rate direction and indicator sentiment
		if err := hs.executeFundingArbitrage(symbol, fundingInfo, indicatorSignals); err != nil {
			hs.logger.Error("Failed to execute funding arbitrage",
				zap.String("symbol", symbol),
				zap.Error(err))
		}
	}

	return nil
}

// executeFundingArbitrage executes a funding arbitrage trade
func (hs *hyperliquidStrategy) executeFundingArbitrage(symbol string, funding FundingInfo, signals TradingSignal) error {
	// Calculate position size for arbitrage (smaller than normal trades)
	baseSize := hs.account.AvailableBalance.Mul(decimal.NewFromFloat(0.01)) // 1% of balance
	positionSize := baseSize.Div(signals.Price)

	var side PositionSide
	var rationale string

	// Funding arbitrage logic
	if funding.CurrentRate.GreaterThan(decimal.Zero) {
		// Positive funding rate - longs pay shorts
		// Go short to collect funding
		side = PositionShort
		rationale = "Collect positive funding rate"
	} else {
		// Negative funding rate - shorts pay longs
		// Go long to collect funding
		side = PositionLong
		rationale = "Collect negative funding rate"
	}

	// Adjust strategy based on YOUR indicator sentiment
	if signals.Signal == "BUY" && side == PositionShort {
		// Indicators suggest bullish but funding suggests short
		// Reduce position size to limit risk
		positionSize = positionSize.Mul(decimal.NewFromFloat(0.5))
	} else if signals.Signal == "SELL" && side == PositionLong {
		// Indicators suggest bearish but funding suggests long
		// Reduce position size to limit risk
		positionSize = positionSize.Mul(decimal.NewFromFloat(0.5))
	}

	// Create funding arbitrage order
	order := &Order{
		ClientOrderID: hs.generateClientOrderID(),
		Symbol:        symbol,
		Side:          side,
		Type:          OrderTypeLimit,
		Size:          positionSize,
		Price:         signals.Price,
		TimeInForce:   "GTD", // Good Till Date (until next funding)
		ReduceOnly:    false,
		PostOnly:      true,
	}

	hs.logger.Info("Executing funding arbitrage",
		zap.String("symbol", symbol),
		zap.String("side", string(side)),
		zap.String("funding_rate", funding.CurrentRate.String()),
		zap.String("rationale", rationale))

	return hs.PlaceOrder(order)
}

// ExecuteGridTradingStrategy implements grid trading strategy
func (hs *hyperliquidStrategy) ExecuteGridTradingStrategy(symbol string) error {
	gridConfig := hs.gridConfigs[symbol]
	if gridConfig == nil || !gridConfig.Enabled {
		return nil
	}

	// Get current market data
	marketData, exists := hs.marketData[symbol]
	if !exists {
		return errors.New("market data not available for " + symbol)
	}

	currentPrice := marketData.Price

	// Check if current price is within grid bounds
	if currentPrice.GreaterThan(gridConfig.UpperBound) || currentPrice.LessThan(gridConfig.LowerBound) {
		return hs.adjustGridBounds(symbol, currentPrice)
	}

	// Get existing grid orders
	existingOrders := hs.getGridOrders(symbol)

	// Calculate required grid levels
	requiredOrders := hs.calculateGridOrders(gridConfig, currentPrice)

	// Cancel orders that are no longer needed
	for _, orderID := range existingOrders {
		if !hs.isOrderStillValid(orderID, requiredOrders) {
			hs.CancelOrder(orderID)
		}
	}

	// Place missing grid orders
	for _, order := range requiredOrders {
		if !hs.hasExistingOrderAtLevel(order.Price, existingOrders) {
			if err := hs.PlaceOrder(order); err != nil {
				hs.logger.Error("Failed to place grid order",
					zap.String("symbol", symbol),
					zap.Error(err))
			}
		}
	}

	return nil
}

// calculateGridOrders calculates all grid orders for current price
func (hs *hyperliquidStrategy) calculateGridOrders(config *GridConfig, currentPrice decimal.Decimal) []*Order {
	orders := make([]*Order, 0)
	spacing := config.GridSpacing.Div(decimal.NewFromInt(100)) // Convert percentage to decimal

	// Calculate buy orders (below current price)
	for i := 1; i <= config.GridLevels/2; i++ {
		buyPrice := currentPrice.Mul(decimal.NewFromInt(1).Sub(spacing.Mul(decimal.NewFromInt(int64(i)))))
		if buyPrice.GreaterThanOrEqual(config.LowerBound) {
			orders = append(orders, &Order{
				ClientOrderID: hs.generateClientOrderID(),
				Symbol:        config.Symbol,
				Side:          PositionLong,
				Type:          OrderTypeLimit,
				Size:          config.OrderSize,
				Price:         buyPrice,
				TimeInForce:   "GTC",
				PostOnly:      true,
			})
		}
	}

	// Calculate sell orders (above current price)
	for i := 1; i <= config.GridLevels/2; i++ {
		sellPrice := currentPrice.Mul(decimal.NewFromInt(1).Add(spacing.Mul(decimal.NewFromInt(int64(i)))))
		if sellPrice.LessThanOrEqual(config.UpperBound) {
			orders = append(orders, &Order{
				ClientOrderID: hs.generateClientOrderID(),
				Symbol:        config.Symbol,
				Side:          PositionShort,
				Type:          OrderTypeLimit,
				Size:          config.OrderSize,
				Price:         sellPrice,
				TimeInForce:   "GTC",
				PostOnly:      true,
			})
		}
	}

	return orders
}

// ExecuteLiquidationHuntingStrategy implements liquidation hunting using YOUR indicators
func (hs *hyperliquidStrategy) ExecuteLiquidationHuntingStrategy() error {
	for _, symbol := range hs.config.TradingPairs {
		if err := hs.executeLiquidationHunting(symbol); err != nil {
			hs.logger.Error("Failed to execute liquidation hunting",
				zap.String("symbol", symbol),
				zap.Error(err))
		}
	}

	return nil
}

// executeLiquidationHunting executes liquidation hunting for a symbol
func (hs *hyperliquidStrategy) executeLiquidationHunting(symbol string) error {
	// Get market data and YOUR indicators
	marketData, exists := hs.marketData[symbol]
	if !exists {
		return errors.New("market data not available")
	}

	signals, err := hs.GetTradingSignals(symbol)
	if err != nil {
		return err
	}

	currentPrice := marketData.Price

	// Use YOUR indicators to identify potential liquidation zones
	snapshot, err := hs.GetIndicatorSnapshot(symbol)
	if err != nil {
		return err
	}

	// Use Bollinger Bands from YOUR indicators to identify potential liquidation zones
	bb := snapshot.BollingerBands
	atr := decimal.NewFromFloat(snapshot.ATR)

	var liquidationZones []decimal.Decimal

	// Potential liquidation zones near Bollinger Bands
	liquidationZones = append(liquidationZones, decimal.NewFromFloat(bb.Lower))
	liquidationZones = append(liquidationZones, decimal.NewFromFloat(bb.Upper))

	// Zones based on ATR levels
	liquidationZones = append(liquidationZones, currentPrice.Sub(atr.Mul(decimal.NewFromFloat(2))))
	liquidationZones = append(liquidationZones, currentPrice.Add(atr.Mul(decimal.NewFromFloat(2))))

	// Check if current price is approaching any liquidation zone
	for _, zone := range liquidationZones {
		distance := currentPrice.Sub(zone).Abs()
		pricePercent := distance.Div(currentPrice).Mul(decimal.NewFromInt(100))

		// If within 1% of a liquidation zone
		if pricePercent.LessThan(decimal.NewFromFloat(1)) {
			return hs.executeLiquidationTrade(symbol, zone, currentPrice, signals)
		}
	}

	return nil
}

// executeLiquidationTrade executes a trade around liquidation zones
func (hs *hyperliquidStrategy) executeLiquidationTrade(symbol string, liquidationZone, currentPrice decimal.Decimal, signals TradingSignal) error {
	var side PositionSide
	var entryPrice decimal.Decimal

	// Determine trade direction based on liquidation zone and YOUR indicators
	if currentPrice.GreaterThan(liquidationZone) {
		// Price above liquidation zone - expect bounce down then up
		if signals.Signal == "SELL" || signals.Confidence.GreaterThan(decimal.NewFromFloat(0.7)) {
			side = PositionShort
			entryPrice = currentPrice.Mul(decimal.NewFromFloat(0.999)) // Slightly below current
		} else {
			return nil // Don't trade against strong bullish signals
		}
	} else {
		// Price below liquidation zone - expect bounce up then down
		if signals.Signal == "BUY" || signals.Confidence.GreaterThan(decimal.NewFromFloat(0.7)) {
			side = PositionLong
			entryPrice = currentPrice.Mul(decimal.NewFromFloat(1.001)) // Slightly above current
		} else {
			return nil // Don't trade against strong bearish signals
		}
	}

	// Calculate position size (smaller for liquidation hunting)
	baseSize := hs.account.AvailableBalance.Mul(decimal.NewFromFloat(0.015)) // 1.5% of balance
	positionSize := baseSize.Div(entryPrice)

	order := &Order{
		ClientOrderID: hs.generateClientOrderID(),
		Symbol:        symbol,
		Side:          side,
		Type:          OrderTypeLimit,
		Size:          positionSize,
		Price:         entryPrice,
		TimeInForce:   "GTC",
		PostOnly:      true,
	}

	hs.logger.Info("Executing liquidation hunting trade",
		zap.String("symbol", symbol),
		zap.String("side", string(side)),
		zap.String("liquidation_zone", liquidationZone.String()),
		zap.String("entry_price", entryPrice.String()))

	return hs.PlaceOrder(order)
}

// executePositionManagement manages existing positions
func (hs *hyperliquidStrategy) executePositionManagement(symbol string) error {
	position := hs.getPosition(symbol)
	if position == nil {
		return nil // No position to manage
	}

	// Update position with current market price
	if marketData, exists := hs.marketData[symbol]; exists {
		position.CurrentPrice = marketData.Price
		position.UnrealizedPnL = hs.calculateUnrealizedPnL(position)
		position.LastUpdate = time.Now()
	}

	// Get current signals from YOUR indicators
	signals, err := hs.GetTradingSignals(symbol)
	if err != nil {
		return err
	}

	// Check for position management actions

	// 1. Check stop loss
	if hs.shouldTriggerStopLoss(position, signals) {
		return hs.ClosePosition(symbol)
	}

	// 2. Check take profit
	if hs.shouldTriggerTakeProfit(position, signals) {
		return hs.ClosePosition(symbol)
	}

	// 3. Check liquidation risk
	if hs.isNearLiquidation(position) {
		hs.logger.Warn("Position near liquidation",
			zap.String("symbol", symbol),
			zap.String("liquidation_price", position.LiquidationPrice.String()),
			zap.String("current_price", position.CurrentPrice.String()))

		// Consider reducing position size
		return hs.reducePositionSize(position)
	}

	return nil
}

// shouldTriggerStopLoss checks if stop loss should be triggered
func (hs *hyperliquidStrategy) shouldTriggerStopLoss(position *Position, signals TradingSignal) bool {
	// Calculate current loss percentage
	var lossPercent decimal.Decimal

	if position.Side == PositionLong {
		if position.CurrentPrice.LessThan(position.EntryPrice) {
			lossPercent = position.EntryPrice.Sub(position.CurrentPrice).Div(position.EntryPrice)
		}
	} else {
		if position.CurrentPrice.GreaterThan(position.EntryPrice) {
			lossPercent = position.CurrentPrice.Sub(position.EntryPrice).Div(position.EntryPrice)
		}
	}

	// Trigger stop loss if loss exceeds configured percentage
	if lossPercent.GreaterThan(hs.config.StopLossPercent) {
		return true
	}

	// Also check if YOUR indicators strongly suggest opposite direction
	if position.Side == PositionLong && signals.Signal == "SELL" && signals.Confidence.GreaterThan(decimal.NewFromFloat(0.8)) {
		return true
	}

	if position.Side == PositionShort && signals.Signal == "BUY" && signals.Confidence.GreaterThan(decimal.NewFromFloat(0.8)) {
		return true
	}

	return false
}

// shouldTriggerTakeProfit checks if take profit should be triggered
func (hs *hyperliquidStrategy) shouldTriggerTakeProfit(position *Position, signals TradingSignal) bool {
	// Calculate current profit percentage
	var profitPercent decimal.Decimal

	if position.Side == PositionLong {
		if position.CurrentPrice.GreaterThan(position.EntryPrice) {
			profitPercent = position.CurrentPrice.Sub(position.EntryPrice).Div(position.EntryPrice)
		}
	} else {
		if position.CurrentPrice.LessThan(position.EntryPrice) {
			profitPercent = position.EntryPrice.Sub(position.CurrentPrice).Div(position.EntryPrice)
		}
	}

	// Trigger take profit if profit exceeds configured percentage
	if profitPercent.GreaterThan(hs.config.TakeProfitPercent) {
		return true
	}

	return false
}

// isNearLiquidation checks if position is near liquidation
func (hs *hyperliquidStrategy) isNearLiquidation(position *Position) bool {
	if position.LiquidationPrice.LessThanOrEqual(decimal.Zero) {
		return false
	}

	var distanceToLiquidation decimal.Decimal

	if position.Side == PositionLong {
		distanceToLiquidation = position.CurrentPrice.Sub(position.LiquidationPrice).Div(position.CurrentPrice)
	} else {
		distanceToLiquidation = position.LiquidationPrice.Sub(position.CurrentPrice).Div(position.CurrentPrice)
	}

	// Return true if within 20% of liquidation
	return distanceToLiquidation.LessThan(decimal.NewFromFloat(0.2))
}

// reducePositionSize reduces position size when near liquidation
func (hs *hyperliquidStrategy) reducePositionSize(position *Position) error {
	// Reduce position by 50%
	reduceSize := position.Size.Mul(decimal.NewFromFloat(0.5))

	var closingSide PositionSide
	if position.Side == PositionLong {
		closingSide = PositionShort
	} else {
		closingSide = PositionLong
	}

	order := &Order{
		ClientOrderID: hs.generateClientOrderID(),
		Symbol:        position.Symbol,
		Side:          closingSide,
		Type:          OrderTypeMarket,
		Size:          reduceSize,
		Price:         position.CurrentPrice,
		TimeInForce:   "IOC", // Immediate or Cancel
		ReduceOnly:    true,
	}

	return hs.PlaceOrder(order)
}

// =============================================================================
// UTILITY FUNCTIONS AND HELPERS
// =============================================================================

// ClosePosition implements HyperliquidStrategy.ClosePosition
func (hs *hyperliquidStrategy) ClosePosition(symbol string) error {
	hs.mu.Lock()
	position := hs.positions[symbol]
	hs.mu.Unlock()

	if position == nil {
		return errors.New("no position found for symbol: " + symbol)
	}

	// Create closing order (opposite side)
	var closingSide PositionSide
	if position.Side == PositionLong {
		closingSide = PositionShort
	} else {
		closingSide = PositionLong
	}

	order := &Order{
		ClientOrderID: hs.generateClientOrderID(),
		Symbol:        symbol,
		Side:          closingSide,
		Type:          OrderTypeMarket,
		Size:          position.Size,
		Price:         position.CurrentPrice,
		TimeInForce:   "IOC", // Immediate or Cancel
		ReduceOnly:    true,
	}

	return hs.PlaceOrder(order)
}

// CancelOrder implements HyperliquidStrategy.CancelOrder
func (hs *hyperliquidStrategy) CancelOrder(orderID string) error {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	order, exists := hs.orders[orderID]
	if !exists {
		return errors.New("order not found: " + orderID)
	}

	order.Status = "CANCELLED"
	order.UpdateTime = time.Now()

	delete(hs.orders, orderID)
	delete(hs.state.Orders, orderID)

	hs.logger.Info("Order cancelled", zap.String("id", orderID))
	return nil
}

// GetPositions implements HyperliquidStrategy.GetPositions
func (hs *hyperliquidStrategy) GetPositions() map[string]*Position {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	positions := make(map[string]*Position)
	for k, v := range hs.positions {
		positions[k] = v
	}
	return positions
}

// GetOrders implements HyperliquidStrategy.GetOrders
func (hs *hyperliquidStrategy) GetOrders() map[string]*Order {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	orders := make(map[string]*Order)
	for k, v := range hs.orders {
		orders[k] = v
	}
	return orders
}

// GetPortfolioValue implements HyperliquidStrategy.GetPortfolioValue
func (hs *hyperliquidStrategy) GetPortfolioValue() decimal.Decimal {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	return hs.account.TotalBalance.Add(hs.account.TotalUnrealizedPnL)
}

// GetAccountInfo implements HyperliquidStrategy.GetAccountInfo
func (hs *hyperliquidStrategy) GetAccountInfo() AccountInfo {
	hs.mu.RLock()
	defer hs.mu.RUnlock()
	return hs.account
}

// ExecutePerpetualFuturesStrategy implements HyperliquidStrategy.ExecutePerpetualFuturesStrategy
func (hs *hyperliquidStrategy) ExecutePerpetualFuturesStrategy(symbol string) error {
	return hs.executeSymbolStrategy(symbol)
}

// RebalancePortfolio implements HyperliquidStrategy.RebalancePortfolio
func (hs *hyperliquidStrategy) RebalancePortfolio() error {
	// TODO: Implement portfolio rebalancing logic
	return nil
}

// UpdateConfig implements HyperliquidStrategy.UpdateConfig
func (hs *hyperliquidStrategy) UpdateConfig(config HyperliquidConfig) error {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hs.config = config
	return nil
}

// GetState implements HyperliquidStrategy.GetState
func (hs *hyperliquidStrategy) GetState() StrategyState {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	// Create a copy of the state
	state := *hs.state
	state.Positions = make(map[string]*Position)
	state.Orders = make(map[string]*Order)

	for k, v := range hs.state.Positions {
		state.Positions[k] = v
	}

	for k, v := range hs.state.Orders {
		state.Orders[k] = v
	}

	return state
}

// Reset implements HyperliquidStrategy.Reset
func (hs *hyperliquidStrategy) Reset() error {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	// Reset all indicator engines
	for _, engine := range hs.indicatorEngines {
		engine.Reset()
	}

	// Clear all positions and orders
	hs.positions = make(map[string]*Position)
	hs.orders = make(map[string]*Order)
	hs.tradeHistory = make([]Trade, 0)
	hs.performanceLog = make([]PerformanceSnapshot, 0)

	// Reset state
	hs.state.Positions = make(map[string]*Position)
	hs.state.Orders = make(map[string]*Order)
	hs.state.Signals = make([]TradingSignal, 0)
	hs.state.TotalTrades = 0
	hs.state.WinningTrades = 0
	hs.state.LosingTrades = 0
	hs.state.TotalPnL = decimal.Zero
	hs.state.TotalFees = decimal.Zero
	hs.state.MaxDrawdown = decimal.Zero
	hs.state.CurrentDrawdown = decimal.Zero

	hs.logger.Info("Strategy reset completed")
	return nil
}

// updateAccountInfo updates account information
func (hs *hyperliquidStrategy) updateAccountInfo() error {
	// In real implementation, this would call Hyperliquid API
	// For now, calculate from positions

	totalUnrealizedPnL := decimal.Zero
	usedMargin := decimal.Zero

	for _, position := range hs.positions {
		totalUnrealizedPnL = totalUnrealizedPnL.Add(position.UnrealizedPnL)
		usedMargin = usedMargin.Add(position.Margin)
	}

	hs.mu.Lock()
	hs.account.TotalUnrealizedPnL = totalUnrealizedPnL
	hs.account.UsedMargin = usedMargin
	hs.account.FreeMargin = hs.account.TotalBalance.Sub(usedMargin)
	hs.account.PositionCount = len(hs.positions)
	hs.account.OrderCount = len(hs.orders)
	hs.account.LastUpdate = time.Now()
	hs.mu.Unlock()

	return nil
}

// Helper functions
func (hs *hyperliquidStrategy) generateOrderID() string {
	return fmt.Sprintf("order_%d_%d", time.Now().Unix(), time.Now().UnixNano()%1000000)
}

func (hs *hyperliquidStrategy) generateClientOrderID() string {
	return fmt.Sprintf("client_%d_%d", time.Now().Unix(), time.Now().UnixNano()%1000000)
}

func (hs *hyperliquidStrategy) generatePositionID() string {
	return fmt.Sprintf("pos_%d_%d", time.Now().Unix(), time.Now().UnixNano()%1000000)
}

func (hs *hyperliquidStrategy) getPosition(symbol string) *Position {
	hs.mu.RLock()
	defer hs.mu.RUnlock()
	return hs.positions[symbol]
}

func (hs *hyperliquidStrategy) calculateUnrealizedPnL(position *Position) decimal.Decimal {
	if position.Side == PositionLong {
		return position.Size.Mul(position.CurrentPrice.Sub(position.EntryPrice))
	} else {
		return position.Size.Mul(position.EntryPrice.Sub(position.CurrentPrice))
	}
}

func (hs *hyperliquidStrategy) calculateDailyPnL() decimal.Decimal {
	// Calculate P&L for today
	today := time.Now().Truncate(24 * time.Hour)
	dailyPnL := decimal.Zero

	for _, trade := range hs.tradeHistory {
		if trade.CloseTime.After(today) {
			dailyPnL = dailyPnL.Add(trade.PnL)
		}
	}

	// Add unrealized PnL from current positions
	for _, position := range hs.positions {
		if position.OpenTime.After(today) {
			dailyPnL = dailyPnL.Add(position.UnrealizedPnL)
		}
	}

	return dailyPnL
}

func (hs *hyperliquidStrategy) calculateWinRate() decimal.Decimal {
	if len(hs.tradeHistory) == 0 {
		return decimal.Zero
	}

	winningTrades := 0
	for _, trade := range hs.tradeHistory {
		if trade.PnL.GreaterThan(decimal.Zero) {
			winningTrades++
		}
	}

	return decimal.NewFromInt(int64(winningTrades)).Div(decimal.NewFromInt(int64(len(hs.tradeHistory))))
}

func (hs *hyperliquidStrategy) updatePerformanceMetrics() {
	snapshot := PerformanceSnapshot{
		Timestamp:       time.Now(),
		PortfolioValue:  hs.GetPortfolioValue(),
		TotalPnL:        hs.account.TotalRealizedPnL.Add(hs.account.TotalUnrealizedPnL),
		DailyPnL:        hs.calculateDailyPnL(),
		WinRate:         hs.calculateWinRate(),
		ActivePositions: len(hs.positions),
		TotalTrades:     len(hs.tradeHistory),
	}

	hs.mu.Lock()
	hs.performanceLog = append(hs.performanceLog, snapshot)

	// Keep only last 1000 snapshots
	if len(hs.performanceLog) > 1000 {
		hs.performanceLog = hs.performanceLog[1:]
	}
	hs.mu.Unlock()
}

func (hs *hyperliquidStrategy) initializeGridConfig(symbol string) {
	// Initialize default grid configuration
	gridConfig := &GridConfig{
		Symbol:       symbol,
		GridLevels:   10,
		GridSpacing:  decimal.NewFromFloat(0.5),  // 0.5% spacing
		OrderSize:    decimal.NewFromFloat(100),  // $100 per order
		TotalCapital: decimal.NewFromFloat(1000), // $1000 total
		MaxPositions: 5,
		Enabled:      true,
	}

	hs.gridConfigs[symbol] = gridConfig
}

// Grid trading helper functions (simplified implementations)
func (hs *hyperliquidStrategy) adjustGridBounds(symbol string, currentPrice decimal.Decimal) error {
	// TODO: Implement grid bounds adjustment
	return nil
}

func (hs *hyperliquidStrategy) getGridOrders(symbol string) []string {
	return hs.gridOrders[symbol]
}

func (hs *hyperliquidStrategy) isOrderStillValid(orderID string, requiredOrders []*Order) bool {
	// TODO: Implement order validation logic
	return true
}

func (hs *hyperliquidStrategy) hasExistingOrderAtLevel(price decimal.Decimal, existingOrders []string) bool {
	// TODO: Implement level checking logic
	return false
}
