package execution

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/websocket"
)

// Core interfaces and types
type ExecutionEngine interface {
	Start() error
	Stop() error
	PlaceOrder(order Order) (*OrderResult, error)
	CancelOrder(orderID string) error
	ModifyOrder(orderID string, modification OrderModification) error
	
	GetPositions() []Position
	GetPosition(symbol string) (*Position, error)
	ClosePosition(symbol string) error
	ReducePosition(symbol string, percentage float64) error
	
	GetAccountInfo() (*AccountInfo, error)
	GetBalances() (map[string]Balance, error)
	GetMarginInfo() (*MarginInfo, error)
	
	GetOpenOrders() []Order
	GetOrderHistory(limit int) []Order
	GetOrderStatus(orderID string) (*OrderStatus, error)
	
	SetRiskLimits(limits RiskLimits) error
	GetLiquidationPrices() map[string]float64
	
	SetOrderUpdateCallback(callback OrderUpdateCallback)
	SetPositionUpdateCallback(callback PositionUpdateCallback)
	SetAccountUpdateCallback(callback AccountUpdateCallback)
	
	GetExecutionStats() ExecutionStatistics
	GetLatencyMetrics() LatencyMetrics
}

// Order types and structures
type OrderSide string
type OrderType string
type OrderStatus string

const (
	OrderSideBuy  OrderSide = "B"
	OrderSideSell OrderSide = "S"
	
	OrderTypeMarket     OrderType = "M"
	OrderTypeLimit      OrderType = "L"
	OrderTypeStopLoss   OrderType = "SL"
	OrderTypeTakeProfit OrderType = "TP"
	
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusOpen      OrderStatus = "open"
	OrderStatusFilled    OrderStatus = "filled"
	OrderStatusCancelled OrderStatus = "cancelled"
	OrderStatusRejected  OrderStatus = "rejected"
)

