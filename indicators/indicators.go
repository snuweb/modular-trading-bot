package indicators

import (
	"errors"
	"math"
	"sync"
	"time"
)

// Public Interface
type IndicatorEngine interface {
	UpdatePrice(price float64) error
	GetSMA(period int) (float64, error)
	GetEMA(period int) (float64, error)
	GetRSI(period int) (float64, error)
	GetMACD() (MACDData, error)
	GetBollingerBands(period int, stdDev float64) (BollingerBands, error)
	GetStochastic(kPeriod, dPeriod int) (StochasticData, error)
	GetATR(period int) (float64, error)
	GetWilliamsR(period int) (float64, error)
	GetCCI(period int) (float64, error)
	GetADX(period int) (float64, error)
	GetSignals() TradingSignals
	SetSignalCallback(callback SignalCallback)
	GetIndicatorValues() IndicatorSnapshot
	Reset()
}

type SignalCallback func(signals TradingSignals)

// Core Data Types
type IndicatorConfig struct {
	MaxHistorySize   int              `json:"max_history_size"`
	SMAPeriods       []int            `json:"sma_periods"`
	EMAPeriods       []int            `json:"ema_periods"`
	RSIPeriod        int              `json:"rsi_period"`
	MACDConfig       MACDSettings     `json:"macd_config"`
	BBConfig         BBSettings       `json:"bb_config"`
	StochConfig      StochSettings    `json:"stoch_config"`
	EnableLogging    bool             `json:"enable_logging"`
	SignalThresholds SignalThresholds `json:"signal_thresholds"`
}

type MACDSettings struct {
	FastPeriod   int `json:"fast_period"`
	SlowPeriod   int `json:"slow_period"`
	SignalPeriod int `json:"signal_period"`
}

type BBSettings struct {
	Period int     `json:"period"`
	StdDev float64 `json:"std_dev"`
}

type StochSettings struct {
	KPeriod int `json:"k_period"`
	DPeriod int `json:"d_period"`
}

type SignalThresholds struct {
	RSIOverbought   float64 `json:"rsi_overbought"`
	RSIOversold     float64 `json:"rsi_oversold"`
	StochOverbought float64 `json:"stoch_overbought"`
	StochOversold   float64 `json:"stoch_oversold"`
	MACDThreshold   float64 `json:"macd_threshold"`
	ADXTrending     float64 `json:"adx_trending"`
}

type MACDData struct {
	MACD      float64   `json:"macd"`
	Signal    float64   `json:"signal"`
	Histogram float64   `json:"histogram"`
	Timestamp time.Time `json:"timestamp"`
}

type BollingerBands struct {
	Upper     float64   `json:"upper"`
	Middle    float64   `json:"middle"`
	Lower     float64   `json:"lower"`
	Timestamp time.Time `json:"timestamp"`
}

type StochasticData struct {
	K         float64   `json:"k"`
	D         float64   `json:"d"`
	Timestamp time.Time `json:"timestamp"`
}

type TradingSignals struct {
	Timestamp  time.Time              `json:"timestamp"`
	Price      float64                `json:"price"`
	Trend      TrendSignal            `json:"trend"`
	Momentum   MomentumSignal         `json:"momentum"`
	Volatility VolatilitySignal       `json:"volatility"`
	Overall    OverallSignal          `json:"overall"`
	Confidence float64                `json:"confidence"`
	Indicators map[string]interface{} `json:"indicators"`
}

type TrendSignal string

const (
	TrendBullish TrendSignal = "BULLISH"
	TrendBearish TrendSignal = "BEARISH"
	TrendNeutral TrendSignal = "NEUTRAL"
)

type MomentumSignal string

const (
	MomentumOversold   MomentumSignal = "OVERSOLD"
	MomentumOverbought MomentumSignal = "OVERBOUGHT"
	MomentumNeutral    MomentumSignal = "NEUTRAL"
)

type VolatilitySignal string

const (
	VolatilityHigh   VolatilitySignal = "HIGH"
	VolatilityLow    VolatilitySignal = "LOW"
	VolatilityNormal VolatilitySignal = "NORMAL"
)

type OverallSignal string

const (
	SignalBuy  OverallSignal = "BUY"
	SignalSell OverallSignal = "SELL"
	SignalHold OverallSignal = "HOLD"
)

type IndicatorSnapshot struct {
	Timestamp      time.Time       `json:"timestamp"`
	Price          float64         `json:"price"`
	SMA            map[int]float64 `json:"sma"`
	EMA            map[int]float64 `json:"ema"`
	RSI            float64         `json:"rsi"`
	MACD           MACDData        `json:"macd"`
	BollingerBands BollingerBands  `json:"bollinger_bands"`
	Stochastic     StochasticData  `json:"stochastic"`
	ATR            float64         `json:"atr"`
	WilliamsR      float64         `json:"williams_r"`
	CCI            float64         `json:"cci"`
	ADX            float64         `json:"adx"`
}

// Internal data structures
type priceHistory struct {
	prices  []float64
	highs   []float64
	lows    []float64
	volumes []float64
	maxSize int
	mu      sync.RWMutex
}

type RollingWindow struct {
	values   []float64
	sum      float64
	position int
	size     int
	full     bool
	mu       sync.RWMutex
}

type SignalWeights struct {
	TrendWeight      float64
	MomentumWeight   float64
	VolatilityWeight float64
}

type signalGenerator struct {
	thresholds SignalThresholds
	history    []TradingSignals
	weights    SignalWeights
	mu         sync.RWMutex
}

// Calculator interfaces
type Calculator interface {
	Update(price float64) error
	GetValue() (float64, error)
	Reset()
}

type SMACalculator struct {
	window *RollingWindow
	period int
}

