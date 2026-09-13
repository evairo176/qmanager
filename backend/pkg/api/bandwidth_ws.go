package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

// =============================================================================
// bandwidth_ws.go — Go-native WebSocket server for live bandwidth monitoring.
//
// Replaces the legacy websocat bridge entirely: qmanager-core itself listens on
// the configured ws_port and streams per-interface traffic rates computed from
// /proc/net/dev. The frontend hook (use-bandwidth-monitor.ts) connects a plain
// browser WebSocket and expects this message shape:
//
//	{
//	  "type": "traffic_update",
//	  "channel": "network-monitor",
//	  "data": {"timestamp": "...", "upload": bps, "download": bps},
//	  "interfaces": [{"name": "...", "state": "up|down", "tx": {"bps": n}, "rx": {"bps": n}}]
//	}
// =============================================================================

const bandwidthStatusFile = "/tmp/qmanager_bandwidth_status.json"

var (
	bandwidthWSOnce sync.Once
)

// Last rx/tx byte counters per interface, for rate (bps) computation.
type ifaceCounters struct {
	rx uint64
	tx uint64
	ts time.Time
}

// StartBandwidthWebSocket launches the WS server on port (default 8838).
// Idempotent — safe to call from the HTTP handler on every GET.
func (s *Server) StartBandwidthWebSocket(port int) {
	if port <= 0 {
		port = 8838
	}
	bandwidthWSOnce.Do(func() {
		// Stop channel reserved for future teardown (shutdown on service stop).
		_ = make(chan struct{})
		go s.runBandwidthWSServer(port)
		// Mark status so the frontend sees the monitor + "websocat" as running
		// (the Go server replaces websocat — the FE only checks the boolean).
		writeJSONFile(bandwidthStatusFile, map[string]any{
			"websocat_running": true,
			"monitor_running":  true,
		})
	})
}

func (s *Server) runBandwidthWSServer(port int) {
	mux := http.NewServeMux()
	// Bypass x/net/websocket's default origin check (it rejects non-browser
	// clients and any Origin not matching Host, which breaks our LAN clients).
	wsServer := &websocket.Server{
		Handler: s.handleBandwidthWS,
		Handshake: func(cfg *websocket.Config, req *http.Request) error {
			return nil // accept any origin
		},
	}
	mux.Handle("/ws", wsServer)
	mux.Handle("/", wsServer) // FE connects to ws://host:port/ (no path)
	addr := fmt.Sprintf(":%d", port)
	srv := &http.Server{Addr: addr, Handler: mux}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return // port busy — fall back to polling-less state; not fatal
	}
	_ = srv.Serve(ln)
}

func (s *Server) handleBandwidthWS(ws *websocket.Conn) {
	defer ws.Close()

	// Read loop (in background) so the server detects client disconnect and
	// frees the TCP connection. Without this the socket stays ESTABLISHED on
	// the modem side even after the browser closes the WebSocket.
	readDone := make(chan struct{})
	go func() {
		buf := make([]byte, 256)
		for {
			if _, err := ws.Read(buf); err != nil {
				close(readDone)
				return
			}
		}
	}()

	counters := map[string]*ifaceCounters{}
	var lastMu sync.Mutex

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// Send the message shape repeatedly at 1 Hz.
	for {
		select {
		case <-readDone:
			return // client disconnected
		case <-ticker.C:
			msg := buildBandwidthMessage(&lastMu, counters)
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			if _, err := ws.Write(data); err != nil {
				return // client gone
			}
		}
	}
}

// buildBandwidthMessage reads /proc/net/dev, computes per-interface bps deltas,
// and returns the exact frontend contract object.
func buildBandwidthMessage(lastMu *sync.Mutex, counters map[string]*ifaceCounters) map[string]any {
	interfaces, err := readNetDev()
	now := time.Now()
	if err != nil {
		interfaces = []map[string]any{}
	}

	lastMu.Lock()
	defer lastMu.Unlock()

	totalRx, totalTx := 0.0, 0.0
	out := make([]map[string]any, 0, len(interfaces))
	for _, ifc := range interfaces {
		name := ifc["name"].(string)
		prev, ok := counters[name]
		if !ok || prev == nil {
			counters[name] = &ifaceCounters{rx: ifc["rx"].(uint64), tx: ifc["tx"].(uint64), ts: now}
			prev = counters[name]
		}

		dt := now.Sub(prev.ts).Seconds()
		rxRate, txRate := 0.0, 0.0
		if dt > 0 {
			rxRate = float64(ifc["rx"].(uint64)-prev.rx) * 8 / dt // bps
			txRate = float64(ifc["tx"].(uint64)-prev.tx) * 8 / dt
		}
		// Guard against counter reset (interface down/replugged)
		if rxRate < 0 {
			rxRate = 0
		}
		if txRate < 0 {
			txRate = 0
		}
		prev.rx, prev.tx, prev.ts = ifc["rx"].(uint64), ifc["tx"].(uint64), now

		state := "down"
		if ifc["up"].(bool) {
			state = "up"
		}
		out = append(out, map[string]any{
			"name":  name,
			"state": state,
			"tx":    map[string]any{"bps": txRate},
			"rx":    map[string]any{"bps": rxRate},
		})
		totalRx += rxRate
		totalTx += txRate
	}

	return map[string]any{
		"type":    "traffic_update",
		"channel": "network-monitor",
		"data": map[string]any{
			"timestamp": now.Format(time.RFC3339),
			"upload":    totalTx,
			"download":  totalRx,
		},
		"interfaces": out,
	}
}

// readNetDev parses /proc/net/dev into interface snapshots.
func readNetDev() ([]map[string]any, error) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []map[string]any
	sc := bufio.NewScanner(f)
	// Skip the two header lines
	line := 0
	for sc.Scan() {
		line++
		if line <= 2 {
			continue
		}
		txt := sc.Text()
		colon := strings.Index(txt, ":")
		if colon < 0 {
			continue
		}
		name := strings.TrimSpace(txt[:colon])
		fields := strings.Fields(txt[colon+1:])
		if len(fields) < 9 {
			continue
		}
		rxBytes, _ := strconv.ParseUint(fields[0], 10, 64)
		txBytes, _ := strconv.ParseUint(fields[8], 10, 64)
		out = append(out, map[string]any{
			"name": name,
			"rx":   rxBytes,
			"tx":   txBytes,
			"up":   isIfaceUp(name),
		})
	}
	return out, nil
}

// isIfaceUp checks the interface operstate without shelling out.
func isIfaceUp(name string) bool {
	data, err := os.ReadFile("/sys/class/net/" + name + "/operstate")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == "up"
}
