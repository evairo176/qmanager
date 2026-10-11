package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type WatchdogStatus struct {
	Enabled          bool   `json:"enabled"`
	TargetHost       string `json:"target_host"`
	LastCheckTime    int64  `json:"last_check_time"`
	IsConnected      bool   `json:"is_connected"`
	ConsecutiveFails int    `json:"consecutive_fails"`
	FailThreshold    int    `json:"fail_threshold"`
	ActionTaken      string `json:"action_taken"`
}

type Watchdog struct {
	targetHost    string
	interval      time.Duration
	failThreshold int
	stopChan      chan struct{}
	mu            sync.Mutex
	status        WatchdogStatus
	pingFunc      func(host string) bool

	// recoveryFunc is invoked when the fail threshold is crossed. The parent
	// (cmd/server) wires this to real modem recovery: Tier-1 radio reset via
	// AT+CFUN, Tier-2 hard reboot. Nil recoveryFunc = no-op (old behavior).
	recoveryFunc func(fails int)
}

func NewWatchdog(targetHost string, interval time.Duration, failThreshold int) *Watchdog {
	if targetHost == "" {
		targetHost = "1.1.1.1"
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if failThreshold <= 0 {
		failThreshold = 3
	}

	w := &Watchdog{
		targetHost:    targetHost,
		interval:      interval,
		failThreshold: failThreshold,
		stopChan:      make(chan struct{}),
		status: WatchdogStatus{
			Enabled:       true,
			TargetHost:    targetHost,
			FailThreshold: failThreshold,
			IsConnected:   true,
		},
	}
	w.pingFunc = w.defaultPing
	return w
}

// SetRecoveryFunc wires a real recovery action (radio reset / reboot) that
// runs when the fail threshold is crossed. Call this before Start().
func (w *Watchdog) SetRecoveryFunc(fn func(fails int)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.recoveryFunc = fn
}

func (w *Watchdog) defaultPing(host string) bool {
	// Multi-target: cek dua host sekaligus (1.1.1.1 + 8.8.8.8).
	// Satu host aja yang mati (blackhole isp) gak boleh dianggap LOS.
	hosts := []string{host}
	if host != "8.8.8.8" {
		hosts = append(hosts, "8.8.8.8")
	}
	for _, h := range hosts {
		if w.pingOne(h) {
			return true
		}
	}
	return false
}

func (w *Watchdog) pingOne(host string) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, "53"), 3*time.Second)
	if err == nil {
		_ = conn.Close()
		return true
	}
	connHTTP, errHTTP := net.DialTimeout("tcp", net.JoinHostPort(host, "80"), 3*time.Second)
	if errHTTP == nil {
		_ = connHTTP.Close()
		return true
	}
	return false
}

// Start launches watchdog loop in a goroutine
func (w *Watchdog) Start() {
	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				w.CheckOnce()
			case <-w.stopChan:
				log.Println("[Watchdog] Stopping Watchdog daemon goroutine")
				return
			}
		}
	}()
}

// Stop cleanly terminates the watchdog loop
func (w *Watchdog) Stop() {
	close(w.stopChan)
}

func (w *Watchdog) CheckOnce() {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now().Unix()
	w.status.LastCheckTime = now

	start := time.Now()
	success := w.pingFunc(w.targetHost)
	latencyMs := float64(time.Since(start).Microseconds()) / 1000.0
	w.recordPingHistory(now, success, latencyMs)

	if success {
		w.status.IsConnected = true
		w.status.ConsecutiveFails = 0
		w.status.ActionTaken = "none"
	} else {
		w.status.ConsecutiveFails++
		w.status.IsConnected = false
		log.Printf("[Watchdog] Warning: Ping check failed for %s (fail count: %d/%d)",
			w.targetHost, w.status.ConsecutiveFails, w.failThreshold)

		if w.status.ConsecutiveFails >= w.failThreshold {
			w.status.ActionTaken = "recovery_triggered"
			log.Printf("[Watchdog] ALERT: Connection lost! Recovery threshold reached (%d consecutive fails)",
				w.status.ConsecutiveFails)
			if w.recoveryFunc != nil {
				go w.recoveryFunc(w.status.ConsecutiveFails)
			}
		}
	}

	w.writeStatusFile()
}

