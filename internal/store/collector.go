package store

import (
	"sync"
	"time"
)

// Collector collects and records metrics periodically.
type Collector struct {
	store    *Store
	interval time.Duration
	stopChan chan struct{}
	wg       sync.WaitGroup

	// Metric sources (registered callbacks)
	sources   map[string]MetricSource
	sourcesMu sync.RWMutex
}

// MetricSource is a callback that returns current metric values.
type MetricSource func() map[string]float64

// NewCollector creates a new metrics collector.
func NewCollector(store *Store, interval time.Duration) *Collector {
	if interval < time.Second {
		interval = time.Second
	}
	return &Collector{
		store:    store,
		interval: interval,
		stopChan: make(chan struct{}),
		sources:  make(map[string]MetricSource),
	}
}

// RegisterSource registers a metric source.
func (c *Collector) RegisterSource(name string, source MetricSource) {
	c.sourcesMu.Lock()
	defer c.sourcesMu.Unlock()
	c.sources[name] = source
}

// UnregisterSource removes a metric source.
func (c *Collector) UnregisterSource(name string) {
	c.sourcesMu.Lock()
	defer c.sourcesMu.Unlock()
	delete(c.sources, name)
}

// Start begins collecting metrics.
func (c *Collector) Start() {
	c.wg.Add(1)
	go c.collectLoop()
}

// Stop stops the collector.
func (c *Collector) Stop() {
	close(c.stopChan)
	c.wg.Wait()
}

func (c *Collector) collectLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Collect immediately on start
	c.collect()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.collect()
		}
	}
}

func (c *Collector) collect() {
	c.sourcesMu.RLock()
	defer c.sourcesMu.RUnlock()

	now := time.Now()
	var metrics []MetricPoint

	for _, source := range c.sources {
		values := source()
		for name, value := range values {
			metrics = append(metrics, MetricPoint{
				Timestamp: now,
				Name:      name,
				Value:     value,
			})
		}
	}

	if len(metrics) > 0 {
		c.store.WriteBatchMetrics(metrics)
	}
}

// StandardMetrics returns common VPN metrics as a source.
type StandardMetrics struct {
	mu sync.RWMutex

	// Connection stats
	BytesSent     uint64
	BytesRecv     uint64
	PacketsSent   uint64
	PacketsRecv   uint64
	ActivePeers   int
	TotalConns    uint64
	FailedConns   uint64

	// Performance
	LatencyMs     float64
	PacketLoss    float64

	// System
	StartTime     time.Time
	LastHeartbeat time.Time
}

// NewStandardMetrics creates a new standard metrics tracker.
func NewStandardMetrics() *StandardMetrics {
	return &StandardMetrics{
		StartTime: time.Now(),
	}
}

// Update updates the metrics with new values.
func (m *StandardMetrics) Update(bytesSent, bytesRecv, packetsSent, packetsRecv uint64, peers int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BytesSent = bytesSent
	m.BytesRecv = bytesRecv
	m.PacketsSent = packetsSent
	m.PacketsRecv = packetsRecv
	m.ActivePeers = peers
	m.LastHeartbeat = time.Now()
}

// IncrementConns increments connection counters.
func (m *StandardMetrics) IncrementConns(success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalConns++
	if !success {
		m.FailedConns++
	}
}

// SetLatency sets the current latency measurement.
func (m *StandardMetrics) SetLatency(latencyMs float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LatencyMs = latencyMs
}

// SetPacketLoss sets the current packet loss percentage.
func (m *StandardMetrics) SetPacketLoss(loss float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PacketLoss = loss
}