type Order struct {
	ID        string    `json:"id"`
	Symbol    string    `json:"symbol"`
	Side      OrderSide `json:"side"`
	Type      OrderType `json:"type"`
	Quantity  float64   `json:"quantity"`
	Price     float64   `json:"price,omitempty"`
	StopPrice float64   `json:"stop_price,omitempty"`
	Leverage  float64   `json:"leverage"`
	Status    OrderStatus `json:"status"`
	Timestamp time.Time `json:"timestamp"`

	// Hyperliquid specific
	ReduceOnly    bool    `json:"reduce_only"`
	PostOnly      bool    `json:"post_only"`
	TimeInForce   string  `json:"time_in_force"`
	ClientOrderID string  `json:"client_order_id,omitempty"`

	// Added for compatibility
	Size         float64   `json:"size"`
	FilledSize   float64   `json:"filled_size"`
	AvgFillPrice float64   `json:"avg_fill_price"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type OrderResult struct {
	OrderID     string      `json:"order_id"`
	Status      OrderStatus `json:"status"`
	FilledQty   float64     `json:"filled_qty"`
	AvgPrice    float64     `json:"avg_price"`
	Commission  float64     `json:"commission"`
	Timestamp   time.Time   `json:"timestamp"`
	Latency     time.Duration `json:"latency"`
}

type OrderModification struct {
	Price    *float64 `json:"price,omitempty"`
	Quantity *float64 `json:"quantity,omitempty"`
}

type Position struct {
	Symbol        string    `json:"symbol"`
	Side          string    `json:"side"`
	Size          float64   `json:"size"`
	EntryPrice    float64   `json:"entry_price"`
	MarkPrice     float64   `json:"mark_price"`
	UnrealizedPnL float64   `json:"unrealized_pnl"`
	Leverage      float64   `json:"leverage"`
	Margin        float64   `json:"margin"`
	LiqPrice      float64   `json:"liq_price"`
	Timestamp     time.Time `json:"timestamp"`
}

type Balance struct {
	Asset     string  `json:"asset"`
	Free      float64 `json:"free"`
	Used      float64 `json:"used"`
	Total     float64 `json:"total"`
}

type AccountInfo struct {
	TotalBalance    float64            `json:"total_balance"`
	AvailableBalance float64           `json:"available_balance"`
	UsedMargin      float64            `json:"used_margin"`
	FreeMargin      float64            `json:"free_margin"`
	Balances        map[string]Balance `json:"balances"`
	Positions       []Position         `json:"positions"`
	Timestamp       time.Time          `json:"timestamp"`
}

type MarginInfo struct {
	MaintenanceMargin float64 `json:"maintenance_margin"`
	InitialMargin     float64 `json:"initial_margin"`
	MarginRatio       float64 `json:"margin_ratio"`
	LiquidationRisk   float64 `json:"liquidation_risk"`
}

type RiskLimits struct {
	MaxPositionSize   float64 `json:"max_position_size"`
	MaxLeverage       float64 `json:"max_leverage"`
	MaxDailyLoss      float64 `json:"max_daily_loss"`
	MaxDrawdown       float64 `json:"max_drawdown"`
	StopLossRequired  bool    `json:"stop_loss_required"`
}

// Callback types
type OrderUpdateCallback func(OrderUpdate)
type PositionUpdateCallback func(Position)
type AccountUpdateCallback func(AccountInfo)

type OrderUpdate struct {
	OrderID   string      `json:"order_id"`
	OldStatus OrderStatus `json:"old_status"`
	NewStatus OrderStatus `json:"new_status"`
	FilledQty float64     `json:"filled_qty"`
	AvgPrice  float64     `json:"avg_price"`
	Timestamp time.Time   `json:"timestamp"`
}

// Performance monitoring types
type ExecutionStatistics struct {
	OrdersPlaced         int64   `json:"orders_placed"`
	OrdersExecuted       int64   `json:"orders_executed"`
	OrdersCancelled      int64   `json:"orders_cancelled"`
	AverageLatency       float64 `json:"avg_latency_ms"`
	SuccessRate          float64 `json:"success_rate"`
	TotalVolume          float64 `json:"total_volume"`
	ActiveConnections    int     `json:"active_connections"`
	PendingOrders        int     `json:"pending_orders"`
	UptimeSeconds        int64   `json:"uptime_seconds"`
	LastHeartbeat        time.Time `json:"last_heartbeat"`
}

type LatencyMetrics struct {
	OrderPlacement   LatencyStats `json:"order_placement"`
	OrderCancellation LatencyStats `json:"order_cancellation"`
	PositionUpdates  LatencyStats `json:"position_updates"`
	MarketDataProcessing LatencyStats `json:"market_data_processing"`
}

type LatencyStats struct {
	Min        float64 `json:"min_ms"`
	Max        float64 `json:"max_ms"`
	Average    float64 `json:"average_ms"`
	P50        float64 `json:"p50_ms"`
	P95        float64 `json:"p95_ms"`
	P99        float64 `json:"p99_ms"`
	SampleSize int64   `json:"sample_size"`
}

// Hyperliquid execution engine implementation
type HyperliquidEngine struct {
	config     *Config
	privateKey *ecdsa.PrivateKey
	address    string
	
	// HTTP client for REST API
	httpClient *http.Client
	
	// WebSocket connections
	wsConn     *websocket.Conn
	wsConnMu   sync.RWMutex
	
	// State management
	positions  map[string]*Position
	orders     map[string]*Order
	balances   map[string]Balance
	stateMu    sync.RWMutex
	
	// Risk management
	riskLimits RiskLimits
	riskMu     sync.RWMutex
	
	// Callbacks
	orderCallback    OrderUpdateCallback
	positionCallback PositionUpdateCallback
	accountCallback  AccountUpdateCallback
	callbackMu       sync.RWMutex
	
	// Performance tracking
	stats        *ExecutionStatistics
	latencies    map[string][]float64
	performanceMu sync.RWMutex
	startTime    time.Time
	
	// Control channels
	ctx        context.Context
	cancel     context.CancelFunc
	stopCh     chan struct{}
	
	// Connection management
	reconnectAttempts int
	maxReconnects     int
	reconnectDelay    time.Duration
	
	// Rate limiting
	rateLimiter *RateLimiter
	
	// Order management
	orderSeq      int64
	orderSeqMu    sync.Mutex
}

type Config struct {
	PrivateKeyHex    string        `json:"private_key_hex"`
	BaseURL          string        `json:"base_url"`
	WebSocketURL     string        `json:"websocket_url"`
	Timeout          time.Duration `json:"timeout"`
	MaxReconnects    int           `json:"max_reconnects"`
	ReconnectDelay   time.Duration `json:"reconnect_delay"`
	EnableLogging    bool          `json:"enable_logging"`
	RateLimitRPS     int           `json:"rate_limit_rps"`
	
	// Hyperliquid specific
	VaultAddress     string `json:"vault_address,omitempty"`
	IsMainnet        bool   `json:"is_mainnet"`
}

// Rate limiter implementation
type RateLimiter struct {
	tokens    int
	maxTokens int
	refillRate time.Duration
	lastRefill time.Time
	mu         sync.Mutex
}

func NewRateLimiter(rps int) *RateLimiter {
	return &RateLimiter{
		tokens:     rps,
		maxTokens:  rps,
		refillRate: time.Second / time.Duration(rps),
		lastRefill: time.Now(),
	}
}

func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)
	tokensToAdd := int(elapsed / rl.refillRate)
	
	if tokensToAdd > 0 {
		rl.tokens = min(rl.maxTokens, rl.tokens+tokensToAdd)
		rl.lastRefill = now
	}
	
	if rl.tokens > 0 {
		rl.tokens--
		return true
	}
	
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Default configuration for Hyperliquid
func DefaultConfig() *Config {
	return &Config{
		BaseURL:        "https://api.hyperliquid.xyz",
		WebSocketURL:   "wss://api.hyperliquid.xyz/ws",
		Timeout:        30 * time.Second,
		MaxReconnects:  10,
		ReconnectDelay: 5 * time.Second,
		EnableLogging:  true,
		RateLimitRPS:   100,
		IsMainnet:      true,
	}
}

// Create new Hyperliquid execution engine
func NewHyperliquidEngine(config *Config) (*HyperliquidEngine, error) {
	if config.PrivateKeyHex == "" {
		return nil, fmt.Errorf("private key is required")
	}
	
	// Parse private key
	privateKeyBytes, err := hex.DecodeString(strings.TrimPrefix(config.PrivateKeyHex, "0x"))
	if err != nil {
		return nil, fmt.Errorf("invalid private key hex: %w", err)
	}
	
	privateKey, err := crypto.ToECDSA(privateKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}
	
	// Get address from private key
	address := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	
	ctx, cancel := context.WithCancel(context.Background())
	
	engine := &HyperliquidEngine{
		config:     config,
		privateKey: privateKey,
		address:    address,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		positions:         make(map[string]*Position),
		orders:           make(map[string]*Order),
		balances:         make(map[string]Balance),
		ctx:              ctx,
		cancel:           cancel,
		stopCh:           make(chan struct{}),
		maxReconnects:    config.MaxReconnects,
		reconnectDelay:   config.ReconnectDelay,
		rateLimiter:      NewRateLimiter(config.RateLimitRPS),
		latencies:        make(map[string][]float64),
		startTime:        time.Now(),
		stats: &ExecutionStatistics{
			ActiveConnections: 0,
		},
	}
	
	// Initialize default risk limits
	engine.riskLimits = RiskLimits{
		MaxPositionSize:  100000,  // $100k max position
		MaxLeverage:      20,      // 20x max leverage
		MaxDailyLoss:     5000,    // $5k max daily loss
		MaxDrawdown:      0.20,    // 20% max drawdown
		StopLossRequired: true,
	}
	
	return engine, nil
}

// Start the execution engine
func (e *HyperliquidEngine) Start() error {
	if e.config.EnableLogging {
		log.Printf("🚀 Starting Hyperliquid Execution Engine for address: %s", e.address)
	}
	
	// Initialize account state
	if err := e.initializeAccount(); err != nil {
		return fmt.Errorf("failed to initialize account: %w", err)
	}
	
	// Start WebSocket connection
	if err := e.connectWebSocket(); err != nil {
		return fmt.Errorf("failed to connect WebSocket: %w", err)
	}
	
	// Start background goroutines
	go e.wsMessageHandler()
	go e.periodicSync()
	go e.heartbeat()
	
	if e.config.EnableLogging {
		log.Printf("✅ Hyperliquid Execution Engine started successfully")
	}
	
	return nil
}

// Convert order to Hyperliquid format
func (e *HyperliquidEngine) convertToHyperliquidOrder(order Order) map[string]interface{} {
	hlOrder := map[string]interface{}{
		"coin":     order.Symbol,
		"is_buy":   order.Side == OrderSideBuy,
		"sz":       fmt.Sprintf("%.6f", order.Quantity),
		"limit_px": fmt.Sprintf("%.6f", fmt.Sprintf("%.2f", order.Price)),
		"order_type": map[string]interface{}{
			"limit": map[string]interface{}{
				"tif": order.TimeInForce,
			},
		},
		"reduce_only": order.ReduceOnly,
	}
	
	if order.Type == OrderTypeMarket {
		hlOrder["order_type"] = map[string]interface{}{
			"market": map[string]interface{}{},
		}
	}
	
	if order.PostOnly {
		hlOrder["order_type"].(map[string]interface{})["limit"].(map[string]interface{})["post_only"] = true
	}
	
	return hlOrder
}

// Submit order to Hyperliquid
func (e *HyperliquidEngine) submitOrder(order map[string]interface{}) (*OrderResult, error) {
	// Create order action
	action := map[string]interface{}{
		"type":   "order",
		"orders": []interface{}{order},
		"grouping": "na",
	}
	
	// Sign the action
	signature, err := e.signAction(action)
	if err != nil {
		return nil, fmt.Errorf("failed to sign order: %w", err)
	}
	
	// Prepare request
	requestBody := map[string]interface{}{
		"action":    action,
		"nonce":     time.Now().UnixMilli(),
		"signature": signature,
	}
	
	// Submit to API
	response, err := e.makeAPIRequest("POST", "/exchange", requestBody)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	
	// Parse response
	var apiResponse map[string]interface{}
	if err := json.Unmarshal(response, &apiResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	if status, ok := apiResponse["status"].(string); ok && status == "err" {
		return nil, fmt.Errorf("order rejected: %v", apiResponse["response"])
	}
	
	// Extract order result
	responseData, ok := apiResponse["response"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}
	
	statuses, ok := responseData["statuses"].([]interface{})
	if !ok || len(statuses) == 0 {
		return nil, fmt.Errorf("no order status in response")
	}
	
	orderStatus := statuses[0].(map[string]interface{})
	
	result := &OrderResult{
		OrderID:   fmt.Sprintf("%.0f", orderStatus["resting"].(map[string]interface{})["oid"].(float64)),
		Status:    OrderStatusOpen,
		FilledQty: 0,
		AvgPrice:  0,
		Timestamp: time.Now(),
	}
	
	if filled, ok := orderStatus["filled"].(map[string]interface{}); ok {
		if totalSz, exists := filled["totalSz"].(string); exists {
			result.FilledQty, _ = strconv.ParseFloat(totalSz, 64)
			if result.FilledQty > 0 {
				result.Status = OrderStatusFilled
				if avgPx, exists := filled["avgPx"].(string); exists {
					result.AvgPrice, _ = strconv.ParseFloat(avgPx, 64)
				}
			}
		}
	}
	
	return result, nil
}

// Sign action for Hyperliquid
func (e *HyperliquidEngine) signAction(action map[string]interface{}) (string, error) {
	// Convert action to JSON
	actionBytes, err := json.Marshal(action)
	if err != nil {
		return "", err
	}
	
	// Create hash
	hash := crypto.Keccak256Hash(actionBytes)
	
	// Sign hash
	signature, err := crypto.Sign(hash.Bytes(), e.privateKey)
	if err != nil {
		return "", err
	}
	
	// Convert to hex
	return hex.EncodeToString(signature), nil
}

// Make API request to Hyperliquid
func (e *HyperliquidEngine) makeAPIRequest(method, endpoint string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}
	
	req, err := http.NewRequest(method, e.config.BaseURL+endpoint, reqBody)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}
	
	return respBody, nil
}

// Cancel order
func (e *HyperliquidEngine) CancelOrder(orderID string) error {
	startTime := time.Now()
	
	if !e.rateLimiter.Allow() {
		return fmt.Errorf("rate limit exceeded")
	}
	
	// Get order details
	e.stateMu.RLock()
	order, exists := e.orders[orderID]
	e.stateMu.RUnlock()
	
	if !exists {
		return fmt.Errorf("order not found: %s", orderID)
	}
	
	// Create cancel action
	action := map[string]interface{}{
		"type": "cancel",
		"cancels": []interface{}{
			map[string]interface{}{
				"coin": order.Symbol,
				"oid":  orderID,
			},
		},
	}
	
	// Sign and submit
	signature, err := e.signAction(action)
	if err != nil {
		return fmt.Errorf("failed to sign cancel: %w", err)
	}
	
	requestBody := map[string]interface{}{
		"action":    action,
		"nonce":     time.Now().UnixMilli(),
		"signature": signature,
	}
	
	response, err := e.makeAPIRequest("POST", "/exchange", requestBody)
	if err != nil {
		e.recordLatency("order_cancellation", startTime, false)
		return fmt.Errorf("cancel request failed: %w", err)
	}
	
	// Parse response
	var apiResponse map[string]interface{}
	if err := json.Unmarshal(response, &apiResponse); err != nil {
		return fmt.Errorf("failed to parse cancel response: %w", err)
	}
	
	if status, ok := apiResponse["status"].(string); ok && status == "err" {
		return fmt.Errorf("cancel rejected: %v", apiResponse["response"])
	}
	
	// Update order status
	e.stateMu.Lock()
	if order, exists := e.orders[orderID]; exists {
		order.Status = OrderStatusCancelled
	}
	e.stateMu.Unlock()
	
	// Update statistics
	e.performanceMu.Lock()
	e.stats.OrdersCancelled++
	e.performanceMu.Unlock()
	
	e.recordLatency("order_cancellation", startTime, true)
	
	if e.config.EnableLogging {
		log.Printf("❌ Order cancelled: %s", orderID)
	}
	
	return nil
}

// Modify order
func (e *HyperliquidEngine) ModifyOrder(orderID string, modification OrderModification) error {
	// Hyperliquid doesn't support direct order modification
	// We need to cancel and replace
	
	e.stateMu.RLock()
	order, exists := e.orders[orderID]
	e.stateMu.RUnlock()
	
	if !exists {
		return fmt.Errorf("order not found: %s", orderID)
	}
	
	// Cancel existing order
	if err := e.CancelOrder(orderID); err != nil {
		return fmt.Errorf("failed to cancel order for modification: %w", err)
	}
	
	// Create new order with modifications
	newOrder := *order
	if modification.Price != nil {
		newOrder.Price = *modification.Price
	}
	if modification.Quantity != nil {
		newOrder.Quantity = *modification.Quantity
	}
	
	// Place new order
	_, err := e.PlaceOrder(newOrder)
	return err
}

// Get current positions
func (e *HyperliquidEngine) GetPositions() []Position {
	e.stateMu.RLock()
	defer e.stateMu.RUnlock()
	
	positions := make([]Position, 0, len(e.positions))
	for _, pos := range e.positions {
		if pos.Size != 0 {
			positions = append(positions, *pos)
		}
	}
	
	return positions
}

// Get specific position
func (e *HyperliquidEngine) GetPosition(symbol string) (*Position, error) {
	e.stateMu.RLock()
	defer e.stateMu.RUnlock()
	
	if pos, exists := e.positions[symbol]; exists && pos.Size != 0 {
		return pos, nil
	}
	
	return nil, fmt.Errorf("no position found for symbol: %s", symbol)
}

// Close position
func (e *HyperliquidEngine) ClosePosition(symbol string) error {
	position, err := e.GetPosition(symbol)
	if err != nil {
		return err
	}
	
	// Create closing order
	var side OrderSide
	if position.Side == "long" {
		side = OrderSideSell
	} else {
		side = OrderSideBuy
	}
	
	order := Order{
		Symbol:     symbol,
		Side:       side,
		Type:       OrderTypeMarket,
		Quantity:   math.Abs(position.Size),
		ReduceOnly: true,
		Leverage:   1, // Market order for closing
	}
	
	_, err = e.PlaceOrder(order)
	return err
}

// Reduce position by percentage
func (e *HyperliquidEngine) ReducePosition(symbol string, percentage float64) error {
	if percentage <= 0 || percentage > 1 {
		return fmt.Errorf("invalid percentage: %.2f (must be between 0 and 1)", percentage)
	}
	
	position, err := e.GetPosition(symbol)
	if err != nil {
		return err
	}
	
	// Calculate reduction quantity
	reduceQty := math.Abs(position.Size) * percentage
	
	var side OrderSide
	if position.Side == "long" {
		side = OrderSideSell
	} else {
		side = OrderSideBuy
	}
	
	order := Order{
		Symbol:     symbol,
		Side:       side,
		Type:       OrderTypeMarket,
		Quantity:   reduceQty,
		ReduceOnly: true,
		Leverage:   1,
	}
	
	_, err = e.PlaceOrder(order)
	return err
}

// Get account information
func (e *HyperliquidEngine) GetAccountInfo() (*AccountInfo, error) {
	requestBody := map[string]interface{}{
		"type": "clearinghouseState",
		"user": e.address,
	}
	
	response, err := e.makeAPIRequest("POST", "/info", requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to get account info: %w", err)
	}
	
	var apiResponse map[string]interface{}
	if err := json.Unmarshal(response, &apiResponse); err != nil {
		return nil, fmt.Errorf("failed to parse account info: %w", err)
	}
	
	// Parse balances
	balances := make(map[string]Balance)
	if balanceData, ok := apiResponse["balances"].([]interface{}); ok {
		for _, bal := range balanceData {
			balanceMap := bal.(map[string]interface{})
			coin, _ := balanceMap["coin"].(string)
			hold, _ := strconv.ParseFloat(balanceMap["hold"].(string), 64)
			total, _ := strconv.ParseFloat(balanceMap["total"].(string), 64)
			
			balances[coin] = Balance{
				Asset: coin,
				Total: total,
				Used:  hold,
				Free:  total - hold,
			}
		}
	}
	
	// Parse positions
	var positions []Position
	if positionData, ok := apiResponse["assetPositions"].([]interface{}); ok {
		for _, pos := range positionData {
			posMap := pos.(map[string]interface{})
			position, _ := posMap["position"].(map[string]interface{})
			
			coin, _ := position["coin"].(string)
			side, _ := position["side"].(string)
			szi, _ := strconv.ParseFloat(position["szi"].(string), 64)
			entryPx, _ := strconv.ParseFloat(position["entryPx"].(string), 64)
			
			if szi != 0 {
				positions = append(positions, Position{
					Symbol:     coin,
					Side:       side,
					Size:       szi,
					EntryPrice: entryPx,
					Timestamp:  time.Now(),
				})
			}
		}
	}
	
	// Calculate totals
	var totalBalance, availableBalance float64
	for _, balance := range balances {
		totalBalance += balance.Total
		availableBalance += balance.Free
	}
	
	return &AccountInfo{
		TotalBalance:     totalBalance,
		AvailableBalance: availableBalance,
		Balances:         balances,
		Positions:        positions,
		Timestamp:        time.Now(),
	}, nil
}

// Get balances
func (e *HyperliquidEngine) GetBalances() (map[string]Balance, error) {
	accountInfo, err := e.GetAccountInfo()
	if err != nil {
		return nil, err
	}
	return accountInfo.Balances, nil
}

// Get margin information
func (e *HyperliquidEngine) GetMarginInfo() (*MarginInfo, error) {
	// This would require additional API calls to get margin details
	// For now, return basic info from positions
	positions := e.GetPositions()
	
	var usedMargin, maintenanceMargin float64
	for _, pos := range positions {
		usedMargin += pos.Margin
		maintenanceMargin += pos.Margin * 0.05 // Approximate 5% maintenance margin
	}
	
	balances, err := e.GetBalances()
	if err != nil {
		return nil, err
	}
	
	var totalBalance float64
	for _, balance := range balances {
		totalBalance += balance.Total
	}
	
	marginRatio := 0.0
	if usedMargin > 0 {
		marginRatio = totalBalance / usedMargin
	}
	
	return &MarginInfo{
		MaintenanceMargin: maintenanceMargin,
		InitialMargin:     usedMargin,
		MarginRatio:       marginRatio,
		LiquidationRisk:   math.Max(0, 1-marginRatio/1.2), // Risk increases as ratio approaches 1.2
	}, nil
}

// Get open orders
func (e *HyperliquidEngine) GetOpenOrders() []Order {
	requestBody := map[string]interface{}{
		"type": "openOrders",
		"user": e.address,
	}
	
	response, err := e.makeAPIRequest("POST", "/info", requestBody)
	if err != nil {
		if e.config.EnableLogging {
			log.Printf("Failed to get open orders: %v", err)
		}
		return []Order{}
	}
	
	var orders []interface{}
	if err := json.Unmarshal(response, &orders); err != nil {
		if e.config.EnableLogging {
			log.Printf("Failed to parse open orders: %v", err)
		}
		return []Order{}
	}
	
	var result []Order
	for _, orderData := range orders {
		orderMap := orderData.(map[string]interface{})
		order := orderMap["order"].(map[string]interface{})
		
		oid, _ := strconv.ParseFloat(order["oid"].(string), 64)
		coin, _ := order["coin"].(string)
		side, _ := order["side"].(string)
		sz, _ := strconv.ParseFloat(order["sz"].(string), 64)
		limitPx, _ := strconv.ParseFloat(order["limitPx"].(string), 64)
		
		var orderSide OrderSide
		if side == "B" {
			orderSide = OrderSideBuy
		} else {
			orderSide = OrderSideSell
		}
		
		result = append(result, Order{
			ID:       fmt.Sprintf("%.0f", oid),
			Symbol:   coin,
			Side:     orderSide,
			Type:     OrderTypeLimit,
			Quantity: sz,
			Price:    limitPx,
			Status:   OrderStatusOpen,
		})
	}
	
	return result
}

// Get order history
func (e *HyperliquidEngine) GetOrderHistory(limit int) []Order {
	// This would require additional API implementation
	// For now, return from internal cache
	e.stateMu.RLock()
	defer e.stateMu.RUnlock()
	
	var orders []Order
	for _, order := range e.orders {
		orders = append(orders, *order)
	}
	
	// Sort by timestamp (newest first)
	sort.Slice(orders, func(i, j int) bool {
		return orders[i].Timestamp.After(orders[j].Timestamp)
	})
	
	if limit > 0 && len(orders) > limit {
		orders = orders[:limit]
	}
	
	return orders
}

// Get order status
func (e *HyperliquidEngine) GetOrderStatus(orderID string) (*OrderStatus, error) {
	e.stateMu.RLock()
	order, exists := e.orders[orderID]
	e.stateMu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("order not found: %s", orderID)
	}
	
	status := order.Status
	return &status, nil
}

// Set risk limits
func (e *HyperliquidEngine) SetRiskLimits(limits RiskLimits) error {
	e.riskMu.Lock()
	e.riskLimits = limits
	e.riskMu.Unlock()
	
	if e.config.EnableLogging {
		log.Printf("🛡️  Risk limits updated: MaxPos=$%.0f, MaxLev=%.1fx, MaxLoss=$%.0f", 
			limits.MaxPositionSize, limits.MaxLeverage, limits.MaxDailyLoss)
	}
	
	return nil
}

// Get liquidation prices
func (e *HyperliquidEngine) GetLiquidationPrices() map[string]float64 {
	positions := e.GetPositions()
	liquidationPrices := make(map[string]float64)
	
	for _, pos := range positions {
		if pos.LiqPrice > 0 {
			liquidationPrices[pos.Symbol] = pos.LiqPrice
		} else {
			// Calculate approximate liquidation price
			maintenanceMargin := 0.05 // 5% maintenance margin
			if pos.Side == "long" {
				liquidationPrices[pos.Symbol] = pos.EntryPrice * (1 - (1/pos.Leverage - maintenanceMargin))
			} else {
				liquidationPrices[pos.Symbol] = pos.EntryPrice * (1 + (1/pos.Leverage - maintenanceMargin))
			}
		}
	}
	
	return liquidationPrices
}

// Set callbacks
func (e *HyperliquidEngine) SetOrderUpdateCallback(callback OrderUpdateCallback) {
	e.callbackMu.Lock()
	e.orderCallback = callback
	e.callbackMu.Unlock()
}

func (e *HyperliquidEngine) SetPositionUpdateCallback(callback PositionUpdateCallback) {
	e.callbackMu.Lock()
	e.positionCallback = callback
	e.callbackMu.Unlock()
}

func (e *HyperliquidEngine) SetAccountUpdateCallback(callback AccountUpdateCallback) {
	e.callbackMu.Lock()
	e.accountCallback = callback
	e.callbackMu.Unlock()
}

// Get execution statistics
func (e *HyperliquidEngine) GetExecutionStats() ExecutionStatistics {
	e.performanceMu.RLock()
	defer e.performanceMu.RUnlock()
	
	stats := *e.stats
	
	// Calculate success rate
	if stats.OrdersPlaced > 0 {
		stats.SuccessRate = float64(stats.OrdersExecuted) / float64(stats.OrdersPlaced)
	}
	
	// Calculate average latency
	if latencies, exists := e.latencies["order_placement"]; exists && len(latencies) > 0 {
		var total float64
		for _, latency := range latencies {
			total += latency
		}
		stats.AverageLatency = total / float64(len(latencies))
	}
	
	// Count active connections
	e.wsConnMu.RLock()
	if e.wsConn != nil {
		stats.ActiveConnections = 1
	}
	e.wsConnMu.RUnlock()
	
	// Count pending orders
	e.stateMu.RLock()
	for _, order := range e.orders {
		if order.Status == OrderStatusPending || order.Status == OrderStatusOpen {
			stats.PendingOrders++
		}
	}
	e.stateMu.RUnlock()
	
	return stats
}

// Get latency metrics
func (e *HyperliquidEngine) GetLatencyMetrics() LatencyMetrics {
	e.performanceMu.RLock()
	defer e.performanceMu.RUnlock()
	
	metrics := LatencyMetrics{
		OrderPlacement:       e.calculateLatencyStats("order_placement"),
		OrderCancellation:    e.calculateLatencyStats("order_cancellation"),
		PositionUpdates:      e.calculateLatencyStats("position_updates"),
		MarketDataProcessing: e.calculateLatencyStats("market_data_processing"),
	}
	
	return metrics
}

// Calculate latency statistics
func (e *HyperliquidEngine) calculateLatencyStats(operation string) LatencyStats {
	latencies, exists := e.latencies[operation]
	if !exists || len(latencies) == 0 {
		return LatencyStats{}
	}
	
	// Sort for percentile calculations
	sorted := make([]float64, len(latencies))
	copy(sorted, latencies)
	sort.Float64s(sorted)
	
	var sum float64
	for _, latency := range sorted {
		sum += latency
	}
	
	stats := LatencyStats{
		Min:        sorted[0],
		Max:        sorted[len(sorted)-1],
		Average:    sum / float64(len(sorted)),
		SampleSize: int64(len(sorted)),
	}
	
	// Calculate percentiles
	stats.P50 = sorted[int(float64(len(sorted))*0.5)]
	stats.P95 = sorted[int(float64(len(sorted))*0.95)]
	stats.P99 = sorted[int(float64(len(sorted))*0.99)]
	
	return stats
}

// Record latency for performance monitoring
func (e *HyperliquidEngine) recordLatency(operation string, startTime time.Time, success bool) {
	latency := float64(time.Since(startTime).Nanoseconds()) / 1000000.0 // Convert to milliseconds
	
	e.performanceMu.Lock()
	defer e.performanceMu.Unlock()
	
	if _, exists := e.latencies[operation]; !exists {
		e.latencies[operation] = make([]float64, 0, 1000)
	}
	
	e.latencies[operation] = append(e.latencies[operation], latency)
	
	// Keep only recent latencies (last 1000 measurements)
	if len(e.latencies[operation]) > 1000 {
		e.latencies[operation] = e.latencies[operation][len(e.latencies[operation])-1000:]
	}
}

// Stop the execution engine
func (e *HyperliquidEngine) Stop() error {
	if e.config.EnableLogging {
		log.Printf("🛑 Stopping Hyperliquid Execution Engine...")
	}
	
	e.cancel()
	close(e.stopCh)
	
	// Close WebSocket connection
	e.wsConnMu.Lock()
	if e.wsConn != nil {
		e.wsConn.Close()
	}
	e.wsConnMu.Unlock()
	
	if e.config.EnableLogging {
		log.Printf("✅ Hyperliquid Execution Engine stopped")
	}
	
	return nil
}

// Initialize account state
func (e *HyperliquidEngine) initializeAccount() error {
	// Get account info
	accountInfo, err := e.GetAccountInfo()
	if err != nil {
		return fmt.Errorf("failed to get account info: %w", err)
	}
	
	// Update internal state
	e.stateMu.Lock()
	e.balances = accountInfo.Balances
	for _, pos := range accountInfo.Positions {
		e.positions[pos.Symbol] = &pos
	}
	e.stateMu.Unlock()
	
	// Get open orders
	openOrders := e.GetOpenOrders()
	e.stateMu.Lock()
	for _, order := range openOrders {
		e.orders[order.ID] = &order
	}
	e.stateMu.Unlock()
	
	return nil
}

// Connect to Hyperliquid WebSocket
func (e *HyperliquidEngine) connectWebSocket() error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	
	conn, _, err := dialer.Dial(e.config.WebSocketURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to WebSocket: %w", err)
	}
	
	e.wsConnMu.Lock()
	e.wsConn = conn
	e.wsConnMu.Unlock()
	
	// Subscribe to user data
	subscribeMsg := map[string]interface{}{
		"method": "subscribe",
		"subscription": map[string]interface{}{
			"type": "user",
			"user": e.address,
		},
	}
	
	return e.sendWebSocketMessage(subscribeMsg)
}

// Send WebSocket message
func (e *HyperliquidEngine) sendWebSocketMessage(msg interface{}) error {
	e.wsConnMu.Lock()
	defer e.wsConnMu.Unlock()
	
	if e.wsConn == nil {
		return fmt.Errorf("WebSocket connection not established")
	}
	
	return e.wsConn.WriteJSON(msg)
}

// WebSocket message handler
func (e *HyperliquidEngine) wsMessageHandler() {
	for {
		select {
		case <-e.ctx.Done():
			return
		default:
			e.wsConnMu.RLock()
			conn := e.wsConn
			e.wsConnMu.RUnlock()
			
			if conn == nil {
				time.Sleep(time.Second)
				continue
			}
			
			_, message, err := conn.ReadMessage()
			if err != nil {
				if e.config.EnableLogging {
					log.Printf("WebSocket read error: %v", err)
				}
				e.handleWebSocketReconnect()
				continue
			}
			
			e.processWebSocketMessage(message)
		}
	}
}

// Process WebSocket message
func (e *HyperliquidEngine) processWebSocketMessage(message []byte) {
	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		if e.config.EnableLogging {
			log.Printf("Failed to unmarshal WebSocket message: %v", err)
		}
		return
	}
	
	msgType, ok := msg["channel"].(string)
	if !ok {
		return
	}
	
	switch msgType {
	case "user":
		e.handleUserUpdate(msg)
	case "orderUpdate":
		e.handleOrderUpdate(msg)
	case "positionUpdate":
		e.handlePositionUpdate(msg)
	}
}

// Handle user update from WebSocket
func (e *HyperliquidEngine) handleUserUpdate(msg map[string]interface{}) {
	data, ok := msg["data"].(map[string]interface{})
	if !ok {
		return
	}
	
	// Update account info if present
	if balances, ok := data["balances"].([]interface{}); ok {
		e.updateBalancesFromWS(balances)
	}
	
	// Update positions if present
	if positions, ok := data["positions"].([]interface{}); ok {
		e.updatePositionsFromWS(positions)
	}
}

// Handle order update from WebSocket
func (e *HyperliquidEngine) handleOrderUpdate(msg map[string]interface{}) {
	data, ok := msg["data"].(map[string]interface{})
	if !ok {
		return
	}
	
	orderID, _ := data["orderId"].(string)
	oldStatus, _ := data["oldStatus"].(string)
	newStatus, _ := data["status"].(string)
	filledQty, _ := data["filledQty"].(float64)
	avgPrice, _ := data["avgPrice"].(float64)
	
	// Update internal order state
	e.stateMu.Lock()
	if order, exists := e.orders[orderID]; exists {
		order.Status = OrderStatus(newStatus)
	}
	e.stateMu.Unlock()
	
	// Trigger callback
	if e.orderCallback != nil {
		update := OrderUpdate{
			OrderID:   orderID,
			OldStatus: OrderStatus(oldStatus),
			NewStatus: OrderStatus(newStatus),
			FilledQty: filledQty,
			AvgPrice:  avgPrice,
			Timestamp: time.Now(),
		}
		
		go func() {
			e.callbackMu.RLock()
			callback := e.orderCallback
			e.callbackMu.RUnlock()
			
			if callback != nil {
				callback(update)
			}
		}()
	}
}

// Handle position update from WebSocket
func (e *HyperliquidEngine) handlePositionUpdate(msg map[string]interface{}) {
	data, ok := msg["data"].(map[string]interface{})
	if !ok {
		return
	}
	
	symbol, _ := data["coin"].(string)
	side, _ := data["side"].(string)
	size, _ := data["szi"].(string)
	entryPrice, _ := data["entryPx"].(string)
	
	sizeFloat, _ := strconv.ParseFloat(size, 64)
	entryPriceFloat, _ := strconv.ParseFloat(entryPrice, 64)
	
	position := Position{
		Symbol:     symbol,
		Side:       side,
		Size:       sizeFloat,
		EntryPrice: entryPriceFloat,
		Timestamp:  time.Now(),
	}
	
	// Update internal position state
	e.stateMu.Lock()
	e.positions[symbol] = &position
	e.stateMu.Unlock()
	
	// Trigger callback
	if e.positionCallback != nil {
		go func() {
			e.callbackMu.RLock()
			callback := e.positionCallback
			e.callbackMu.RUnlock()
			
			if callback != nil {
				callback(position)
			}
		}()
	}
}

// Handle WebSocket reconnection
func (e *HyperliquidEngine) handleWebSocketReconnect() {
	if e.reconnectAttempts >= e.maxReconnects {
		if e.config.EnableLogging {
			log.Printf("Max reconnection attempts reached")
		}
		return
	}
	
	e.reconnectAttempts++
	
	if e.config.EnableLogging {
		log.Printf("Attempting WebSocket reconnection (%d/%d)...", 
			e.reconnectAttempts, e.maxReconnects)
	}
	
	time.Sleep(e.reconnectDelay)
	
	if err := e.connectWebSocket(); err != nil {
		if e.config.EnableLogging {
			log.Printf("Reconnection failed: %v", err)
		}
	} else {
		e.reconnectAttempts = 0
		if e.config.EnableLogging {
			log.Printf("WebSocket reconnected successfully")
		}
	}
}

// Update balances from WebSocket data
func (e *HyperliquidEngine) updateBalancesFromWS(balances []interface{}) {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	
	for _, bal := range balances {
		balanceMap, ok := bal.(map[string]interface{})
		if !ok {
			continue
		}
		
		asset, _ := balanceMap["coin"].(string)
		total, _ := strconv.ParseFloat(balanceMap["total"].(string), 64)
		hold, _ := strconv.ParseFloat(balanceMap["hold"].(string), 64)
		
		e.balances[asset] = Balance{
			Asset: asset,
			Total: total,
			Used:  hold,
			Free:  total - hold,
		}
	}
}

// Update positions from WebSocket data
func (e *HyperliquidEngine) updatePositionsFromWS(positions []interface{}) {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	
	for _, pos := range positions {
		posMap, ok := pos.(map[string]interface{})
		if !ok {
			continue
		}
		
		symbol, _ := posMap["coin"].(string)
		side, _ := posMap["side"].(string)
		size, _ := strconv.ParseFloat(posMap["szi"].(string), 64)
		entryPrice, _ := strconv.ParseFloat(posMap["entryPx"].(string), 64)
		
		e.positions[symbol] = &Position{
			Symbol:     symbol,
			Side:       side,
			Size:       size,
			EntryPrice: entryPrice,
			Timestamp:  time.Now(),
		}
	}
}

// Periodic sync with Hyperliquid API
func (e *HyperliquidEngine) periodicSync() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.syncAccountState()
		}
	}
}

// Sync account state with API
func (e *HyperliquidEngine) syncAccountState() {
	accountInfo, err := e.GetAccountInfo()
	if err != nil {
		if e.config.EnableLogging {
			log.Printf("Failed to sync account state: %v", err)
		}
		return
	}
	
	e.stateMu.Lock()
	e.balances = accountInfo.Balances
	for _, pos := range accountInfo.Positions {
		e.positions[pos.Symbol] = &pos
	}
	e.stateMu.Unlock()
}

// Heartbeat to track uptime
func (e *HyperliquidEngine) heartbeat() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.performanceMu.Lock()
			e.stats.LastHeartbeat = time.Now()
			e.stats.UptimeSeconds = int64(time.Since(e.startTime).Seconds())
			e.performanceMu.Unlock()
		}
	}
}

// Place order on Hyperliquid
func (e *HyperliquidEngine) PlaceOrder(order Order) (*OrderResult, error) {
	startTime := time.Now()
	
	// Rate limiting
	if !e.rateLimiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	
	// Risk validation
	if err := e.validateOrder(order); err != nil {
		return nil, fmt.Errorf("order validation failed: %w", err)
	}
	
	// Generate client order ID
	e.orderSeqMu.Lock()
	e.orderSeq++
	clientOrderID := fmt.Sprintf("bot_%d_%d", time.Now().Unix(), e.orderSeq)
	e.orderSeqMu.Unlock()
	
	order.ClientOrderID = clientOrderID
	
	// Convert to Hyperliquid format
	hlOrder := e.convertToHyperliquidOrder(order)
	
	// Sign and submit order
	result, err := e.submitOrder(hlOrder)
	if err != nil {
		e.recordLatency("order_placement", startTime, false)
		return nil, err
	}
	
	// Update internal state
	order.ID = result.OrderID
	order.Status = result.Status
	order.Timestamp = time.Now()
	
	e.stateMu.Lock()
	e.orders[result.OrderID] = &order
	e.stateMu.Unlock()
	
	// Update statistics
	e.performanceMu.Lock()
	e.stats.OrdersPlaced++
	if result.Status == OrderStatusFilled {
		e.stats.OrdersExecuted++
		e.stats.TotalVolume += order.Quantity * order.Price
	}
	e.performanceMu.Unlock()
	
	e.recordLatency("order_placement", startTime, true)
	
	result.Latency = time.Since(startTime)
	
	if e.config.EnableLogging {
		log.Printf("📋 Order placed: %s %s %.4f %s @ $%.4f (ID: %s)", 
			order.Side, order.Symbol, order.Quantity, order.Type, order.Price, result.OrderID)
	}
	
	return result, nil
}

// Validate order before submission
func (e *HyperliquidEngine) validateOrder(order Order) error {
	e.riskMu.RLock()
	limits := e.riskLimits
	e.riskMu.RUnlock()
	
	// Check leverage limits
	if order.Leverage > limits.MaxLeverage {
		return fmt.Errorf("leverage %.1fx exceeds maximum %.1fx", 
			order.Leverage, limits.MaxLeverage)
	}
	
	// Check position size limits
	notionalValue := order.Quantity * order.Price
	if notionalValue > limits.MaxPositionSize {
		return fmt.Errorf("position size $%.2f exceeds maximum $%.2f", 
			notionalValue, limits.MaxPositionSize)
	}
	
	// Check if stop loss is required
	if limits.StopLossRequired && order.Type != OrderTypeStopLoss && order.StopPrice == 0 {
		return fmt.Errorf("stop loss is required for this order")
	}
	
	// Check available balance
	e.stateMu.RLock()
	balances := e.balances
	e.stateMu.RUnlock()
	
	requiredMargin := (notionalValue / order.Leverage)
	if balance, exists := balances["USDC"]; exists {
		if balance.Free < requiredMargin {
			return fmt.Errorf("insufficient margin: required $%.2f, available $%.2f", 
				requiredMargin, balance.Free)
		}

	return nil

	return nil

	return nil
	}
	
	return nil
}
