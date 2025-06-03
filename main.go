package main

import (
	"context"
	"strings"
	"github.com/joho/godotenv" // For loading .env files
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"modular-trading-bot/execution"
	"modular-trading-bot/indicators"
	"modular-trading-bot/marketdata"
	"modular-trading-bot/strategy"

	"github.com/shopspring/decimal"
)

// HYPE-USDC Trading Bot Configuration
type HypeTradingBot struct {
	// Core engines
	marketData marketdata.MarketDataEngine
	indicators indicators.IndicatorEngine
	strategy   strategy.StrategyEngine
	execution  *execution.HyperliquidEngine

	// Configuration
	config *HypeTradingConfig

	// State management
	lastPrice float64
	position  *execution.Position
	stateMu   sync.RWMutex

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	stopCh chan struct{}

	// Performance tracking
	startTime   time.Time
	totalTrades int64
	totalPnL    float64
	perfMu      sync.RWMutex
}

type HypeTradingConfig struct {
	// Hyperliquid Configuration
	PrivateKeyHex string `json:"private_key_hex"`
	IsMainnet     bool   `json:"is_mainnet"`

	// Trading Configuration
	TradingSymbol string `json:"trading_symbol"`
	BaseAsset     string `json:"base_asset"`
	QuoteAsset    string `json:"quote_asset"`

	// Risk Management
	MaxPositionSize   float64 `json:"max_position_size"`
	MaxLeverage       float64 `json:"max_leverage"`
	StopLossPercent   float64 `json:"stop_loss_percent"`
	TakeProfitPercent float64 `json:"take_profit_percent"`

	// Strategy Parameters
	TrendFollowingEnabled bool `json:"trend_following_enabled"`
	MeanReversionEnabled  bool `json:"mean_reversion_enabled"`
	ScalpingEnabled       bool `json:"scalping_enabled"`

	// Position Sizing
	PositionSizePercent float64 `json:"position_size_percent"`
	MinOrderSize        float64 `json:"min_order_size"`
	MaxOrderSize        float64 `json:"max_order_size"`

	// Timing
	EntryTimeframe string `json:"entry_timeframe"`
	ExitTimeframe  string `json:"exit_timeframe"`

	// Advanced Features
	DynamicLeverage         bool `json:"dynamic_leverage"`
	FundingRateOptimization bool `json:"funding_rate_optimization"`
	MEVProtection           bool `json:"mev_protection"`

	// Logging and Monitoring
	EnableLogging    bool   `json:"enable_logging"`
	EnableTelegram   bool   `json:"enable_telegram"`
	TelegramBotToken string `json:"telegram_bot_token,omitempty"`
	TelegramChatID   string `json:"telegram_chat_id,omitempty"`
}

// Default configuration for HYPE-USDC trading
func DefaultHypeConfig() *HypeTradingConfig {
	return &HypeTradingConfig{
		IsMainnet:               true,
		TradingSymbol:           "HYPE",
		BaseAsset:               "HYPE",
		QuoteAsset:              "USDC",
		MaxPositionSize:         5000, // Conservative $5k max position
		MaxLeverage:             5,    // Conservative 5x max leverage
		StopLossPercent:         2.0,  // 2% stop loss
		TakeProfitPercent:       4.0,  // 4% take profit
		TrendFollowingEnabled:   true,
		MeanReversionEnabled:    true,
		ScalpingEnabled:         false, // Disabled by default for conservative approach
		PositionSizePercent:     25,    // 25% of account per trade
		MinOrderSize:            10,    // $10 minimum
		MaxOrderSize:            1000,  // $1k maximum single order (conservative)
		EntryTimeframe:          "1m",
		ExitTimeframe:           "30s",
		DynamicLeverage:         true,
		FundingRateOptimization: true,
		MEVProtection:           true,
		EnableLogging:           true,
	}
}