type EMACalculator struct {
	period      int
	multiplier  float64
	ema         float64
	initialized bool
	mu          sync.RWMutex
}

type RSICalculator struct {
	period    int
	gains     *RollingWindow
	losses    *RollingWindow
	lastPrice float64
	firstRun  bool
}

type MACDCalculator struct {
	fastEMA   *EMACalculator
	slowEMA   *EMACalculator
	signalEMA *EMACalculator
	config    MACDSettings
}

type BBCalculator struct {
	sma    *SMACalculator
	window *RollingWindow
	config BBSettings
}

type StochCalculator struct {
	kPeriod     int
	dPeriod     int
	priceWindow *RollingWindow
	highWindow  *RollingWindow
	lowWindow   *RollingWindow
	kValues     *RollingWindow
}

type ATRCalculator struct {
	period      int
	trueRanges  *RollingWindow
	lastClose   float64
	initialized bool
}

type WilliamsCalculator struct {
	period      int
	priceWindow *RollingWindow
	highWindow  *RollingWindow
	lowWindow   *RollingWindow
}

type CCICalculator struct {
	period     int
	tpWindow   *RollingWindow
	smaTP      *SMACalculator
	deviations []float64
}

type ADXCalculator struct {
	period      int
	trueRanges  *RollingWindow
	plusDMs     *RollingWindow
	minusDMs    *RollingWindow
	plusDIs     *RollingWindow
	minusDIs    *RollingWindow
	dxValues    *RollingWindow
	lastHigh    float64
	lastLow     float64
	lastClose   float64
	initialized bool
}

// Main indicator engine implementation
type indicatorEngine struct {
	config     IndicatorConfig
	history    *priceHistory
	lastPrice  float64
	lastUpdate time.Time
	callback   SignalCallback
	mu         sync.RWMutex
	signalGen  *signalGenerator

	// Individual calculators
	smaCalculators     map[int]*SMACalculator
	emaCalculators     map[int]*EMACalculator
	rsiCalculator      *RSICalculator
	macdCalculator     *MACDCalculator
	bbCalculator       *BBCalculator
	stochCalculator    *StochCalculator
	atrCalculator      *ATRCalculator
	williamsCalculator *WilliamsCalculator
	cciCalculator      *CCICalculator
	adxCalculator      *ADXCalculator
}

// Factory function
func NewIndicatorEngine(config IndicatorConfig) IndicatorEngine {
	// Set default values if not provided
	if config.MaxHistorySize == 0 {
		config.MaxHistorySize = 1000
	}
	if len(config.SMAPeriods) == 0 {
		config.SMAPeriods = []int{9, 21, 50, 200}
	}
	if len(config.EMAPeriods) == 0 {
		config.EMAPeriods = []int{12, 26, 50}
	}
	if config.RSIPeriod == 0 {
		config.RSIPeriod = 14
	}
	if config.MACDConfig.FastPeriod == 0 {
		config.MACDConfig.FastPeriod = 12
	}
	if config.MACDConfig.SlowPeriod == 0 {
		config.MACDConfig.SlowPeriod = 26
	}
	if config.MACDConfig.SignalPeriod == 0 {
		config.MACDConfig.SignalPeriod = 9
	}
	if config.BBConfig.Period == 0 {
		config.BBConfig.Period = 20
	}
	if config.BBConfig.StdDev == 0 {
		config.BBConfig.StdDev = 2.0
	}
	if config.StochConfig.KPeriod == 0 {
		config.StochConfig.KPeriod = 14
	}
	if config.StochConfig.DPeriod == 0 {
		config.StochConfig.DPeriod = 3
	}
	if config.SignalThresholds.RSIOverbought == 0 {
		config.SignalThresholds.RSIOverbought = 70
	}
	if config.SignalThresholds.RSIOversold == 0 {
		config.SignalThresholds.RSIOversold = 30
	}
	if config.SignalThresholds.StochOverbought == 0 {
		config.SignalThresholds.StochOverbought = 80
	}
	if config.SignalThresholds.StochOversold == 0 {
		config.SignalThresholds.StochOversold = 20
	}
	if config.SignalThresholds.ADXTrending == 0 {
		config.SignalThresholds.ADXTrending = 25
	}

	engine := &indicatorEngine{
		config: config,
		history: &priceHistory{
			prices:  make([]float64, 0, config.MaxHistorySize),
			highs:   make([]float64, 0, config.MaxHistorySize),
			lows:    make([]float64, 0, config.MaxHistorySize),
			volumes: make([]float64, 0, config.MaxHistorySize),
			maxSize: config.MaxHistorySize,
		},
		smaCalculators: make(map[int]*SMACalculator),
		emaCalculators: make(map[int]*EMACalculator),
		signalGen: &signalGenerator{
			thresholds: config.SignalThresholds,
			history:    make([]TradingSignals, 0, 100),
			weights: SignalWeights{
				TrendWeight:      0.4,
				MomentumWeight:   0.4,
				VolatilityWeight: 0.2,
			},
		},
	}

	// Initialize SMA calculators
	for _, period := range config.SMAPeriods {
		engine.smaCalculators[period] = NewSMACalculator(period)
	}

	// Initialize EMA calculators
	for _, period := range config.EMAPeriods {
		engine.emaCalculators[period] = NewEMACalculator(period)
	}

	// Initialize other calculators
	engine.rsiCalculator = NewRSICalculator(config.RSIPeriod)
	engine.macdCalculator = NewMACDCalculator(config.MACDConfig)
	engine.bbCalculator = NewBBCalculator(config.BBConfig)
	engine.stochCalculator = NewStochCalculator(config.StochConfig)
	engine.atrCalculator = NewATRCalculator(14) // Standard ATR period
	engine.williamsCalculator = NewWilliamsCalculator(14)
	engine.cciCalculator = NewCCICalculator(20)
	engine.adxCalculator = NewADXCalculator(14)

	return engine
}

