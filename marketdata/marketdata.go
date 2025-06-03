// Package marketdata provides real-time market data collection for Hyperliquid DEX
package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// MarketDataEngine defines the public interface for market data operations
type MarketDataEngine interface {
	Start() error
	Stop() error
	Subscribe(symbol string) error
	Unsubscribe(symbol string) error
	GetLatestPrice(symbol string) (float64, error)
	SetPriceCallback(callback PriceCallback)
	GetConnectionStatus() ConnectionStatus
	GetSubscribedSymbols() []string
}

// PriceCallback is called when new price data is received
type PriceCallback func(symbol string, price float64, timestamp time.Time)

// MarketDataConfig holds configuration for the market data engine
// DefaultConfig provides a default configuration for the MarketDataEngine.
var DefaultConfig = MarketDataConfig{
	HyperliquidWSURL:  "wss://api.hyperliquid.xyz/ws",
	ReconnectInterval: 5 * time.Second,
	HeartbeatInterval: 20 * time.Second,
	MaxReconnects:     10,
	Symbols:           []string{"BTC", "ETH"}, // Default symbols, can be overridden
	EnableLogging:     true,
	EnableWebSocket:   true, // Default to true
	EnableCache:       true, // Default to true
}

type MarketDataConfig struct {
	HyperliquidWSURL  string        // Hyperliquid WebSocket URL
	ReconnectInterval time.Duration // Reconnection delay
	HeartbeatInterval time.Duration // Heartbeat/ping interval
	MaxReconnects     int           // Maximum reconnection attempts
	Symbols           []string      // Initial symbols to subscribe
	EnableLogging     bool          // Detailed logging
	EnableWebSocket   bool          // Whether to use WebSocket for real-time data
	EnableCache       bool          // Whether to cache price data
}

// ConnectionStatus provides information about the WebSocket connection
type ConnectionStatus struct {
	IsConnected     bool
	LastHeartbeat   time.Time
	ReconnectCount  int
	SubscribedCount int
	MessageCount    int64
	LastMessage     time.Time
	ErrorCount      int64
}

// PriceData represents a single price point
type PriceData struct {
	Symbol    string
	Price     float64
	Volume    float64
	Timestamp time.Time
	IsValid   bool
}

// OrderBookLevel represents a single level in the order book
type OrderBookLevel struct {
	Price    float64
	Quantity float64
}

// OrderBook represents the current order book state
type OrderBook struct {
	Symbol    string
	Bids      []OrderBookLevel
	Asks      []OrderBookLevel
	Timestamp time.Time
}

// marketDataEngine implements the MarketDataEngine interface
type marketDataEngine struct {
	config              MarketDataConfig
	wsManager           *websocketManager
	priceCache          *priceCache
	subscriptionManager *subscriptionManager
	priceCallback       PriceCallback
	status              ConnectionStatus
	statusMu            sync.RWMutex
	ctx                 context.Context
	cancel              context.CancelFunc
	wg                  sync.WaitGroup
	logger              *log.Logger
}

// websocketManager handles WebSocket connection and messaging
type websocketManager struct {
	conn              *websocket.Conn
	url               string
	isConnected       bool
	reconnectAttempts int
	lastPing          time.Time
	messageQueue      chan []byte
	errorQueue        chan error
	mu                sync.RWMutex
	dialer            *websocket.Dialer
}

// priceCache stores the latest price data with thread safety
type priceCache struct {
	mu     sync.RWMutex
	prices map[string]PriceData
	maxAge time.Duration
}

// subscriptionManager handles symbol subscriptions
type subscriptionManager struct {
	mu            sync.RWMutex
	subscriptions map[string]bool
	pendingSubs   []string
}

// Hyperliquid WebSocket message structures
type SubscriptionMessage struct {
	Method       string                 `json:"method"`
	Subscription map[string]interface{} `json:"subscription"`
}

type AllMidsResponse struct {
	Channel string `json:"channel"`
	Data    struct {
		Mids map[string]string `json:"mids"`
	} `json:"data"`
}

type TradeResponse struct {
	Channel string `json:"channel"`
	Data    struct {
		Coin string `json:"coin"`
		Side string `json:"side"`
		Px   string `json:"px"`
		Sz   string `json:"sz"`
		Time int64  `json:"time"`
	} `json:"data"`
}