// Create new HYPE-USDC trading bot
func NewHypeTradingBot(config *HypeTradingConfig) (*HypeTradingBot, error) {
	if config.PrivateKeyHex == "" {
		return nil, fmt.Errorf("private key is required")
	}

	log.Printf("🚀 Initializing HYPE-USDC Trading Bot...")

	ctx, cancel := context.WithCancel(context.Background())

	// 1. Initialize Market Data Engine
	mdConfig := marketdata.DefaultConfig
	mdConfig.EnableWebSocket = true
	mdConfig.EnableCache = true
	mdConfig.Symbols = []string{config.TradingSymbol}
	mdConfig.EnableLogging = config.EnableLogging

	marketDataEngine, err := marketdata.NewMarketDataEngine(mdConfig)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create market data engine: %w", err)
	}

	// 2. Initialize Indicators Engine
	indicatorConfig := indicators.DefaultConfig()
	indicatorConfig.EnableLogging = config.EnableLogging
	indicatorConfig.RSIPeriod = 14
	indicatorConfig.MACDConfig.FastPeriod = 12
	indicatorConfig.MACDConfig.SlowPeriod = 26
	indicatorConfig.MACDConfig.SignalPeriod = 9
	indicatorConfig.BBConfig.Period = 20
	indicatorConfig.BBConfig.StdDev = 2.0

	indicatorEngine := indicators.NewIndicatorEngine(indicatorConfig)

	// 3. Initialize Strategy Engine
	strategyConfig := strategy.DefaultHyperliquidConfig()
	strategyConfig.InitialBalance = decimal.NewFromFloat(config.MaxPositionSize * 4) // Assume 4x the max position
	strategyConfig.EnableLogging = config.EnableLogging

	// Configure enabled strategies
	var enabledStrategies []string
	if config.TrendFollowingEnabled {
		enabledStrategies = append(enabledStrategies, "trend_following")
	}
	if config.MeanReversionEnabled {
		enabledStrategies = append(enabledStrategies, "mean_reversion")
	}
	if config.ScalpingEnabled {
		enabledStrategies = append(enabledStrategies, "scalping")
	}
	var typedEnabledStrategies []strategy.StrategyType
	for _, s := range enabledStrategies {
		switch strings.ToUpper(s) { // Assuming strategy names from config match constants after ToUpper
		case string(strategy.StrategyPerpetualFutures):
			typedEnabledStrategies = append(typedEnabledStrategies, strategy.StrategyPerpetualFutures)
		case string(strategy.StrategyFundingArbitrage):
			typedEnabledStrategies = append(typedEnabledStrategies, strategy.StrategyFundingArbitrage)
		case string(strategy.StrategyLiquidationHunting):
			typedEnabledStrategies = append(typedEnabledStrategies, strategy.StrategyLiquidationHunting)
		case string(strategy.StrategyGridTrading):
			typedEnabledStrategies = append(typedEnabledStrategies, strategy.StrategyGridTrading)
		case string(strategy.StrategyMomentumBreakout), "TREND_FOLLOWING": // Handles "MOMENTUM_BREAKOUT" or "TREND_FOLLOWING"
			typedEnabledStrategies = append(typedEnabledStrategies, strategy.StrategyMomentumBreakout)
		case string(strategy.StrategyMeanReversion): // Handles "MEAN_REVERSION" (which is what ToUpper("mean_reversion") becomes)
			typedEnabledStrategies = append(typedEnabledStrategies, strategy.StrategyMeanReversion)
		case string(strategy.StrategyDeltaNeutral):
			typedEnabledStrategies = append(typedEnabledStrategies, strategy.StrategyDeltaNeutral)
		// SCALPING is not a direct StrategyType, map to a relevant one or log warning
		case "SCALPING":
			// For now, let's assume scalping might use momentum or mean reversion logic
			// Or, you might want to define StrategyScalping if it's distinct enough.
			// Adding to MomentumBreakout as a placeholder.
			log.Printf("Warning: 'SCALPING' strategy mapped to 'MOMENTUM_BREAKOUT'. Consider defining a specific StrategyType for scalping.")
			typedEnabledStrategies = append(typedEnabledStrategies, strategy.StrategyMomentumBreakout) 
		default:
			log.Printf("Warning: Unknown strategy string '%s' in config, skipping.", s)
		}
	}
	strategyConfig.EnabledStrategies = typedEnabledStrategies

	strategyEngine := strategy.NewStrategyEngine(strategyConfig, indicatorEngine)

	// 4. Initialize Execution Engine
	executionConfig := execution.DefaultConfig()
	executionConfig.PrivateKeyHex = config.PrivateKeyHex
	executionConfig.IsMainnet = config.IsMainnet
	executionConfig.EnableLogging = config.EnableLogging

	executionEngine, err := execution.NewHyperliquidEngine(executionConfig)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create execution engine: %w", err)
	}

	// Set risk limits
	riskLimits := execution.RiskLimits{
		MaxPositionSize:  config.MaxPositionSize,
		MaxLeverage:      config.MaxLeverage,
		MaxDailyLoss:     config.MaxPositionSize * 0.1, // 10% of max position as daily loss limit
		MaxDrawdown:      0.15,                         // 15% max drawdown
		StopLossRequired: true,
	}
	executionEngine.SetRiskLimits(riskLimits)

	bot := &HypeTradingBot{
		marketData: marketDataEngine,
		indicators: indicatorEngine,
		strategy:   strategyEngine,
		execution:  executionEngine,
		config:     config,
		ctx:        ctx,
		cancel:     cancel,
		stopCh:     make(chan struct{}),
		startTime:  time.Now(),
	}

	// Set up interconnections
	bot.setupConnections()

	return bot, nil
}