// RollingWindow implementation
func NewRollingWindow(size int) *RollingWindow {
	return &RollingWindow{
		values:   make([]float64, size),
		position: 0,
		size:     size,
		full:     false,
		sum:      0,
	}
}

func (rw *RollingWindow) Add(value float64) {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	if rw.full {
		rw.sum -= rw.values[rw.position]
	}

	rw.values[rw.position] = value
	rw.sum += value
	rw.position = (rw.position + 1) % rw.size

	if !rw.full && rw.position == 0 {
		rw.full = true
	}
}

func (rw *RollingWindow) Average() float64 {
	rw.mu.RLock()
	defer rw.mu.RUnlock()

	if !rw.full && rw.position == 0 {
		return 0
	}

	count := rw.size
	if !rw.full {
		count = rw.position
	}

	return rw.sum / float64(count)
}

func (rw *RollingWindow) Sum() float64 {
	rw.mu.RLock()
	defer rw.mu.RUnlock()
	return rw.sum
}

func (rw *RollingWindow) IsFull() bool {
	rw.mu.RLock()
	defer rw.mu.RUnlock()
	return rw.full
}

func (rw *RollingWindow) Count() int {
	rw.mu.RLock()
	defer rw.mu.RUnlock()
	if rw.full {
		return rw.size
	}
	return rw.position
}

func (rw *RollingWindow) GetValues() []float64 {
	rw.mu.RLock()
	defer rw.mu.RUnlock()

	values := make([]float64, 0, rw.Count())
	if rw.full {
		for i := 0; i < rw.size; i++ {
			idx := (rw.position + i) % rw.size
			values = append(values, rw.values[idx])
		}
	} else {
		for i := 0; i < rw.position; i++ {
			values = append(values, rw.values[i])
		}
	}
	return values
}

func (rw *RollingWindow) GetLatest() (float64, error) {
	rw.mu.RLock()
	defer rw.mu.RUnlock()

	if !rw.full && rw.position == 0 {
		return 0, errors.New("no data available")
	}

	latestIdx := rw.position - 1
	if latestIdx < 0 {
		latestIdx = rw.size - 1
	}

	return rw.values[latestIdx], nil
}

func (rw *RollingWindow) Max() float64 {
	rw.mu.RLock()
	defer rw.mu.RUnlock()

	if !rw.full && rw.position == 0 {
		return 0
	}

	maxVal := math.Inf(-1)
	count := rw.size
	if !rw.full {
		count = rw.position
	}

	for i := 0; i < count; i++ {
		if rw.values[i] > maxVal {
			maxVal = rw.values[i]
		}
	}

	return maxVal
}

func (rw *RollingWindow) Min() float64 {
	rw.mu.RLock()
	defer rw.mu.RUnlock()

	if !rw.full && rw.position == 0 {
		return 0
	}

	minVal := math.Inf(1)
	count := rw.size
	if !rw.full {
		count = rw.position
	}

	for i := 0; i < count; i++ {
		if rw.values[i] < minVal {
			minVal = rw.values[i]
		}
	}

	return minVal
}

func (rw *RollingWindow) StdDev() float64 {
	rw.mu.RLock()
	defer rw.mu.RUnlock()

	count := rw.Count()
	if count < 2 {
		return 0
	}

	avg := rw.sum / float64(count)
	variance := 0.0

	for i := 0; i < count; i++ {
		diff := rw.values[i] - avg
		variance += diff * diff
	}

	variance /= float64(count)
	return math.Sqrt(variance)
}

// Calculator implementations
func NewSMACalculator(period int) *SMACalculator {
	return &SMACalculator{
		window: NewRollingWindow(period),
		period: period,
	}
}

func (sma *SMACalculator) Update(price float64) error {
	sma.window.Add(price)
	return nil
}

func (sma *SMACalculator) GetValue() (float64, error) {
	if !sma.window.IsFull() {
		return 0, errors.New("insufficient data for SMA calculation")
	}
	return sma.window.Average(), nil
}

func (sma *SMACalculator) Reset() {
	sma.window = NewRollingWindow(sma.period)
}

func NewEMACalculator(period int) *EMACalculator {
	multiplier := 2.0 / (float64(period) + 1.0)
	return &EMACalculator{
		period:      period,
		multiplier:  multiplier,
		initialized: false,
	}
}

func (ema *EMACalculator) Update(price float64) error {
	ema.mu.Lock()
	defer ema.mu.Unlock()

	if !ema.initialized {
		ema.ema = price
		ema.initialized = true
	} else {
		ema.ema = (price * ema.multiplier) + (ema.ema * (1 - ema.multiplier))
	}
	return nil
}

func (ema *EMACalculator) GetValue() (float64, error) {
	ema.mu.RLock()
	defer ema.mu.RUnlock()

	if !ema.initialized {
		return 0, errors.New("EMA not initialized")
	}
	return ema.ema, nil
}

func (ema *EMACalculator) Reset() {
	ema.mu.Lock()
	defer ema.mu.Unlock()
	ema.initialized = false
	ema.ema = 0
}

func NewRSICalculator(period int) *RSICalculator {
	return &RSICalculator{
		period:   period,
		gains:    NewRollingWindow(period),
		losses:   NewRollingWindow(period),
		firstRun: true,
	}
}

func (rsi *RSICalculator) Update(price float64) error {
	if rsi.firstRun {
		rsi.lastPrice = price
		rsi.firstRun = false
		return nil
	}

	change := price - rsi.lastPrice
	gain := 0.0
	loss := 0.0

	if change > 0 {
		gain = change
	} else {
		loss = -change
	}

	rsi.gains.Add(gain)
	rsi.losses.Add(loss)
	rsi.lastPrice = price

	return nil
}

