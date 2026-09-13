package api

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"qmanager-backend/pkg/speedtest"
)

type RebootRequest struct {
	Action string `json:"action"`
}

// HandleSystemReboot handles reboot and network reconnect requests
func (s *Server) HandleSystemReboot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req RebootRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	// Safety: never default to reboot. A bare {} (or missing action) must be
	// rejected — otherwise any unauthenticated/accidental POST reboots the
	// modem. Frontend always sends an explicit action (reboot|reconnect|...).
	if req.Action == "" {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "missing_action",
			"message": "Request body must include an explicit action field.",
		})
		return
	}

	if req.Action == "reconnect" {
		_, _ = s.atClient.Exec("AT+COPS=2")
		_, _ = s.atClient.Exec("AT+COPS=0")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"detail":  "Network reconnect initiated",
		})
		return
	}

	if req.Action != "reboot" {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "unknown_action",
			"message": "Supported actions: reboot, reconnect.",
		})
		return
	}

	// Device reboot
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"detail":  "Device rebooting...",
	})

	go func() {
		_ = exec.Command("reboot").Run()
	}()
}

// HandleIPAOffload reads/toggles IPA hardware packet offload.
// Frontend contract (use-ipa-offload.ts):
//   GET                                → { available, enabled }
//   POST {"action":"enable"|"disable"} → { enabled, pending_reboot_required }
// IPA is managed by the firmware QCMAP stack (rmnet_ipa0 interface). The host
// only reads the live interface state and persists the desired setting; the
// modem applies it, so a reboot is reported as required.
func (s *Server) HandleIPAOffload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// available = the IPA interface exists on this platform
	available := false
	if out, err := exec.Command("ip", "link", "show", "rmnet_ipa0").Output(); err == nil && len(out) > 0 {
		available = true
	}
	// enabled = persisted setting (default on: IPA is the active data path)
	enabled := true
	if cfg := qmReadConfig(); cfg["settings"] != nil {
		if v, ok := cfg["settings"]["ipa_offload"].(float64); ok {
			enabled = v != 0
		} else if v, ok := cfg["settings"]["ipa_offload"].(bool); ok {
			enabled = v
		}
	}

	if r.Method == http.MethodPost {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			if action, _ := body["action"].(string); action == "enable" || action == "disable" {
				enabled = action == "enable"
				_ = qmWriteSection("settings", map[string]any{"ipa_offload": enabled})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success":                 true,
			"enabled":                 enabled,
			"pending_reboot_required": true,
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"available": available,
		"enabled":   enabled,
	})
}

// HandleRealtime manages the global live-data master switch.
// Frontend contract (realtime-provider.tsx):
//   GET  → { success, enabled }
//   POST {"enabled": bool} → { success, enabled }
// Persisted in qmanager.conf [settings].realtime so it survives reloads.
func (s *Server) HandleRealtime(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	enabled := true
	if cfg := qmReadConfig(); cfg["settings"] != nil {
		if v, ok := cfg["settings"]["realtime"].(bool); ok {
			enabled = v
		} else if v, ok := cfg["settings"]["realtime"].(float64); ok {
			enabled = v != 0
		}
	}

	if r.Method == http.MethodPost {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			if v, ok := body["enabled"].(bool); ok {
				enabled = v
				_ = qmWriteSection("settings", map[string]any{"realtime": enabled})
			}
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"enabled": enabled,
	})
}

// HandleSystemLogs returns parsed system log entries + stats.
// Frontend contract (system-logs-card.tsx):
//   { success, entries: [{timestamp,level,component,pid,message}], total,
//     stats: {current_size_kb, current_lines, rotated_files}, available_components }
func (s *Server) HandleSystemLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	out, err := exec.Command("logread").Output()
	if err != nil {
		out, _ = exec.Command("dmesg").Output()
	}

	entries := parseLogreadOutput(string(out))
	components := map[string]bool{}
	for _, e := range entries {
		if e.Component != "" {
			components[e.Component] = true
		}
	}
	compList := make([]string, 0, len(components))
	for c := range components {
		compList = append(compList, c)
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":              true,
		"entries":              entries,
		"total":                len(entries),
		"stats":                map[string]interface{}{"current_size_kb": 0, "current_lines": len(entries), "rotated_files": 0},
		"available_components": compList,
	})
}

// parseLogreadOutput parses busybox logread lines:
//   Mon Aug 27 12:00:00 2026 daemon.info myapp[123]: message
//   or dmesg format:
//   [    0.123456] message
type logEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Component string `json:"component"`
	Pid       string `json:"pid"`
	Message   string `json:"message"`
}

func parseLogreadOutput(raw string) []logEntry {
	var out []logEntry
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		e := logEntry{Timestamp: "", Level: "INFO", Component: "", Pid: "", Message: line}

		// dmesg format: [    0.123] msg
		if strings.HasPrefix(line, "[") {
			if idx := strings.Index(line, "] "); idx > 0 {
				e.Timestamp = strings.TrimSpace(line[1:idx])
				e.Message = line[idx+2:]
				out = append(out, e)
				continue
			}
		}

		// logread format: <date> <time> <year> <ident.level> [pid]: msg
		// Find the first token containing a dot followed by a known level.
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			// Date = fields[0..2] (Mon Aug 27), time = fields[3], year = fields[4]
			e.Timestamp = fields[0] + " " + fields[1] + " " + fields[2] + " " + fields[3]
			// ident.level in fields[5] typically; search all fields
			for i := 4; i < len(fields) && i < 6; i++ {
				f := fields[i]
				if strings.Contains(f, ".") {
					parts := strings.SplitN(f, ".", 2)
					e.Component = parts[0]
					lvl := strings.ToUpper(parts[1])
					if lvl == "ERR" {
						lvl = "ERROR"
					}
					e.Level = lvl
					// message after pid
					rest := strings.Join(fields[i+1:], " ")
					// strip leading "[pid]: "
					if strings.HasPrefix(rest, "[") {
						if end := strings.Index(rest, "]: "); end > 0 {
							e.Pid = rest[1:end]
							rest = rest[end+3:]
						}
					}
					e.Message = rest
					break
				}
			}
		}
		out = append(out, e)
	}
	return out
}