// Set up connections between modules
func (bot *HypeTradingBot) setupConnections() {
	log.Printf("🔗 Setting up module interconnections...")

	// Market Data -> Indicators connection
	bot.marketData.SetPriceCallback(func(symbol string, price float64, timestamp time.Time) {
		if symbol != bot.config.TradingSymbol {
			return
		}

		bot.stateMu.Lock()
		bot.lastPrice = price
		bot.stateMu.Unlock()

		// Update indicators with new price
		if err := bot.indicators.UpdatePrice(price); err != nil && bot.config.EnableLogging {
			log.Printf("⚠️  Failed to update indicators: %v", err)
		}

		// Update strategy with price
		if err := bot.strategy.ProcessPrice(symbol, price, timestamp); err != nil && bot.config.EnableLogging {
			log.Printf("⚠️  Failed to update strategy: %v", err)
		}
	})

	// Indicators -> Strategy connection
	bot.indicators.SetSignalCallback(func(signals indicators.TradingSignals) {
		// Process signals in strategy engine
		if err := bot.strategy.ProcessSignal(signals); err != nil && bot.config.EnableLogging {
			log.Printf("⚠️  Failed to process signals: %v", err)
		}
	})

	// Strategy -> Execution connection
	bot.strategy.SetExecutionCallback(func(strategyOrder strategy.ExecutionOrder) error {
		return bot.handleStrategyOrder(strategyOrder)
	})

	// Execution callbacks
	bot.execution.SetOrderUpdateCallback(func(update execution.OrderUpdate) {
		bot.handleOrderUpdate(update)
	})

	bot.execution.SetPositionUpdateCallback(func(position execution.Position) {
		bot.handlePositionUpdate(position)
	})

	bot.execution.SetAccountUpdateCallback(func(account execution.AccountInfo) {
		bot.handleAccountUpdate(account)
	})

	// Strategy notifications
	bot.strategy.SetNotificationCallback(func(notification strategy.StrategyNotification) {
		bot.handleStrategyNotification(notification)
	})
}

// Handle strategy order execution
func (bot *HypeTradingBot) handleStrategyOrder(strategyOrder strategy.ExecutionOrder) error {
	if bot.config.EnableLogging {
		log.Printf("📋 Strategy order received: %s %s %.4f @ $%.4f (Leverage: %.1fx)",
			strategyOrder.Side, strategyOrder.Symbol, strategyOrder.Quantity,
			strategyOrder.Price, strategyOrder.Leverage)
	}

	// Validate symbol
	if strategyOrder.Symbol != bot.config.TradingSymbol {
		return fmt.Errorf("invalid symbol: %s (expected %s)", strategyOrder.Symbol, bot.config.TradingSymbol)
	}

	// Apply position sizing rules
	adjustedOrder := bot.applyPositionSizing(strategyOrder)

	// Apply dynamic leverage if enabled
	if bot.config.DynamicLeverage {
		adjustedOrder = bot.applyDynamicLeverage(adjustedOrder)
	}

	// Apply MEV protection if enabled
	if bot.config.MEVProtection {
		adjustedOrder = bot.applyMEVProtection(adjustedOrder)
	}

	// Convert to execution order
	executionOrder := bot.convertToExecutionOrder(adjustedOrder)

	// Add stop loss and take profit orders
	mainResult, err := bot.execution.PlaceOrder(executionOrder)
	if err != nil {
		if bot.config.EnableLogging {
			log.Printf("❌ Failed to place main order: %v", err)
		}
		return err
	}

	// Place stop loss order
	if bot.config.StopLossPercent > 0 {
		go bot.placeStopLossOrder(executionOrder, mainResult)
	}

	// Place take profit order
	if bot.config.TakeProfitPercent > 0 {
		go bot.takeProfitOrder(executionOrder, mainResult)
	}

	// Update performance tracking
	bot.perfMu.Lock()
	bot.totalTrades++
	bot.perfMu.Unlock()

	if bot.config.EnableLogging {
		log.Printf("✅ Order executed successfully: %s (Latency: %v)",
			mainResult.OrderID, mainResult.Latency)
	}

	return nil
}