func (rsi *RSICalculator) GetValue() (float64, error) {
	if !rsi.gains.IsFull() || !rsi.losses.IsFull() {
		return 50, errors.New("insufficient data for RSI calculation")
	}

	avgGain := rsi.gains.Average()
	avgLoss := rsi.losses.Average()

	if avgLoss == 0 {
		return 100, nil
	}

	rs := avgGain / avgLoss
	rsiValue := 100 - (100 / (1 + rs))

	return rsiValue, nil
}

func (rsi *RSICalculator) Reset() {
	rsi.gains = NewRollingWindow(rsi.period)
	rsi.losses = NewRollingWindow(rsi.period)
	rsi.firstRun = true
	rsi.lastPrice = 0
}

func NewMACDCalculator(config MACDSettings) *MACDCalculator {
	return &MACDCalculator{
		fastEMA:   NewEMACalculator(config.FastPeriod),
		slowEMA:   NewEMACalculator(config.SlowPeriod),
		signalEMA: NewEMACalculator(config.SignalPeriod),
		config:    config,
	}
}

func (macd *MACDCalculator) Update(price float64) error {
	macd.fastEMA.Update(price)
	macd.slowEMA.Update(price)

	fastValue, fastErr := macd.fastEMA.GetValue()
	slowValue, slowErr := macd.slowEMA.GetValue()

	if fastErr == nil && slowErr == nil {
		macdLine := fastValue - slowValue
		macd.signalEMA.Update(macdLine)
	}

	return nil
}

func (macd *MACDCalculator) GetValue() (MACDData, error) {
	fastValue, fastErr := macd.fastEMA.GetValue()
	slowValue, slowErr := macd.slowEMA.GetValue()

	if fastErr != nil || slowErr != nil {
		return MACDData{}, errors.New("insufficient data for MACD calculation")
	}

	macdLine := fastValue - slowValue
	signalLine, signalErr := macd.signalEMA.GetValue()

	if signalErr != nil {
		signalLine = macdLine // Use MACD line as signal if not enough data
	}

	histogram := macdLine - signalLine

	return MACDData{
		MACD:      macdLine,
		Signal:    signalLine,
		Histogram: histogram,
		Timestamp: time.Now(),
	}, nil
}

func (macd *MACDCalculator) Reset() {
	macd.fastEMA.Reset()
	macd.slowEMA.Reset()
	macd.signalEMA.Reset()
}

func NewBBCalculator(config BBSettings) *BBCalculator {
	return &BBCalculator{
		sma:    NewSMACalculator(config.Period),
		window: NewRollingWindow(config.Period),
		config: config,
	}
}

func (bb *BBCalculator) Update(price float64) error {
	bb.sma.Update(price)
	bb.window.Add(price)
	return nil
}

func (bb *BBCalculator) GetValue() (BollingerBands, error) {
	if !bb.window.IsFull() {
		return BollingerBands{}, errors.New("insufficient data for Bollinger Bands calculation")
	}

	smaValue, err := bb.sma.GetValue()
	if err != nil {
		return BollingerBands{}, err
	}

	stdDev := bb.window.StdDev()
	deviation := stdDev * bb.config.StdDev

	return BollingerBands{
		Upper:     smaValue + deviation,
		Middle:    smaValue,
		Lower:     smaValue - deviation,
		Timestamp: time.Now(),
	}, nil
}

func (bb *BBCalculator) Reset() {
	bb.sma.Reset()
	bb.window = NewRollingWindow(bb.config.Period)
}

func NewStochCalculator(config StochSettings) *StochCalculator {
	return &StochCalculator{
		kPeriod:     config.KPeriod,
		dPeriod:     config.DPeriod,
		priceWindow: NewRollingWindow(config.KPeriod),
		highWindow:  NewRollingWindow(config.KPeriod),
		lowWindow:   NewRollingWindow(config.KPeriod),
		kValues:     NewRollingWindow(config.DPeriod),
	}
}

func (stoch *StochCalculator) Update(price float64) error {
	// For simplicity, assume high/low are the same as price
	// In real implementation, you'd pass high, low, close separately
	stoch.priceWindow.Add(price)
	stoch.highWindow.Add(price)
	stoch.lowWindow.Add(price)

	if stoch.priceWindow.IsFull() {
		highestHigh := stoch.highWindow.Max()
		lowestLow := stoch.lowWindow.Min()

		if highestHigh != lowestLow {
			kValue := ((price - lowestLow) / (highestHigh - lowestLow)) * 100
			stoch.kValues.Add(kValue)
		}
	}

	return nil
}

func (stoch *StochCalculator) GetValue() (StochasticData, error) {
	if !stoch.kValues.IsFull() {
		return StochasticData{}, errors.New("insufficient data for Stochastic calculation")
	}

	kValue, err := stoch.kValues.GetLatest()
	if err != nil {
		return StochasticData{}, err
	}

	dValue := stoch.kValues.Average()

	return StochasticData{
		K:         kValue,
		D:         dValue,
		Timestamp: time.Now(),
	}, nil
}

func (stoch *StochCalculator) Reset() {
	stoch.priceWindow = NewRollingWindow(stoch.kPeriod)
	stoch.highWindow = NewRollingWindow(stoch.kPeriod)
	stoch.lowWindow = NewRollingWindow(stoch.kPeriod)
	stoch.kValues = NewRollingWindow(stoch.dPeriod)
}

func NewATRCalculator(period int) *ATRCalculator {
	return &ATRCalculator{
		period:      period,
		trueRanges:  NewRollingWindow(period),
		initialized: false,
	}
}