type OrderBookResponse struct {
	Channel string `json:"channel"`
	Data    struct {
		Coin       string     `json:"coin"`
		Levels     [][]string `json:"levels"`
		Time       int64      `json:"time"`
		IsSnapshot bool       `json:"isSnapshot"`
	} `json:"data"`
}

// NewMarketDataEngine creates a new market data engine instance
func NewMarketDataEngine(config MarketDataConfig) (MarketDataEngine, error) {
	ctx, cancel := context.WithCancel(context.Background())

	engine := &marketDataEngine{
		config: config,
		ctx:    ctx,
		cancel: cancel,
		wsManager: &websocketManager{
			url:          config.HyperliquidWSURL,
			messageQueue: make(chan []byte, 1000),
			errorQueue:   make(chan error, 100),
			dialer: &websocket.Dialer{
				HandshakeTimeout: 10 * time.Second,
				ReadBufferSize:   4096,
				WriteBufferSize:  4096,
			},
		},
		priceCache: &priceCache{
			prices: make(map[string]PriceData),
			maxAge: 30 * time.Second,
		},
		subscriptionManager: &subscriptionManager{
			subscriptions: make(map[string]bool),
			pendingSubs:   make([]string, 0),
		},
		status: ConnectionStatus{},
	}

	if config.EnableLogging {
		engine.logger = log.New(log.Writer(), "[MarketData] ", log.LstdFlags|log.Lshortfile)
	}

	return engine, nil
}

// Start initializes and starts the market data engine
func (e *marketDataEngine) Start() error {
	e.log("Starting market data engine...")

	// Start WebSocket connection
	if err := e.connect(); err != nil {
		return fmt.Errorf("failed to establish WebSocket connection: %w", err)
	}

	// Start background goroutines
	e.wg.Add(4)
	go e.messageProcessor()
	go e.heartbeatManager()
	go e.connectionMonitor()
	go e.dataValidator()

	// Subscribe to initial symbols
	for _, symbol := range e.config.Symbols {
		if err := e.Subscribe(symbol); err != nil {
			e.log("Failed to subscribe to %s: %v", symbol, err)
		}
	}

	e.updateStatus(func(status *ConnectionStatus) {
		status.IsConnected = true
	})

	e.log("Market data engine started successfully")
	return nil
}

// Stop gracefully shuts down the market data engine
func (e *marketDataEngine) Stop() error {
	e.log("Stopping market data engine...")

	e.cancel()

	// Close WebSocket connection
	e.wsManager.mu.Lock()
	if e.wsManager.conn != nil {
		e.wsManager.conn.Close()
		e.wsManager.isConnected = false
	}
	e.wsManager.mu.Unlock()

	// Wait for all goroutines to finish
	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		e.log("Market data engine stopped successfully")
		return nil
	case <-time.After(10 * time.Second):
		return errors.New("timeout waiting for goroutines to finish")
	}
}

// Subscribe adds a symbol to the subscription list
func (e *marketDataEngine) Subscribe(symbol string) error {
	e.subscriptionManager.mu.Lock()
	defer e.subscriptionManager.mu.Unlock()

	if e.subscriptionManager.subscriptions[symbol] {
		return nil // Already subscribed
	}

	// Subscribe to allMids for price data
	allMidsMsg := SubscriptionMessage{
		Method: "subscribe",
		Subscription: map[string]interface{}{
			"type": "allMids",
		},
	}

	if err := e.sendMessage(allMidsMsg); err != nil {
		return fmt.Errorf("failed to subscribe to allMids: %w", err)
	}

	// Subscribe to trades for volume data
	tradesMsg := SubscriptionMessage{
		Method: "subscribe",
		Subscription: map[string]interface{}{
			"type": "trades",
			"coin": symbol,
		},
	}

	if err := e.sendMessage(tradesMsg); err != nil {
		return fmt.Errorf("failed to subscribe to trades for %s: %w", symbol, err)
	}

	e.subscriptionManager.subscriptions[symbol] = true
	e.updateStatus(func(status *ConnectionStatus) {
		status.SubscribedCount++
	})

	e.log("Successfully subscribed to %s", symbol)
	return nil
}