// Apply position sizing rules
func (bot *HypeTradingBot) applyPositionSizing(order strategy.ExecutionOrder) strategy.ExecutionOrder {
	// Get current account balance
	accountInfo, err := bot.execution.GetAccountInfo()
	if err != nil {
		if bot.config.EnableLogging {
			log.Printf("⚠️  Failed to get account info for position sizing: %v", err)
		}
		return order
	}

	// Calculate position size based on percentage of account
	accountBalance := accountInfo.AvailableBalance
	maxPositionValue := accountBalance * (bot.config.PositionSizePercent / 100.0)

	// Apply maximum position size limit
	if maxPositionValue > bot.config.MaxPositionSize {
		maxPositionValue = bot.config.MaxPositionSize
	}

	// Calculate quantity based on current price and leverage
	bot.stateMu.RLock()
	currentPrice := bot.lastPrice
	bot.stateMu.RUnlock()

	if currentPrice <= 0 {
		currentPrice = order.Price
	}

	// Calculate maximum quantity
	maxQuantity := (maxPositionValue * order.Leverage) / currentPrice

	// Apply order size limits
	orderValue := order.Quantity * currentPrice / order.Leverage
	if orderValue < bot.config.MinOrderSize {
		order.Quantity = (bot.config.MinOrderSize * order.Leverage) / currentPrice
	} else if orderValue > bot.config.MaxOrderSize {
		order.Quantity = (bot.config.MaxOrderSize * order.Leverage) / currentPrice
	}

	// Ensure we don't exceed maximum quantity
	if order.Quantity > maxQuantity {
		order.Quantity = maxQuantity
	}

	return order
}

// Apply dynamic leverage based on market conditions
func (bot *HypeTradingBot) applyDynamicLeverage(order strategy.ExecutionOrder) strategy.ExecutionOrder {
	// Get current market volatility from indicators
	signals := bot.indicators.GetSignals()

	// Reduce leverage in high volatility conditions
	volatilityMultiplier := 1.0
	switch signals.Volatility {
	case indicators.VolatilityLow:
		volatilityMultiplier = 1.2 // Increase leverage in low volatility
	case indicators.VolatilityHigh:
		volatilityMultiplier = 0.6 // Reduce leverage in high volatility

	default:
		volatilityMultiplier = 1.0 // Normal volatility
	}

	// Adjust leverage based on signal confidence
	confidenceMultiplier := math.Min(signals.Confidence*1.5, 1.0)

	// Apply multipliers
	originalLeverage := order.Leverage
	newLeverage := order.Leverage * volatilityMultiplier * confidenceMultiplier

	// Ensure we don't exceed maximum leverage
	if newLeverage > bot.config.MaxLeverage {
		newLeverage = bot.config.MaxLeverage
	}

	// Ensure minimum leverage of 1x
	if newLeverage < 1.0 {
		newLeverage = 1.0
	}

	order.Leverage = newLeverage

	if bot.config.EnableLogging && newLeverage != originalLeverage {
		log.Printf("🎛️  Dynamic leverage applied: %.1fx -> %.1fx (Volatility: %v, Confidence: %.2f)",
			originalLeverage, newLeverage, signals.Volatility, signals.Confidence)
	}

	return order
}

// Apply MEV protection
func (bot *HypeTradingBot) applyMEVProtection(order strategy.ExecutionOrder) strategy.ExecutionOrder {
	// Add small random delay to prevent front-running
	delay := time.Duration(50+int64(time.Now().UnixNano()%100)) * time.Millisecond
	time.Sleep(delay)

	// Add slight price improvement for limit orders
	if order.OrderType == "limit" {
		priceImprovement := 0.0001 // 0.01% price improvement
		if order.Side == "buy" {
			order.Price *= (1 + priceImprovement)
		} else {
			order.Price *= (1 - priceImprovement)
		}
	}

	return order
}

// Convert strategy order to execution order
func (bot *HypeTradingBot) convertToExecutionOrder(strategyOrder strategy.ExecutionOrder) execution.Order {
	var side execution.OrderSide
	if strategyOrder.Side == "buy" {
		side = execution.OrderSideBuy
	} else {
		side = execution.OrderSideSell
	}

	var orderType execution.OrderType
	switch strategyOrder.OrderType {
	case "market":
		orderType = execution.OrderTypeMarket
	case "limit":
		orderType = execution.OrderTypeLimit
	default:
		orderType = execution.OrderTypeLimit // Default to limit
	}

	return execution.Order{
		Symbol:      strategyOrder.Symbol,
		Side:        side,
		Type:        orderType,
		Quantity:    strategyOrder.Quantity,
		Price:       strategyOrder.Price,
		Leverage:    strategyOrder.Leverage,
		TimeInForce: "GTC",                              // Good Till Cancelled
		PostOnly:    strategyOrder.OrderType == "limit", // Post-only for limit orders
	}
}