func (atr *ATRCalculator) Update(price float64) error {
	if !atr.initialized {
		atr.lastClose = price
		atr.initialized = true
		return nil
	}

	// Simplified TR calculation (normally needs high, low, close)
	trueRange := math.Abs(price - atr.lastClose)
	atr.trueRanges.Add(trueRange)
	atr.lastClose = price

	return nil
}

func (atr *ATRCalculator) GetValue() (float64, error) {
	if !atr.trueRanges.IsFull() {
		return 0, errors.New("insufficient data for ATR calculation")
	}

	return atr.trueRanges.Average(), nil
}

func (atr *ATRCalculator) Reset() {
	atr.trueRanges = NewRollingWindow(atr.period)
	atr.initialized = false
	atr.lastClose = 0
}

func NewWilliamsCalculator(period int) *WilliamsCalculator {
	return &WilliamsCalculator{
		period:      period,
		priceWindow: NewRollingWindow(period),
		highWindow:  NewRollingWindow(period),
		lowWindow:   NewRollingWindow(period),
	}
}

func (wr *WilliamsCalculator) Update(price float64) error {
	wr.priceWindow.Add(price)
	wr.highWindow.Add(price)
	wr.lowWindow.Add(price)
	return nil
}

func (wr *WilliamsCalculator) GetValue() (float64, error) {
	if !wr.priceWindow.IsFull() {
		return 0, errors.New("insufficient data for Williams %R calculation")
	}

	highestHigh := wr.highWindow.Max()
	lowestLow := wr.lowWindow.Min()
	currentClose, err := wr.priceWindow.GetLatest()
	if err != nil {
		return 0, err
	}

	if highestHigh == lowestLow {
		return 0, nil
	}

	williamsR := ((highestHigh - currentClose) / (highestHigh - lowestLow)) * -100
	return williamsR, nil
}

func (wr *WilliamsCalculator) Reset() {
	wr.priceWindow = NewRollingWindow(wr.period)
	wr.highWindow = NewRollingWindow(wr.period)
	wr.lowWindow = NewRollingWindow(wr.period)
}

func NewCCICalculator(period int) *CCICalculator {
	return &CCICalculator{
		period:     period,
		tpWindow:   NewRollingWindow(period),
		smaTP:      NewSMACalculator(period),
		deviations: make([]float64, 0, period),
	}
}

func (cci *CCICalculator) Update(price float64) error {
	// Typical Price (simplified - normally (H+L+C)/3)
	typicalPrice := price
	cci.tpWindow.Add(typicalPrice)
	cci.smaTP.Update(typicalPrice)
	return nil
}

func (cci *CCICalculator) GetValue() (float64, error) {
	if !cci.tpWindow.IsFull() {
		return 0, errors.New("insufficient data for CCI calculation")
	}

	smaValue, err := cci.smaTP.GetValue()
	if err != nil {
		return 0, err
	}

	// Calculate mean deviation
	values := cci.tpWindow.GetValues()
	meanDeviation := 0.0
	for _, value := range values {
		meanDeviation += math.Abs(value - smaValue)
	}
	meanDeviation /= float64(len(values))

	if meanDeviation == 0 {
		return 0, nil
	}

	currentTP, err := cci.tpWindow.GetLatest()
	if err != nil {
		return 0, err
	}

	// REPLACE WITH THIS:
	cciValue := (currentTP - smaValue) / (0.015 * meanDeviation)
	return cciValue, nil
}

func (cci *CCICalculator) Reset() {
	cci.tpWindow = NewRollingWindow(cci.period)
	cci.smaTP = NewSMACalculator(cci.period)
	cci.deviations = make([]float64, 0, cci.period)
}

func NewADXCalculator(period int) *ADXCalculator {
	return &ADXCalculator{
		period:      period,
		trueRanges:  NewRollingWindow(period),
		plusDMs:     NewRollingWindow(period),
		minusDMs:    NewRollingWindow(period),
		plusDIs:     NewRollingWindow(period),
		minusDIs:    NewRollingWindow(period),
		dxValues:    NewRollingWindow(period),
		initialized: false,
	}
}

func (adx *ADXCalculator) Update(price float64) error {
	if !adx.initialized {
		adx.lastHigh = price
		adx.lastLow = price
		adx.lastClose = price
		adx.initialized = true
		return nil
	}

	// Simplified ADX calculation (normally needs H, L, C)
	high := price
	low := price
	close := price

	// True Range
	tr1 := high - low
	tr2 := math.Abs(high - adx.lastClose)
	tr3 := math.Abs(low - adx.lastClose)
	trueRange := math.Max(tr1, math.Max(tr2, tr3))
	adx.trueRanges.Add(trueRange)

	// Directional Movement
	plusDM := 0.0
	minusDM := 0.0

	if high-adx.lastHigh > adx.lastLow-low {
		plusDM = math.Max(high-adx.lastHigh, 0)
	}
	if adx.lastLow-low > high-adx.lastHigh {
		minusDM = math.Max(adx.lastLow-low, 0)
	}

	adx.plusDMs.Add(plusDM)
	adx.minusDMs.Add(minusDM)

	if adx.trueRanges.IsFull() {
		avgTR := adx.trueRanges.Average()
		avgPlusDM := adx.plusDMs.Average()
		avgMinusDM := adx.minusDMs.Average()

		if avgTR != 0 {
			plusDI := (avgPlusDM / avgTR) * 100
			minusDI := (avgMinusDM / avgTR) * 100

			adx.plusDIs.Add(plusDI)
			adx.minusDIs.Add(minusDI)

			if plusDI+minusDI != 0 {
				dx := math.Abs(plusDI-minusDI) / (plusDI + minusDI) * 100
				adx.dxValues.Add(dx)
			}
		}
	}

	adx.lastHigh = high
	adx.lastLow = low
	adx.lastClose = close

	return nil
}