// Unsubscribe removes a symbol from the subscription list
func (e *marketDataEngine) Unsubscribe(symbol string) error {
	e.subscriptionManager.mu.Lock()
	defer e.subscriptionManager.mu.Unlock()

	if !e.subscriptionManager.subscriptions[symbol] {
		return nil // Not subscribed
	}

	// Unsubscribe from trades
	unsubMsg := SubscriptionMessage{
		Method: "unsubscribe",
		Subscription: map[string]interface{}{
			"type": "trades",
			"coin": symbol,
		},
	}

	if err := e.sendMessage(unsubMsg); err != nil {
		return fmt.Errorf("failed to unsubscribe from %s: %w", symbol, err)
	}

	delete(e.subscriptionManager.subscriptions, symbol)
	e.updateStatus(func(status *ConnectionStatus) {
		status.SubscribedCount--
	})

	e.log("Successfully unsubscribed from %s", symbol)
	return nil
}

// GetLatestPrice returns the latest price for a symbol
func (e *marketDataEngine) GetLatestPrice(symbol string) (float64, error) {
	e.priceCache.mu.RLock()
	defer e.priceCache.mu.RUnlock()

	priceData, exists := e.priceCache.prices[symbol]
	if !exists {
		return 0, fmt.Errorf("no price data available for symbol: %s", symbol)
	}

	// Check if data is stale
	if time.Since(priceData.Timestamp) > e.priceCache.maxAge {
		return 0, fmt.Errorf("price data for %s is stale", symbol)
	}

	if !priceData.IsValid {
		return 0, fmt.Errorf("invalid price data for symbol: %s", symbol)
	}

	return priceData.Price, nil
}

// SetPriceCallback sets the callback function for price updates
func (e *marketDataEngine) SetPriceCallback(callback PriceCallback) {
	e.priceCallback = callback
}

// GetConnectionStatus returns the current connection status
func (e *marketDataEngine) GetConnectionStatus() ConnectionStatus {
	e.statusMu.RLock()
	defer e.statusMu.RUnlock()
	return e.status
}

// GetSubscribedSymbols returns a list of currently subscribed symbols
func (e *marketDataEngine) GetSubscribedSymbols() []string {
	e.subscriptionManager.mu.RLock()
	defer e.subscriptionManager.mu.RUnlock()

	symbols := make([]string, 0, len(e.subscriptionManager.subscriptions))
	for symbol := range e.subscriptionManager.subscriptions {
		symbols = append(symbols, symbol)
	}
	return symbols
}

// connect establishes a WebSocket connection to Hyperliquid
func (e *marketDataEngine) connect() error {
	e.wsManager.mu.Lock()
	defer e.wsManager.mu.Unlock()

	if e.wsManager.isConnected {
		return nil
	}

	e.log("Connecting to Hyperliquid WebSocket: %s", e.wsManager.url)

	conn, _, err := e.wsManager.dialer.Dial(e.wsManager.url, nil)
	if err != nil {
		return fmt.Errorf("failed to dial WebSocket: %w", err)
	}

	e.wsManager.conn = conn
	e.wsManager.isConnected = true
	e.wsManager.lastPing = time.Now()

	e.log("WebSocket connection established")
	return nil
}

// reconnect attempts to reconnect to the WebSocket
func (e *marketDataEngine) reconnect() error {
	e.wsManager.mu.Lock()
	defer e.wsManager.mu.Unlock()

	if e.wsManager.reconnectAttempts >= e.config.MaxReconnects {
		return fmt.Errorf("maximum reconnection attempts reached (%d)", e.config.MaxReconnects)
	}

	e.log("Attempting reconnection (attempt %d/%d)", e.wsManager.reconnectAttempts+1, e.config.MaxReconnects)

	// Close existing connection
	if e.wsManager.conn != nil {
		e.wsManager.conn.Close()
		e.wsManager.conn = nil
	}
	e.wsManager.isConnected = false

	// Wait before reconnecting
	time.Sleep(e.config.ReconnectInterval)

	// Attempt connection
	conn, _, err := e.wsManager.dialer.Dial(e.wsManager.url, nil)
	if err != nil {
		e.wsManager.reconnectAttempts++
		e.updateStatus(func(status *ConnectionStatus) {
			status.ReconnectCount++
			status.ErrorCount++
		})
		return fmt.Errorf("reconnection failed: %w", err)
	}

	e.wsManager.conn = conn
	e.wsManager.isConnected = true
	e.wsManager.lastPing = time.Now()
	e.wsManager.reconnectAttempts = 0

	e.updateStatus(func(status *ConnectionStatus) {
		status.IsConnected = true
		status.ReconnectCount++
	})

	// Re-subscribe to all symbols
	e.resubscribeAll()

	e.log("Reconnection successful")
	return nil
}