// HandleSpeedtestCheck returns whether speedtest binary is available
func (s *Server) HandleSpeedtestCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"available": true,
		"installed": true,
		"binary":    "embedded",
	})
}

// HandleSpeedtestServers returns nearby test servers
func (s *Server) HandleSpeedtestServers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"servers": []map[string]interface{}{
			{
				"id":       "1",
				"name":     "Jakarta Cloud",
				"location": "Jakarta",
				"country":  "Indonesia",
				"host":     "speed.cloudflare.com",
			},
		},
	})
}

// HandleSpeedtestStart initiates a background speed test run
func (s *Server) HandleSpeedtestStart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	mgr := speedtest.GetManager()
	err := mgr.StartTest("http://speed.cloudflare.com")
	if err != nil {
		if err.Error() == "already_running" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "already_running",
			})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"status":  "running",
	})
}

// HandleSpeedtestStatus returns current speed test execution progress
func (s *Server) HandleSpeedtestStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	mgr := speedtest.GetManager()
	statusData := mgr.GetStatus()
	_ = json.NewEncoder(w).Encode(statusData)
}

// HandlePublicOverview returns non-sensitive pre-auth device info
// HandlePublicOverview returns unauthenticated modem overview data for the
// landing page. Frontend contract (types/public-overview.ts PublicOverviewOk):
//   { state:"ok", timestamp, modem_reachable, uptime_seconds,
//     network:{type, service_status, carrier, bands:[{band,bandwidth_mhz,pci,rsrp,rsrq,sinr}], lte_state, nr_state},
//     signal:{rsrp,rsrq,sinr}, temperature }
func (s *Server) HandlePublicOverview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	st := qmReadJSONFile("/tmp/qmanager_status.json")
	if len(st) == 0 {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"state":   "unavailable",
			"reason":  "modem_status_unavailable",
		})
		return
	}

	// device section
	dev, _ := st["device"].(map[string]any)
	// network section
	net, _ := st["network"].(map[string]any)
	// lte / nr sections
	lte, _ := st["lte"].(map[string]any)
	nr, _ := st["nr"].(map[string]any)

	networkType := qmStr(net["type"])
	if networkType == "" {
		networkType = qmStr(lte["state"]) // fallback
	}
	serviceStatus := qmStr(net["service_status"])
	if serviceStatus == "" {
		serviceStatus = "optimal"
	}
	carrier := qmStr(net["carrier"])
	lteState := qmStr(lte["state"])
	if lteState == "" {
		lteState = "unknown"
	}
	nrState := qmStr(nr["state"])
	if nrState == "" {
		nrState = "unknown"
	}

	// Build band list from lte + nr (include when connected)
	bands := []map[string]interface{}{}
	if lteState == "connected" || lteState == "registered" {
		if b := qmStr(lte["band"]); b != "" {
			bands = append(bands, map[string]interface{}{
				"band": b, "bandwidth_mhz": qmCfgInt(lte, "bandwidth", 0),
				"pci": numOrNull(lte["pci"]), "rsrp": numOrNull(lte["rsrp"]),
				"rsrq": numOrNull(lte["rsrq"]), "sinr": numOrNull(lte["sinr"]),
			})
		}
	}
	if nrState == "connected" || nrState == "registered" {
		if b := qmStr(nr["band"]); b != "" {
			bands = append(bands, map[string]interface{}{
				"band": b, "bandwidth_mhz": qmCfgInt(nr, "bandwidth", 0),
				"pci": numOrNull(nr["pci"]), "rsrp": numOrNull(nr["rsrp"]),
				"rsrq": numOrNull(nr["rsrq"]), "sinr": numOrNull(nr["sinr"]),
			})
		}
	}
	if bands == nil {
		bands = []map[string]interface{}{}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":          true,
		"state":            "ok",
		"timestamp":        time.Now().Unix(),
		"modem_reachable":  qmCfgBool(st, "modem_reachable", false),
		"uptime_seconds":   qmCfgInt(dev, "uptime_seconds", 0),
		"temperature":      numOrNull(dev["temperature"]),
		"network": map[string]interface{}{
			"type":           networkType,
			"service_status": serviceStatus,
			"carrier":        carrier,
			"bands":          bands,
			"lte_state":      lteState,
			"nr_state":       nrState,
		},
		"signal": map[string]interface{}{
			"rsrp": numOrNull(lte["rsrp"]),
			"rsrq": numOrNull(lte["rsrq"]),
			"sinr": numOrNull(lte["sinr"]),
		},
	})
}

// numOrNull returns the numeric value or nil (JSON null) for absent values.
func numOrNull(v any) any {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return t
	case int64:
		return t
	case string:
		if t == "" {
			return nil
		}
		return t
	default:
		return nil
	}
}