func (adx *ADXCalculator) GetValue() (float64, error) {
	if !adx.dxValues.IsFull() {
		return 0, errors.New("insufficient data for ADX calculation")
	}

	return adx.dxValues.Average(), nil
}

func (adx *ADXCalculator) Reset() {
	adx.trueRanges = NewRollingWindow(adx.period)
	adx.plusDMs = NewRollingWindow(adx.period)
	adx.minusDMs = NewRollingWindow(adx.period)
	adx.plusDIs = NewRollingWindow(adx.period)
	adx.minusDIs = NewRollingWindow(adx.period)
	adx.dxValues = NewRollingWindow(adx.period)
	adx.initialized = false
}

// Main IndicatorEngine methods implementation
func (ie *indicatorEngine) UpdatePrice(price float64) error {
	ie.mu.Lock()
	defer ie.mu.Unlock()

	if price <= 0 {
		return errors.New("invalid price: must be greater than 0")
	}

	// Update price history
	ie.history.mu.Lock()
	ie.history.prices = append(ie.history.prices, price)
	ie.history.highs = append(ie.history.highs, price) // Simplified
	ie.history.lows = append(ie.history.lows, price)   // Simplified
	ie.history.volumes = append(ie.history.volumes, 0) // Placeholder

	// Maintain max history size
	if len(ie.history.prices) > ie.history.maxSize {
		ie.history.prices = ie.history.prices[1:]
		ie.history.highs = ie.history.highs[1:]
		ie.history.lows = ie.history.lows[1:]
		ie.history.volumes = ie.history.volumes[1:]
	}
	ie.history.mu.Unlock()

	// Update all calculators
	for _, sma := range ie.smaCalculators {
		sma.Update(price)
	}

	for _, ema := range ie.emaCalculators {
		ema.Update(price)
	}

	ie.rsiCalculator.Update(price)
	ie.macdCalculator.Update(price)
	ie.bbCalculator.Update(price)
	ie.stochCalculator.Update(price)
	ie.atrCalculator.Update(price)
	ie.williamsCalculator.Update(price)
	ie.cciCalculator.Update(price)
	ie.adxCalculator.Update(price)

	ie.lastPrice = price
	ie.lastUpdate = time.Now()

	// Generate and send signals if callback is set
	if ie.callback != nil {
		signals := ie.generateSignals()
		go ie.callback(signals)
	}

	return nil
}

func (ie *indicatorEngine) GetSMA(period int) (float64, error) {
	ie.mu.RLock()
	defer ie.mu.RUnlock()

	sma, exists := ie.smaCalculators[period]
	if !exists {
		return 0, errors.New("SMA calculator for this period not found")
	}

	return sma.GetValue()
}

func (ie *indicatorEngine) GetEMA(period int) (float64, error) {
	ie.mu.RLock()
	defer ie.mu.RUnlock()

	ema, exists := ie.emaCalculators[period]
	if !exists {
		return 0, errors.New("EMA calculator for this period not found")
	}

	return ema.GetValue()
}

func (ie *indicatorEngine) GetRSI(period int) (float64, error) {
	ie.mu.RLock()
	defer ie.mu.RUnlock()

	if period != ie.config.RSIPeriod {
		return 0, errors.New("RSI period mismatch")
	}

	return ie.rsiCalculator.GetValue()
}

func (ie *indicatorEngine) GetMACD() (MACDData, error) {
	ie.mu.RLock()
	defer ie.mu.RUnlock()

	return ie.macdCalculator.GetValue()
}

func (ie *indicatorEngine) GetBollingerBands(period int, stdDev float64) (BollingerBands, error) {
	ie.mu.RLock()
	defer ie.mu.RUnlock()

	if period != ie.config.BBConfig.Period || stdDev != ie.config.BBConfig.StdDev {
		return BollingerBands{}, errors.New("Bollinger Bands parameters mismatch")
	}

	return ie.bbCalculator.GetValue()
}

func (ie *indicatorEngine) GetStochastic(kPeriod, dPeriod int) (StochasticData, error) {
	ie.mu.RLock()
	defer ie.mu.RUnlock()

	if kPeriod != ie.config.StochConfig.KPeriod || dPeriod != ie.config.StochConfig.DPeriod {
		return StochasticData{}, errors.New("Stochastic parameters mismatch")
	}

	return ie.stochCalculator.GetValue()
}

func (ie *indicatorEngine) GetATR(period int) (float64, error) {
	ie.mu.RLock()
	defer ie.mu.RUnlock()

	return ie.atrCalculator.GetValue()
}

func (ie *indicatorEngine) GetWilliamsR(period int) (float64, error) {
	ie.mu.RLock()
	defer ie.mu.RUnlock()

	return ie.williamsCalculator.GetValue()
}

func (ie *indicatorEngine) GetCCI(period int) (float64, error) {
	ie.mu.RLock()
	defer ie.mu.RUnlock()

	return ie.cciCalculator.GetValue()
}

func (ie *indicatorEngine) GetADX(period int) (float64, error) {
	ie.mu.RLock()
	defer ie.mu.RUnlock()

	return ie.adxCalculator.GetValue()
}

func (ie *indicatorEngine) GetSignals() TradingSignals {
	ie.mu.RLock()
	defer ie.mu.RUnlock()

	return ie.generateSignals()
}

func (ie *indicatorEngine) SetSignalCallback(callback SignalCallback) {
	ie.mu.Lock()
	defer ie.mu.Unlock()

	ie.callback = callback
}