// resubscribeAll re-subscribes to all previously subscribed symbols
func (e *marketDataEngine) resubscribeAll() {
	e.subscriptionManager.mu.RLock()
	symbols := make([]string, 0, len(e.subscriptionManager.subscriptions))
	for symbol := range e.subscriptionManager.subscriptions {
		symbols = append(symbols, symbol)
	}
	e.subscriptionManager.mu.RUnlock()

	// Clear current subscriptions and re-subscribe
	e.subscriptionManager.mu.Lock()
	e.subscriptionManager.subscriptions = make(map[string]bool)
	e.subscriptionManager.mu.Unlock()

	for _, symbol := range symbols {
		if err := e.Subscribe(symbol); err != nil {
			e.log("Failed to re-subscribe to %s: %v", symbol, err)
		}
	}
}

// sendMessage sends a message through the WebSocket connection
func (e *marketDataEngine) sendMessage(msg interface{}) error {
	e.wsManager.mu.RLock()
	defer e.wsManager.mu.RUnlock()

	if !e.wsManager.isConnected || e.wsManager.conn == nil {
		return errors.New("WebSocket not connected")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	if err := e.wsManager.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// messageProcessor processes incoming WebSocket messages
func (e *marketDataEngine) messageProcessor() {
	defer e.wg.Done()

	for {
		select {
		case <-e.ctx.Done():
			return
		default:
			e.wsManager.mu.RLock()
			conn := e.wsManager.conn
			isConnected := e.wsManager.isConnected
			e.wsManager.mu.RUnlock()

			if !isConnected || conn == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Set read deadline
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))

			messageType, data, err := conn.ReadMessage()
			if err != nil {
				e.log("Error reading message: %v", err)
				e.updateStatus(func(status *ConnectionStatus) {
					status.ErrorCount++
					status.IsConnected = false
				})

				// Trigger reconnection
				go func() {
					if err := e.reconnect(); err != nil {
						e.log("Reconnection failed: %v", err)
					}
				}()
				continue
			}

			if messageType == websocket.TextMessage {
				e.processMessage(data)
				e.updateStatus(func(status *ConnectionStatus) {
					status.MessageCount++
					status.LastMessage = time.Now()
				})
			}
		}
	}
}

// processMessage processes a single WebSocket message
func (e *marketDataEngine) processMessage(data []byte) {
	// Try to parse as AllMids response
	var allMidsResp AllMidsResponse
	if err := json.Unmarshal(data, &allMidsResp); err == nil && allMidsResp.Channel == "allMids" {
		e.processAllMidsMessage(allMidsResp)
		return
	}

	// Try to parse as Trade response
	var tradeResp TradeResponse
	if err := json.Unmarshal(data, &tradeResp); err == nil && tradeResp.Channel == "trades" {
		e.processTradeMessage(tradeResp)
		return
	}

	// Try to parse as OrderBook response
	var orderBookResp OrderBookResponse
	if err := json.Unmarshal(data, &orderBookResp); err == nil && orderBookResp.Channel == "l2Book" {
		e.processOrderBookMessage(orderBookResp)
		return
	}

	// Handle other message types or log unknown messages
	e.log("Received unknown message type: %s", string(data))
}

// processAllMidsMessage processes allMids price data
func (e *marketDataEngine) processAllMidsMessage(resp AllMidsResponse) {
	timestamp := time.Now()

	for symbol, priceStr := range resp.Data.Mids {
		price, err := strconv.ParseFloat(priceStr, 64)
		if err != nil {
			e.log("Failed to parse price for %s: %v", symbol, err)
			continue
		}

		// Validate price
		if !e.isValidPrice(symbol, price) {
			e.log("Invalid price for %s: %.6f", symbol, price)
			continue
		}

		// Update price cache
		e.priceCache.mu.Lock()
		e.priceCache.prices[symbol] = PriceData{
			Symbol:    symbol,
			Price:     price,
			Timestamp: timestamp,
			IsValid:   true,
		}
		e.priceCache.mu.Unlock()

		// Call price callback if set
		if e.priceCallback != nil {
			e.priceCallback(symbol, price, timestamp)
		}
	}
}