// Place stop loss order
func (bot *HypeTradingBot) placeStopLossOrder(mainOrder execution.Order, mainResult *execution.OrderResult) {
	time.Sleep(100 * time.Millisecond) // Small delay to ensure main order is processed

	var stopPrice float64
	var side execution.OrderSide

	if mainOrder.Side == execution.OrderSideBuy {
		// For long position, stop loss is below entry price
		stopPrice = mainOrder.Price * (1 - bot.config.StopLossPercent/100.0)
		side = execution.OrderSideSell
	} else {
		// For short position, stop loss is above entry price
		stopPrice = mainOrder.Price * (1 + bot.config.StopLossPercent/100.0)
		side = execution.OrderSideBuy
	}

	stopOrder := execution.Order{
		Symbol:     mainOrder.Symbol,
		Side:       side,
		Type:       execution.OrderTypeStopLoss,
		Quantity:   mainOrder.Quantity,
		StopPrice:  stopPrice,
		Leverage:   1, // Market order when triggered
		ReduceOnly: true,
	}

	result, err := bot.execution.PlaceOrder(stopOrder)
	if err != nil {
		if bot.config.EnableLogging {
			log.Printf("❌ Failed to place stop loss order: %v", err)
		}
		return
	}

	if bot.config.EnableLogging {
		log.Printf("🛡️  Stop loss placed: %.4f @ $%.4f (Order ID: %s)",
			stopOrder.Quantity, stopPrice, result.OrderID)
	}
}

// Place take profit order
func (bot *HypeTradingBot) takeProfitOrder(mainOrder execution.Order, mainResult *execution.OrderResult) {
	time.Sleep(100 * time.Millisecond) // Small delay to ensure main order is processed

	var takeProfitPrice float64
	var side execution.OrderSide

	if mainOrder.Side == execution.OrderSideBuy {
		// For long position, take profit is above entry price
		takeProfitPrice = mainOrder.Price * (1 + bot.config.TakeProfitPercent/100.0)
		side = execution.OrderSideSell
	} else {
		// For short position, take profit is below entry price
		takeProfitPrice = mainOrder.Price * (1 - bot.config.TakeProfitPercent/100.0)
		side = execution.OrderSideBuy
	}

	takeProfitOrder := execution.Order{
		Symbol:     mainOrder.Symbol,
		Side:       side,
		Type:       execution.OrderTypeTakeProfit,
		Quantity:   mainOrder.Quantity,
		Price:      takeProfitPrice,
		Leverage:   1, // Market order when triggered
		ReduceOnly: true,
	}

	result, err := bot.execution.PlaceOrder(takeProfitOrder)
	if err != nil {
		if bot.config.EnableLogging {
			log.Printf("❌ Failed to place take profit order: %v", err)
		}
		return
	}

	if bot.config.EnableLogging {
		log.Printf("🎯 Take profit placed: %.4f @ $%.4f (Order ID: %s)",
			takeProfitOrder.Quantity, takeProfitPrice, result.OrderID)
	}
}

// Handle order updates
func (bot *HypeTradingBot) handleOrderUpdate(update execution.OrderUpdate) {
	if bot.config.EnableLogging {
		log.Printf("📊 Order Update: %s [%s -> %s] Filled: %.4f @ $%.4f",
			update.OrderID, update.OldStatus, update.NewStatus,
			update.FilledQty, update.AvgPrice)
	}

	// Update strategy with execution feedback
	if update.NewStatus == execution.OrderStatusFilled {
		// Calculate PnL if this is a closing order
		bot.calculateAndUpdatePnL(update)
	}
}

// Handle position updates
func (bot *HypeTradingBot) handlePositionUpdate(position execution.Position) {
	bot.stateMu.Lock()
	bot.position = &position
	bot.stateMu.Unlock()

	if bot.config.EnableLogging {
		log.Printf("📈 Position Update: %s %s %.4f @ $%.4f (PnL: $%.2f)",
			position.Symbol, position.Side, position.Size,
			position.EntryPrice, position.UnrealizedPnL)
	}

	// Check for liquidation risk
	if position.LiqPrice > 0 {
		bot.stateMu.RLock()
		currentPrice := bot.lastPrice
		bot.stateMu.RUnlock()

		var liquidationRisk float64
		if position.Side == "long" {
			liquidationRisk = (currentPrice - position.LiqPrice) / currentPrice
		} else {
			liquidationRisk = (position.LiqPrice - currentPrice) / currentPrice
		}

		// Alert if liquidation risk is high
		if liquidationRisk < 0.05 { // Less than 5% buffer
			if bot.config.EnableLogging {
				log.Printf("🚨 HIGH LIQUIDATION RISK: %.2f%% buffer remaining", liquidationRisk*100)
			}

			// Consider emergency position reduction
			bot.handleLiquidationRisk(position, liquidationRisk)
		}
	}
}

// Handle account updates
func (bot *HypeTradingBot) handleAccountUpdate(account execution.AccountInfo) {
	if bot.config.EnableLogging {
		log.Printf("💰 Account Update: Balance: $%.2f, Available: $%.2f, Positions: %d",
			account.TotalBalance, account.AvailableBalance, len(account.Positions))
	}
}