func (ie *indicatorEngine) GetIndicatorValues() IndicatorSnapshot {
	ie.mu.RLock()
	defer ie.mu.RUnlock()

	snapshot := IndicatorSnapshot{
		Timestamp: time.Now(),
		Price:     ie.lastPrice,
		SMA:       make(map[int]float64),
		EMA:       make(map[int]float64),
	}

	// Get SMA values
	for period, calc := range ie.smaCalculators {
		if value, err := calc.GetValue(); err == nil {
			snapshot.SMA[period] = value
		}
	}

	// Get EMA values
	for period, calc := range ie.emaCalculators {
		if value, err := calc.GetValue(); err == nil {
			snapshot.EMA[period] = value
		}
	}

	// Get other indicators
	if rsi, err := ie.rsiCalculator.GetValue(); err == nil {
		snapshot.RSI = rsi
	}

	if macd, err := ie.macdCalculator.GetValue(); err == nil {
		snapshot.MACD = macd
	}

	if bb, err := ie.bbCalculator.GetValue(); err == nil {
		snapshot.BollingerBands = bb
	}

	if stoch, err := ie.stochCalculator.GetValue(); err == nil {
		snapshot.Stochastic = stoch
	}

	if atr, err := ie.atrCalculator.GetValue(); err == nil {
		snapshot.ATR = atr
	}

	if wr, err := ie.williamsCalculator.GetValue(); err == nil {
		snapshot.WilliamsR = wr
	}

	if cci, err := ie.cciCalculator.GetValue(); err == nil {
		snapshot.CCI = cci
	}

	if adx, err := ie.adxCalculator.GetValue(); err == nil {
		snapshot.ADX = adx
	}

	return snapshot
}

func (ie *indicatorEngine) Reset() {
	ie.mu.Lock()
	defer ie.mu.Unlock()

	// Reset price history
	ie.history.mu.Lock()
	ie.history.prices = ie.history.prices[:0]
	ie.history.highs = ie.history.highs[:0]
	ie.history.lows = ie.history.lows[:0]
	ie.history.volumes = ie.history.volumes[:0]
	ie.history.mu.Unlock()

	// Reset all calculators
	for _, sma := range ie.smaCalculators {
		sma.Reset()
	}

	for _, ema := range ie.emaCalculators {
		ema.Reset()
	}

	ie.rsiCalculator.Reset()
	ie.macdCalculator.Reset()
	ie.bbCalculator.Reset()
	ie.stochCalculator.Reset()
	ie.atrCalculator.Reset()
	ie.williamsCalculator.Reset()
	ie.cciCalculator.Reset()
	ie.adxCalculator.Reset()

	// Reset signal generator
	ie.signalGen.mu.Lock()
	ie.signalGen.history = ie.signalGen.history[:0]
	ie.signalGen.mu.Unlock()

	ie.lastPrice = 0
	ie.lastUpdate = time.Time{}
}

// Signal generation logic
func (ie *indicatorEngine) generateSignals() TradingSignals {
	signals := TradingSignals{
		Timestamp:  time.Now(),
		Price:      ie.lastPrice,
		Trend:      TrendNeutral,
		Momentum:   MomentumNeutral,
		Volatility: VolatilityNormal,
		Overall:    SignalHold,
		Confidence: 0.0,
		Indicators: make(map[string]interface{}),
	}

	// Generate trend signals
	signals.Trend = ie.generateTrendSignals(&signals)

	// Generate momentum signals
	signals.Momentum = ie.generateMomentumSignals(&signals)

	// Generate volatility signals
	signals.Volatility = ie.generateVolatilitySignals(&signals)

	// Generate overall signal
	signals.Overall, signals.Confidence = ie.generateOverallSignal(signals)

	return signals
}

func (ie *indicatorEngine) generateTrendSignals(signals *TradingSignals) TrendSignal {
	// MACD trend signal
	if macd, err := ie.macdCalculator.GetValue(); err == nil {
		signals.Indicators["MACD"] = macd.MACD
		signals.Indicators["MACDSignal"] = macd.Signal
		signals.Indicators["MACDHistogram"] = macd.Histogram

		if macd.MACD > macd.Signal && macd.Histogram > 0 {
			return TrendBullish
		}
		if macd.MACD < macd.Signal && macd.Histogram < 0 {
			return TrendBearish
		}
	}

	// SMA crossover signals
	if len(ie.config.SMAPeriods) >= 2 {
		shortSMA, shortErr := ie.GetSMA(ie.config.SMAPeriods[0])
		longSMA, longErr := ie.GetSMA(ie.config.SMAPeriods[1])

		if shortErr == nil && longErr == nil {
			signals.Indicators["SMA_Short"] = shortSMA
			signals.Indicators["SMA_Long"] = longSMA

			if shortSMA > longSMA {
				return TrendBullish
			}
			if shortSMA < longSMA {
				return TrendBearish
			}
		}
	}

	// ADX trend strength
	if adx, err := ie.adxCalculator.GetValue(); err == nil {
		signals.Indicators["ADX"] = adx
		if adx > ie.config.SignalThresholds.ADXTrending {
			// Strong trend, but need other indicators for direction
			return TrendNeutral
		}
	}

	return TrendNeutral
}

func (ie *indicatorEngine) generateMomentumSignals(signals *TradingSignals) MomentumSignal {
	// RSI momentum signals
	if rsi, err := ie.rsiCalculator.GetValue(); err == nil {
		signals.Indicators["RSI"] = rsi

		if rsi >= ie.config.SignalThresholds.RSIOverbought {
			return MomentumOverbought
		}
		if rsi <= ie.config.SignalThresholds.RSIOversold {
			return MomentumOversold
		}
	}

	// Stochastic momentum signals
	if stoch, err := ie.stochCalculator.GetValue(); err == nil {
		signals.Indicators["StochK"] = stoch.K
		signals.Indicators["StochD"] = stoch.D

		if stoch.K >= ie.config.SignalThresholds.StochOverbought {
			return MomentumOverbought
		}
		if stoch.K <= ie.config.SignalThresholds.StochOversold {
			return MomentumOversold
		}
	}

	// Williams %R momentum signals
	if wr, err := ie.williamsCalculator.GetValue(); err == nil {
		signals.Indicators["WilliamsR"] = wr

		if wr >= -20 { // Overbought
			return MomentumOverbought
		}
		if wr <= -80 { // Oversold
			return MomentumOversold
		}
	}

	return MomentumNeutral
}