// recordPingHistory appends one NDJSON line to /tmp/qmanager_ping_history.json.
// Contract with frontend (use-latency-history.ts, PingHistoryEntry):
//   {"ts":..., "lat":..., "avg":..., "min":..., "max":..., "loss":..., "jit":...}
// Keep the last 720 entries (~6h at the 30s watchdog cadence).
func (w *Watchdog) recordPingHistory(ts int64, success bool, latencyMs float64) {
	type pingEntry struct {
		Ts   int64    `json:"ts"`
		Lat  *float64 `json:"lat"`
		Avg  *float64 `json:"avg"`
		Min  *float64 `json:"min"`
		Max  *float64 `json:"max"`
		Loss int      `json:"loss"`
		Jit  *float64 `json:"jit"`
	}

	var lat *float64
	if success {
		lat = &latencyMs
	}
	loss := 0
	if !success {
		loss = 100
	}
	e := pingEntry{Ts: ts, Lat: lat, Avg: lat, Min: lat, Max: lat, Loss: loss, Jit: lat}
	lineBytes, err := json.Marshal(e)
	if err != nil {
		return
	}

	histFile := "/tmp/qmanager_ping_history.json"
	var existing []string
	if data, err := os.ReadFile(histFile); err == nil {
		for _, l := range strings.Split(string(data), "\n") {
			l = strings.TrimSpace(l)
			if l != "" {
				existing = append(existing, l)
			}
		}
	}
	existing = append(existing, string(lineBytes))
	if len(existing) > 720 {
		existing = existing[len(existing)-720:]
	}
	_ = os.WriteFile(histFile, []byte(strings.Join(existing, "\n")+"\n"), 0644)
}

func (w *Watchdog) GetStatus() WatchdogStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.status
}

func (w *Watchdog) writeStatusFile() {
	data, err := json.MarshalIndent(w.status, "", "  ")
	if err != nil {
		return
	}

	tmpDir := os.TempDir()
	if _, err := os.Stat("/tmp"); err == nil {
		tmpDir = "/tmp"
	}

	targetFile := filepath.Join(tmpDir, "qmanager_watchdog_status.json")
	tmpFile := fmt.Sprintf("%s.tmp", targetFile)

	if err := os.WriteFile(tmpFile, data, 0644); err == nil {
		_ = os.Rename(tmpFile, targetFile)
	}
}

// RecordWatchcatEvent appends one NDJSON line to /tmp/qmanager_events.json so
// watchdog recovery actions surface in the UI's Recovery Activity feed
// alongside the poller events. Format matches Poller.recordEvent:
//
//	{"timestamp": <unix seconds>, "type": "watchcat_recovery|sim_failover",
//	 "severity": "info|warning|error", "message": "<human readable>"}
//
// Keeps the last 200 events (same cap as the poller feed).
func RecordWatchcatEvent(isHard bool, detail string) {
	now := time.Now().Unix()
	typ := "watchcat_recovery"
	severity := "warning"
	message := "Watchdog soft radio reset (AT+CFUN=0/1) - " + detail
	if isHard {
		severity = "error"
		message = "Watchdog hard reboot - " + detail
	}
	entry := map[string]any{
		"timestamp": now,
		"type":      typ,
		"severity":  severity,
		"message":   message,
	}
	lineBytes, err := json.Marshal(entry)
	if err != nil {
		return
	}

	histFile := "/tmp/qmanager_events.json"
	var existing []string
	if data, err := os.ReadFile(histFile); err == nil {
		for _, l := range strings.Split(string(data), "\n") {
			l = strings.TrimSpace(l)
			if l != "" {
				existing = append(existing, l)
			}
		}
	}
	existing = append(existing, string(lineBytes))
	if len(existing) > 200 {
		existing = existing[len(existing)-200:]
	}
	_ = os.WriteFile(histFile, []byte(strings.Join(existing, "\n")+"\n"), 0644)
}