// Source returns the metrics as a MetricSource for the collector.
func (m *StandardMetrics) Source() MetricSource {
	return func() map[string]float64 {
		m.mu.RLock()
		defer m.mu.RUnlock()

		return map[string]float64{
			"vpn.bytes_sent":      float64(m.BytesSent),
			"vpn.bytes_recv":      float64(m.BytesRecv),
			"vpn.packets_sent":    float64(m.PacketsSent),
			"vpn.packets_recv":    float64(m.PacketsRecv),
			"vpn.active_peers":    float64(m.ActivePeers),
			"vpn.total_conns":     float64(m.TotalConns),
			"vpn.failed_conns":    float64(m.FailedConns),
			"vpn.latency_ms":      m.LatencyMs,
			"vpn.packet_loss_pct": m.PacketLoss,
			"vpn.uptime_seconds":  time.Since(m.StartTime).Seconds(),
		}
	}
}

// BandwidthTracker tracks bandwidth over time windows.
type BandwidthTracker struct {
	mu            sync.RWMutex
	samples       []bandwidthSample
	maxSamples    int
	lastBytesSent uint64
	lastBytesRecv uint64
	lastTime      time.Time
}

type bandwidthSample struct {
	timestamp time.Time
	txBps     float64
	rxBps     float64
}

// NewBandwidthTracker creates a bandwidth tracker.
func NewBandwidthTracker(maxSamples int) *BandwidthTracker {
	if maxSamples <= 0 {
		maxSamples = 60 // 1 minute of samples at 1s intervals
	}
	return &BandwidthTracker{
		maxSamples: maxSamples,
		lastTime:   time.Now(),
	}
}

// Record records a bandwidth measurement.
func (b *BandwidthTracker) Record(bytesSent, bytesRecv uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastTime).Seconds()
	if elapsed <= 0 {
		return
	}

	var txBps, rxBps float64
	if b.lastTime.IsZero() {
		txBps = 0
		rxBps = 0
	} else {
		txBps = float64(bytesSent-b.lastBytesSent) / elapsed
		rxBps = float64(bytesRecv-b.lastBytesRecv) / elapsed
	}

	b.samples = append(b.samples, bandwidthSample{
		timestamp: now,
		txBps:     txBps,
		rxBps:     rxBps,
	})

	// Trim old samples
	if len(b.samples) > b.maxSamples {
		b.samples = b.samples[len(b.samples)-b.maxSamples:]
	}

	b.lastBytesSent = bytesSent
	b.lastBytesRecv = bytesRecv
	b.lastTime = now
}

// Current returns current bandwidth (bytes per second).
func (b *BandwidthTracker) Current() (txBps, rxBps float64) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if len(b.samples) == 0 {
		return 0, 0
	}

	last := b.samples[len(b.samples)-1]
	return last.txBps, last.rxBps
}

// Average returns average bandwidth over the sample window.
func (b *BandwidthTracker) Average() (txBps, rxBps float64) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if len(b.samples) == 0 {
		return 0, 0
	}

	var sumTx, sumRx float64
	for _, s := range b.samples {
		sumTx += s.txBps
		sumRx += s.rxBps
	}

	n := float64(len(b.samples))
	return sumTx / n, sumRx / n
}

// Peak returns peak bandwidth observed.
func (b *BandwidthTracker) Peak() (txBps, rxBps float64) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var maxTx, maxRx float64
	for _, s := range b.samples {
		if s.txBps > maxTx {
			maxTx = s.txBps
		}
		if s.rxBps > maxRx {
			maxRx = s.rxBps
		}
	}
	return maxTx, maxRx
}

// Source returns bandwidth metrics as a MetricSource.
func (b *BandwidthTracker) Source() MetricSource {
	return func() map[string]float64 {
		txCur, rxCur := b.Current()
		txAvg, rxAvg := b.Average()
		txPeak, rxPeak := b.Peak()

		return map[string]float64{
			"bandwidth.tx_current_bps": txCur,
			"bandwidth.rx_current_bps": rxCur,
			"bandwidth.tx_avg_bps":     txAvg,
			"bandwidth.rx_avg_bps":     rxAvg,
			"bandwidth.tx_peak_bps":    txPeak,
			"bandwidth.rx_peak_bps":    rxPeak,
		}
	}
}