// processTradeMessage processes trade data for volume information
func (e *marketDataEngine) processTradeMessage(resp TradeResponse) {
	price, err := strconv.ParseFloat(resp.Data.Px, 64)
	if err != nil {
		e.log("Failed to parse trade price: %v", err)
		return
	}

	volume, err := strconv.ParseFloat(resp.Data.Sz, 64)
	if err != nil {
		e.log("Failed to parse trade volume: %v", err)
		return
	}

	timestamp := time.Unix(resp.Data.Time/1000, (resp.Data.Time%1000)*1000000)

	// Update price cache with volume information
	e.priceCache.mu.Lock()
	if priceData, exists := e.priceCache.prices[resp.Data.Coin]; exists {
		priceData.Volume += volume
		priceData.Timestamp = timestamp
		e.priceCache.prices[resp.Data.Coin] = priceData
	} else {
		e.priceCache.prices[resp.Data.Coin] = PriceData{
			Symbol:    resp.Data.Coin,
			Price:     price,
			Volume:    volume,
			Timestamp: timestamp,
			IsValid:   true,
		}
	}
	e.priceCache.mu.Unlock()
}

// processOrderBookMessage processes order book data
func (e *marketDataEngine) processOrderBookMessage(resp OrderBookResponse) {
	// This can be extended to maintain order book state
	// For now, we'll just log the receipt
	e.log("Received order book update for %s", resp.Data.Coin)
}

// heartbeatManager manages WebSocket heartbeat/ping messages
func (e *marketDataEngine) heartbeatManager() {
	defer e.wg.Done()

	ticker := time.NewTicker(e.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.wsManager.mu.RLock()
			conn := e.wsManager.conn
			isConnected := e.wsManager.isConnected
			e.wsManager.mu.RUnlock()

			if isConnected && conn != nil {
				if err := conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
					e.log("Failed to send ping: %v", err)
					e.updateStatus(func(status *ConnectionStatus) {
						status.ErrorCount++
					})
				} else {
					e.wsManager.mu.Lock()
					e.wsManager.lastPing = time.Now()
					e.wsManager.mu.Unlock()

					e.updateStatus(func(status *ConnectionStatus) {
						status.LastHeartbeat = time.Now()
					})
				}
			}
		}
	}
}

// connectionMonitor monitors connection health and triggers reconnections
func (e *marketDataEngine) connectionMonitor() {
	defer e.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.wsManager.mu.RLock()
			lastPing := e.wsManager.lastPing
			isConnected := e.wsManager.isConnected
			e.wsManager.mu.RUnlock()

			// Check if connection is stale
			if isConnected && time.Since(lastPing) > 2*e.config.HeartbeatInterval {
				e.log("Connection appears stale, triggering reconnection")
				e.updateStatus(func(status *ConnectionStatus) {
					status.IsConnected = false
				})

				go func() {
					if err := e.reconnect(); err != nil {
						e.log("Reconnection failed: %v", err)
					}
				}()
			}
		}
	}
}

// dataValidator validates incoming price data for sanity
func (e *marketDataEngine) dataValidator() {
	defer e.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.priceCache.mu.Lock()
			now := time.Now()
			for symbol, priceData := range e.priceCache.prices {
				// Mark stale data as invalid
				if now.Sub(priceData.Timestamp) > e.priceCache.maxAge {
					priceData.IsValid = false
					e.priceCache.prices[symbol] = priceData
				}
			}
			e.priceCache.mu.Unlock()
		}
	}
}

// isValidPrice performs basic price validation
func (e *marketDataEngine) isValidPrice(symbol string, price float64) bool {
	// Basic validation rules
	if price <= 0 {
		return false
	}

	// Check for reasonable price ranges (this can be made configurable)
	switch symbol {
	case "BTC":
		return price > 1000 && price < 1000000
	case "ETH":
		return price > 10 && price < 100000
	default:
		return price > 0.000001 && price < 1000000
	}
}

// updateStatus safely updates the connection status
func (e *marketDataEngine) updateStatus(updater func(*ConnectionStatus)) {
	e.statusMu.Lock()
	defer e.statusMu.Unlock()
	updater(&e.status)
}

// log writes a log message if logging is enabled
func (e *marketDataEngine) log(format string, args ...interface{}) {
	if e.logger != nil {
		e.logger.Printf(format, args...)
	}
}