// Handle strategy notifications
func (bot *HypeTradingBot) handleStrategyNotification(notification strategy.StrategyNotification) {
	logLevel := ""
	switch string(notification.Severity) {
	case string(strategy.SeverityCritical):
		logLevel = "🚨 CRITICAL"
	case string(strategy.SeverityError):
		logLevel = "❌ ERROR"
	case string(strategy.SeverityWarning):
		logLevel = "⚠️  WARNING"
	default:
		logLevel = "ℹ️  INFO"
	}

	if bot.config.EnableLogging {
		log.Printf("%s: %s - %s", logLevel, notification.Type, notification.Message)
	}

	// Send Telegram notification if enabled
	if bot.config.EnableTelegram {
		go bot.sendTelegramNotification(fmt.Sprintf("%s: %s - %s",
			logLevel, notification.Type, notification.Message))
	}
}

// Handle liquidation risk
func (bot *HypeTradingBot) handleLiquidationRisk(position execution.Position, risk float64) {
	if risk < 0.02 { // Less than 2% buffer - emergency action
		if bot.config.EnableLogging {
			log.Printf("🚨 EMERGENCY: Reducing position due to liquidation risk")
		}

		// Reduce position by 50%
		err := bot.execution.ReducePosition(position.Symbol, 0.5)
		if err != nil {
			log.Printf("❌ Failed to reduce position: %v", err)
		}

		// Send emergency notification
		if bot.config.EnableTelegram {
			go bot.sendTelegramNotification(fmt.Sprintf("🚨 EMERGENCY: Reduced %s position due to liquidation risk (%.2f%% buffer)",
				position.Symbol, risk*100))
		}
	}
}

// Calculate and update PnL
func (bot *HypeTradingBot) calculateAndUpdatePnL(update execution.OrderUpdate) {
	// This is a simplified PnL calculation
	// In production, you'd want more sophisticated tracking

	bot.perfMu.Lock()
	defer bot.perfMu.Unlock()

	// Estimate PnL based on filled quantity and average price
	// This would need to be enhanced with proper position tracking
	estimatedPnL := update.FilledQty * update.AvgPrice * 0.001 // Placeholder calculation
	bot.totalPnL += estimatedPnL

	if bot.config.EnableLogging {
		log.Printf("💰 Estimated PnL update: $%.2f (Total: $%.2f)", estimatedPnL, bot.totalPnL)
	}
}

// Send Telegram notification
func (bot *HypeTradingBot) sendTelegramNotification(message string) {
	if bot.config.TelegramBotToken == "" || bot.config.TelegramChatID == "" {
		return
	}

	// Implementation would use Telegram Bot API
	// For now, just log that we would send notification
	if bot.config.EnableLogging {
		log.Printf("📱 Telegram notification: %s", message)
	}
}

// Start the trading bot
func (bot *HypeTradingBot) Start() error {
	log.Printf("🚀 Starting HYPE-USDC Trading Bot...")

	// Start all engines in sequence
	if err := bot.execution.Start(); err != nil {
		return fmt.Errorf("failed to start execution engine: %w", err)
	}

	if err := bot.strategy.Start(); err != nil {
		return fmt.Errorf("failed to start strategy engine: %w", err)
	}

	if err := bot.marketData.Start(); err != nil {
		return fmt.Errorf("failed to start market data engine: %w", err)
	}

	// Start monitoring goroutines
	go bot.performanceMonitor()
	go bot.riskMonitor()
	go bot.fundingRateMonitor()

	log.Printf("✅ HYPE-USDC Trading Bot started successfully!")
	log.Printf("📊 Trading Symbol: %s", bot.config.TradingSymbol)
	log.Printf("💰 Max Position Size: $%.0f", bot.config.MaxPositionSize)
	log.Printf("⚡ Max Leverage: %.1fx", bot.config.MaxLeverage)
	log.Printf("🛡️  Stop Loss: %.1f%%, Take Profit: %.1f%%",
		bot.config.StopLossPercent, bot.config.TakeProfitPercent)

	return nil
}

// Stop the trading bot
func (bot *HypeTradingBot) Stop() error {
	log.Printf("🛑 Stopping HYPE-USDC Trading Bot...")

	bot.cancel()
	close(bot.stopCh)

	// Stop all engines
	bot.marketData.Stop()
	bot.strategy.Stop()
	bot.execution.Stop()

	log.Printf("✅ HYPE-USDC Trading Bot stopped successfully")

	// Print final performance report
	bot.printFinalReport()

	return nil
}

// Performance monitoring
func (bot *HypeTradingBot) performanceMonitor() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-bot.ctx.Done():
			return
		case <-ticker.C:
			bot.printPerformanceUpdate()
		}
	}
}

// Risk monitoring
func (bot *HypeTradingBot) riskMonitor() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-bot.ctx.Done():
			return
		case <-ticker.C:
			bot.checkRiskLimits()
		}
	}
}

// Funding rate monitoring (for optimization)
func (bot *HypeTradingBot) fundingRateMonitor() {
	if !bot.config.FundingRateOptimization {
		return
	}

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-bot.ctx.Done():
			return
		case <-ticker.C:
			bot.optimizeFundingRate()
		}
	}
}