func (ie *indicatorEngine) generateVolatilitySignals(signals *TradingSignals) VolatilitySignal {
	// ATR volatility signals
	if atr, err := ie.atrCalculator.GetValue(); err == nil {
		signals.Indicators["ATR"] = atr

		// Calculate ATR relative to price
		atrPercent := (atr / ie.lastPrice) * 100

		if atrPercent > 3.0 { // High volatility threshold
			return VolatilityHigh
		}
		if atrPercent < 1.0 { // Low volatility threshold
			return VolatilityLow
		}
	}

	// Bollinger Bands width for volatility
	if bb, err := ie.bbCalculator.GetValue(); err == nil {
		signals.Indicators["BB_Upper"] = bb.Upper
		signals.Indicators["BB_Middle"] = bb.Middle
		signals.Indicators["BB_Lower"] = bb.Lower

		width := ((bb.Upper - bb.Lower) / bb.Middle) * 100

		if width > 10.0 { // High volatility
			return VolatilityHigh
		}
		if width < 2.0 { // Low volatility
			return VolatilityLow
		}

		// Check if price is near bands
		if ie.lastPrice >= bb.Upper*0.98 || ie.lastPrice <= bb.Lower*1.02 {
			return VolatilityHigh
		}
	}

	return VolatilityNormal
}

func (ie *indicatorEngine) generateOverallSignal(signals TradingSignals) (OverallSignal, float64) {
	score := 0.0
	confidence := 0.0

	// Trend component
	switch signals.Trend {
	case TrendBullish:
		score += ie.signalGen.weights.TrendWeight
		confidence += 0.3
	case TrendBearish:
		score -= ie.signalGen.weights.TrendWeight
		confidence += 0.3
	}

	// Momentum component
	switch signals.Momentum {
	case MomentumOversold:
		score += ie.signalGen.weights.MomentumWeight // Buy signal
		confidence += 0.4
	case MomentumOverbought:
		score -= ie.signalGen.weights.MomentumWeight // Sell signal
		confidence += 0.4
	}

	// Volatility component (affects confidence more than direction)
	switch signals.Volatility {
	case VolatilityHigh:
		confidence -= 0.2 // Reduce confidence in high volatility
	case VolatilityLow:
		confidence += 0.1 // Slightly increase confidence in low volatility
	}

	// Normalize confidence to 0-1 range
	confidence = math.Max(0, math.Min(1, confidence))

	// Determine overall signal based on score and confidence
	if score > 0.3 && confidence > 0.5 {
		return SignalBuy, confidence
	}
	if score < -0.3 && confidence > 0.5 {
		return SignalSell, confidence
	}

	return SignalHold, confidence
}

// Utility functions for price history
func (ph *priceHistory) GetLatestPrices(count int) []float64 {
	ph.mu.RLock()
	defer ph.mu.RUnlock()

	if count <= 0 || len(ph.prices) == 0 {
		return []float64{}
	}

	start := len(ph.prices) - count
	if start < 0 {
		start = 0
	}

	return append([]float64{}, ph.prices[start:]...)
}

func (ph *priceHistory) GetLatestPrice() (float64, error) {
	ph.mu.RLock()
	defer ph.mu.RUnlock()

	if len(ph.prices) == 0 {
		return 0, errors.New("no price data available")
	}

	return ph.prices[len(ph.prices)-1], nil
}

func (ph *priceHistory) GetPriceChange() (float64, error) {
	ph.mu.RLock()
	defer ph.mu.RUnlock()

	if len(ph.prices) < 2 {
		return 0, errors.New("insufficient price data for change calculation")
	}

	current := ph.prices[len(ph.prices)-1]
	previous := ph.prices[len(ph.prices)-2]

	return ((current - previous) / previous) * 100, nil
}

// Signal generator helper methods
func (sg *signalGenerator) AddSignal(signal TradingSignals) {
	sg.mu.Lock()
	defer sg.mu.Unlock()

	sg.history = append(sg.history, signal)

	// Maintain history size limit
	if len(sg.history) > 100 {
		sg.history = sg.history[1:]
	}
}

func (sg *signalGenerator) GetRecentSignals(count int) []TradingSignals {
	sg.mu.RLock()
	defer sg.mu.RUnlock()

	if count <= 0 || len(sg.history) == 0 {
		return []TradingSignals{}
	}

	start := len(sg.history) - count
	if start < 0 {
		start = 0
	}

	return append([]TradingSignals{}, sg.history[start:]...)
}

// Default configuration function
func DefaultConfig() IndicatorConfig {
	return IndicatorConfig{
		MaxHistorySize: 1000,
		SMAPeriods:     []int{9, 21, 50, 200},
		EMAPeriods:     []int{12, 26, 50},
		RSIPeriod:      14,
		MACDConfig: MACDSettings{
			FastPeriod:   12,
			SlowPeriod:   26,
			SignalPeriod: 9,
		},
		BBConfig: BBSettings{
			Period: 20,
			StdDev: 2.0,
		},
		StochConfig: StochSettings{
			KPeriod: 14,
			DPeriod: 3,
		},
		EnableLogging: true,
		SignalThresholds: SignalThresholds{
			RSIOverbought:   70,
			RSIOversold:     30,
			StochOverbought: 80,
			StochOversold:   20,
			MACDThreshold:   0,
			ADXTrending:     25,
		},
	}
}