// Print performance update
func (bot *HypeTradingBot) printPerformanceUpdate() {
	bot.perfMu.RLock()
	totalTrades := bot.totalTrades
	totalPnL := bot.totalPnL
	bot.perfMu.RUnlock()

	bot.stateMu.RLock()
	currentPrice := bot.lastPrice
	position := bot.position
	bot.stateMu.RUnlock()

	uptime := time.Since(bot.startTime)
	executionStats := bot.execution.GetExecutionStats()

	positionInfo := "No position"
	if position != nil && position.Size != 0 {
		positionInfo = fmt.Sprintf("%s %.4f @ $%.4f (PnL: $%.2f)",
			position.Side, position.Size, position.EntryPrice, position.UnrealizedPnL)
	}

	log.Printf(`
📊 HYPE-USDC Performance Update:
├─ Uptime: %v
├─ Current Price: $%.4f
├─ Position: %s
├─ Total Trades: %d
├─ Total PnL: $%.2f
├─ Orders Placed: %d
├─ Success Rate: %.1f%%
├─ Avg Latency: %.1fms
└─ Active Orders: %d`,
		uptime.Truncate(time.Second),
		currentPrice,
		positionInfo,
		totalTrades,
		totalPnL,
		executionStats.OrdersPlaced,
		executionStats.SuccessRate*100,
		executionStats.AverageLatency,
		executionStats.PendingOrders)
}

// Check risk limits
func (bot *HypeTradingBot) checkRiskLimits() {
	accountInfo, err := bot.execution.GetAccountInfo()
	if err != nil {
		return
	}

	// Check daily loss limit
	bot.perfMu.RLock()
	dailyPnL := bot.totalPnL // Simplified - would need proper daily tracking
	bot.perfMu.RUnlock()

	maxDailyLoss := bot.config.MaxPositionSize * 0.1
	if dailyPnL < -maxDailyLoss {
		log.Printf("🚨 Daily loss limit exceeded: $%.2f", dailyPnL)
		// Could automatically stop trading or reduce positions
	}

	// Check drawdown
	startingBalance := bot.config.MaxPositionSize * 4 // Assumed starting balance
	currentDrawdown := (startingBalance - accountInfo.TotalBalance) / startingBalance
	if currentDrawdown > 0.15 { // 15% max drawdown
		log.Printf("🚨 Maximum drawdown exceeded: %.1f%%", currentDrawdown*100)
	}
}

// Optimize funding rate
func (bot *HypeTradingBot) optimizeFundingRate() {
	// Implementation would check current funding rates
	// and potentially adjust position sizes or timing
	if bot.config.EnableLogging {
		log.Printf("🔄 Checking funding rate optimization opportunities...")
	}
}

// Print final performance report
func (bot *HypeTradingBot) printFinalReport() {
	bot.perfMu.RLock()
	totalTrades := bot.totalTrades
	totalPnL := bot.totalPnL
	bot.perfMu.RUnlock()

	uptime := time.Since(bot.startTime)
	executionStats := bot.execution.GetExecutionStats()
	latencyMetrics := bot.execution.GetLatencyMetrics()

	avgPnLPerTrade := 0.0
	if totalTrades > 0 {
		avgPnLPerTrade = totalPnL / float64(totalTrades)
	}

	tradesPerHour := 0.0
	if uptime.Hours() > 0 {
		tradesPerHour = float64(totalTrades) / uptime.Hours()
	}

	systemUptime := 100.0
	if executionStats.UptimeSeconds > 0 {
		systemUptime = float64(executionStats.UptimeSeconds) / uptime.Seconds() * 100
	}

	log.Printf(`
🎯 FINAL HYPE-USDC TRADING REPORT:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📊 Trading Performance:
├─ Total Runtime: %v
├─ Total Trades: %d
├─ Total PnL: $%.2f
├─ Average PnL per Trade: $%.2f
└─ Trades per Hour: %.1f

🚀 Execution Performance:
├─ Orders Placed: %d
├─ Orders Executed: %d
├─ Orders Cancelled: %d
├─ Success Rate: %.1f%%
├─ Total Volume: $%.0f
└─ System Uptime: %.1f%%

⚡ Latency Metrics:
├─ Order Placement: %.1fms (avg), %.1fms (p99)
├─ Order Cancellation: %.1fms (avg), %.1fms (p99)
├─ Position Updates: %.1fms (avg)
└─ Market Data: %.1fms (avg)

✅ HYPE-USDC Trading Session Complete
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━`,
		uptime.Truncate(time.Second),
		totalTrades,
		totalPnL,
		avgPnLPerTrade,
		tradesPerHour,

		executionStats.OrdersPlaced,
		executionStats.OrdersExecuted,
		executionStats.OrdersCancelled,
		executionStats.SuccessRate*100,
		executionStats.TotalVolume,
		systemUptime,

		latencyMetrics.OrderPlacement.Average,
		latencyMetrics.OrderPlacement.P99,
		latencyMetrics.OrderCancellation.Average,
		latencyMetrics.OrderCancellation.P99,
		latencyMetrics.PositionUpdates.Average,
		latencyMetrics.MarketDataProcessing.Average)
}

// Load configuration from environment variables
func loadConfigFromEnv() *HypeTradingConfig {
	config := DefaultHypeConfig()

	// Load from environment variables if available
	if privateKey := os.Getenv("HYPERLIQUID_PRIVATE_KEY"); privateKey != "" {
		// Trim leading/trailing quotes and spaces
		trimmedKey := strings.TrimSpace(privateKey)
		trimmedKey = strings.Trim(trimmedKey, "\"") // Trim double quotes
		trimmedKey = strings.Trim(trimmedKey, "'")  // Trim single quotes
		config.PrivateKeyHex = trimmedKey
	}

	if mainnet := os.Getenv("HYPERLIQUID_MAINNET"); mainnet != "" {
		config.IsMainnet = mainnet == "true"
	}

	if symbol := os.Getenv("TRADING_SYMBOL"); symbol != "" {
		config.TradingSymbol = symbol
	}

	if maxPos := os.Getenv("MAX_POSITION_SIZE"); maxPos != "" {
		if val, err := strconv.ParseFloat(maxPos, 64); err == nil {
			config.MaxPositionSize = val
		}
	}

	if maxLev := os.Getenv("MAX_LEVERAGE"); maxLev != "" {
		if val, err := strconv.ParseFloat(maxLev, 64); err == nil {
			config.MaxLeverage = val
		}
	}

	if stopLoss := os.Getenv("STOP_LOSS_PERCENT"); stopLoss != "" {
		if val, err := strconv.ParseFloat(stopLoss, 64); err == nil {
			config.StopLossPercent = val
		}
	}

	if takeProfit := os.Getenv("TAKE_PROFIT_PERCENT"); takeProfit != "" {
		if val, err := strconv.ParseFloat(takeProfit, 64); err == nil {
			config.TakeProfitPercent = val
		}
	}

	return config
}

// Main function to run the HYPE-USDC trading bot
func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	// Load environment variables from .env file
	err := godotenv.Overload()
	if err != nil {
		log.Printf("ℹ️ Info: Error loading .env file (this is not fatal, will rely on existing env vars): %v", err)
	}

	log.Printf("🚀 HYPE-USDC Hyperliquid Trading Bot v1.0")
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Load configuration from environment or use defaults
	config := loadConfigFromEnv()

	// Validate private key
	if config.PrivateKeyHex == "" {
		log.Printf("❌ ERROR: HYPERLIQUID_PRIVATE_KEY environment variable not set!")
		log.Printf("Please set your private key:")
		log.Printf("export HYPERLIQUID_PRIVATE_KEY=\"your_private_key_here\"")
		log.Printf("or add it to your .env file")
		return
	}

	// Log configuration
	log.Printf("📋 Configuration:")
	log.Printf("├─ Trading Symbol: %s", config.TradingSymbol)
	log.Printf("├─ Max Position: $%.0f", config.MaxPositionSize)
	log.Printf("├─ Max Leverage: %.1fx", config.MaxLeverage)
	log.Printf("├─ Stop Loss: %.1f%%", config.StopLossPercent)
	log.Printf("├─ Take Profit: %.1f%%", config.TakeProfitPercent)
	log.Printf("├─ Strategies: TF=%v, MR=%v, Scalp=%v",
		config.TrendFollowingEnabled, config.MeanReversionEnabled, config.ScalpingEnabled)
	log.Printf("└─ Mainnet: %v", config.IsMainnet)

	// Create and configure the trading bot
	bot, err := NewHypeTradingBot(config)
	if err != nil {
		log.Fatalf("❌ Failed to create trading bot: %v", err)
	}

	// Start the bot
	if err := bot.Start(); err != nil {
		log.Fatalf("❌ Failed to start trading bot: %v", err)
	}

	// Set up graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	log.Printf("✅ HYPE-USDC Trading Bot is now LIVE!")
	log.Printf("💰 Trading HYPE against USDC on Hyperliquid")
	log.Printf("📊 Monitoring market conditions and executing trades...")
	log.Printf("🛑 Press Ctrl+C to stop the bot")

	// Wait for shutdown signal
	<-c

	log.Printf("🛑 Shutdown signal received, stopping bot...")
	if err := bot.Stop(); err != nil {
		log.Printf("❌ Error stopping bot: %v", err)
	}

	log.Printf("✅ Trading bot stopped successfully. Goodbye! 👋")
}
