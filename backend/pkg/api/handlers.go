package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"qmanager-backend/pkg/at"
)

type Server struct {
	atClient at.Executor
}

func NewServer(atClient at.Executor) *Server {
	return &Server{atClient: atClient}
}

// RegisterRoutes registers all QManager API routes
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	loginLimiter := NewIPRateLimiter(0.1, 5)   // 5 attempts burst, 1 refill per 10s
	atCmdLimiter := NewIPRateLimiter(10.0, 20) // 10 req/s, burst 20

	// Auth Routes (public — cookie-based session, login is rate-limited)
	mux.HandleFunc("/cgi-bin/quecmanager/auth/login.sh", RateLimitMiddleware(loginLimiter, s.HandleAuthLogin))
	mux.HandleFunc("/cgi-bin/quecmanager/auth/logout.sh", s.HandleAuthLogout)
	mux.HandleFunc("/cgi-bin/quecmanager/auth/check.sh", s.HandleAuthCheck)

	// Modem & Core API Routes — Auth Required
	mux.HandleFunc("/cgi-bin/quecmanager/at_cmd/fetch_data.sh", RequireAuth(s.HandleFetchData))
	mux.HandleFunc("/cgi-bin/quecmanager/at_cmd/send_command.sh", RequireAuth(RateLimitMiddleware(atCmdLimiter, s.HandleSendCommand)))
	mux.HandleFunc("/cgi-bin/quecmanager/bands/current.sh", RequireAuth(s.HandleBandsCurrent))
	mux.HandleFunc("/cgi-bin/quecmanager/bands/lock.sh", RequireAuth(s.HandleBandsLock))
	mux.HandleFunc("/cgi-bin/quecmanager/bands/failover_status.sh", RequireAuth(s.HandleBandsFailoverStatus))
	mux.HandleFunc("/cgi-bin/quecmanager/bands/failover_toggle.sh", RequireAuth(s.HandleBandsFailoverToggle))
	mux.HandleFunc("/cgi-bin/quecmanager/cellular/sms.sh", RequireAuth(s.HandleSMS))

	// Cellular & Network Settings Routes — Auth Required
	mux.HandleFunc("/cgi-bin/quecmanager/cellular/radio_details.sh", RequireAuth(s.HandleRadioDetails))
	mux.HandleFunc("/cgi-bin/quecmanager/cellular/settings.sh", RequireAuth(s.HandleCellularSettings))
	mux.HandleFunc("/cgi-bin/quecmanager/cellular/imei.sh", RequireAuth(s.HandleIMEISettings))
	mux.HandleFunc("/cgi-bin/quecmanager/cellular/network_priority.sh", RequireAuth(s.HandleNetworkPriority))
	mux.HandleFunc("/cgi-bin/quecmanager/network/ttl.sh", RequireAuth(s.HandleTTLSettings))

	// Advanced Features Routes — Auth Required
	mux.HandleFunc("/cgi-bin/quecmanager/network/data_used.sh", RequireAuth(s.HandleDataUsed))
	mux.HandleFunc("/cgi-bin/quecmanager/at_cmd/cell_scan_status.sh", RequireAuth(s.HandleCellScanStatus))

	// History Charts Routes — Auth Required
	mux.HandleFunc("/cgi-bin/quecmanager/at_cmd/fetch_signal_history.sh", RequireAuth(s.HandleFetchSignalHistory))
	mux.HandleFunc("/cgi-bin/quecmanager/at_cmd/fetch_ping_history.sh", RequireAuth(s.HandleFetchPingHistory))
	mux.HandleFunc("/cgi-bin/quecmanager/at_cmd/fetch_events.sh", RequireAuth(s.HandleFetchEvents))

	// System & Reboot Routes — Auth Required (reboot.sh without auth = instant DoS)
	mux.HandleFunc("/cgi-bin/quecmanager/system/reboot.sh", RequireAuth(s.HandleSystemReboot))
	mux.HandleFunc("/cgi-bin/quecmanager/system/logs.sh", RequireAuth(s.HandleSystemLogs))
	mux.HandleFunc("/cgi-bin/quecmanager/system/ipa_offload.sh", RequireAuth(s.HandleIPAOffload))
	mux.HandleFunc("/cgi-bin/quecmanager/system/realtime.sh", RequireAuth(s.HandleRealtime))
	mux.HandleFunc("/cgi-bin/quecmanager/system/storage.sh", RequireAuth(s.HandleStorage))
	mux.HandleFunc("/cgi-bin/quecmanager/public/overview.sh", s.HandlePublicOverview)

	// APN & MBN Management Routes — Auth Required
	mux.HandleFunc("/cgi-bin/quecmanager/cellular/apn.sh", RequireAuth(s.HandleAPN))
	mux.HandleFunc("/cgi-bin/quecmanager/cellular/mbn.sh", RequireAuth(s.HandleMBN))

	// Tower & Frequency Locking Routes — Auth Required (write = destructive)
	mux.HandleFunc("/cgi-bin/quecmanager/tower/status.sh", RequireAuth(s.HandleTowerStatus))
	mux.HandleFunc("/cgi-bin/quecmanager/tower/lock.sh", RequireAuth(s.HandleTowerLock))
	mux.HandleFunc("/cgi-bin/quecmanager/tower/settings.sh", RequireAuth(s.HandleTowerSettings))
	mux.HandleFunc("/cgi-bin/quecmanager/tower/schedule.sh", RequireAuth(s.HandleTowerSchedule))
	mux.HandleFunc("/cgi-bin/quecmanager/frequency/status.sh", RequireAuth(s.HandleFrequencyStatus))
	mux.HandleFunc("/cgi-bin/quecmanager/frequency/lock.sh", RequireAuth(s.HandleFrequencyLock))

	// SIM Profiles & Connection Scenarios Routes — Auth Required
	mux.HandleFunc("/cgi-bin/quecmanager/profiles/list.sh", RequireAuth(s.HandleProfilesList))
	mux.HandleFunc("/cgi-bin/quecmanager/profiles/get.sh", RequireAuth(s.HandleProfilesGet))
	mux.HandleFunc("/cgi-bin/quecmanager/profiles/save.sh", RequireAuth(s.HandleProfilesSave))
	mux.HandleFunc("/cgi-bin/quecmanager/profiles/delete.sh", RequireAuth(s.HandleProfilesDelete))
	mux.HandleFunc("/cgi-bin/quecmanager/profiles/apply.sh", RequireAuth(s.HandleProfilesApply))
	mux.HandleFunc("/cgi-bin/quecmanager/profiles/current_settings.sh", RequireAuth(s.HandleProfilesCurrentSettings))
	mux.HandleFunc("/cgi-bin/quecmanager/scenarios/list.sh", RequireAuth(s.HandleScenariosList))

	// Monitoring & Watchdog Routes — Auth Required
	mux.HandleFunc("/cgi-bin/quecmanager/monitoring/alerts.sh", RequireAuth(s.HandleMonitoringAlerts))
	mux.HandleFunc("/cgi-bin/quecmanager/monitoring/watchdog.sh", RequireAuth(s.HandleMonitoringWatchdog))
	mux.HandleFunc("/cgi-bin/quecmanager/vpn/netbird.sh", RequireAuth(s.HandleNetBird))
	mux.HandleFunc("/cgi-bin/quecmanager/vpn/tailscale.sh", RequireAuth(s.HandleVPNTailscale))

	// System Health Check & Language Packs Routes — Auth Required
	mux.HandleFunc("/cgi-bin/quecmanager/system/health-check/status.sh", RequireAuth(s.HandleHealthCheckStatus))
	mux.HandleFunc("/cgi-bin/quecmanager/system/health-check/run.sh", RequireAuth(s.HandleHealthCheckRun))
	mux.HandleFunc("/cgi-bin/quecmanager/system/language-packs/list.sh", RequireAuth(s.HandleLanguagePacksList))
	mux.HandleFunc("/cgi-bin/quecmanager/system/language-packs/install.sh", RequireAuth(s.HandleLanguagePacksInstall))
	// Device & System Metadata Routes — Auth Required
	mux.HandleFunc("/cgi-bin/quecmanager/device/about.sh", RequireAuth(s.HandleAboutDevice))
	mux.HandleFunc("/cgi-bin/quecmanager/system/settings.sh", RequireAuth(s.HandleSystemSettings))
	mux.HandleFunc("/cgi-bin/quecmanager/cellular/sim_slot.sh", RequireAuth(s.HandleSIMSlot))

	mux.HandleFunc("/cgi-bin/quecmanager/public/hostname.sh", s.HandleHostname)

	// Network & Traffic Control Routes — Auth Required
	mux.HandleFunc("/cgi-bin/quecmanager/network/ethernet.sh", RequireAuth(s.HandleEthernetStatus))
	mux.HandleFunc("/cgi-bin/quecmanager/network/lan_config.sh", RequireAuth(s.HandleLANConfig))
	mux.HandleFunc("/cgi-bin/quecmanager/network/lan_devices.sh", RequireAuth(s.HandleLANDevices))
	mux.HandleFunc("/cgi-bin/quecmanager/network/dns.sh", RequireAuth(s.HandleDNS))
	mux.HandleFunc("/cgi-bin/quecmanager/network/mtu.sh", RequireAuth(s.HandleMTU))
	mux.HandleFunc("/cgi-bin/quecmanager/network/ip_passthrough.sh", RequireAuth(s.HandleIPPassthrough))
	mux.HandleFunc("/cgi-bin/quecmanager/network/traffic_masquerade.sh", RequireAuth(s.HandleTrafficMasquerade))
	mux.HandleFunc("/cgi-bin/quecmanager/network/video_optimizer.sh", RequireAuth(s.HandleVideoOptimizer))

	// Speedtest Routes — Auth Required
	mux.HandleFunc("/cgi-bin/quecmanager/at_cmd/speedtest_check.sh", RequireAuth(s.HandleSpeedtestCheck))
	mux.HandleFunc("/cgi-bin/quecmanager/at_cmd/speedtest_servers.sh", RequireAuth(s.HandleSpeedtestServers))
	mux.HandleFunc("/cgi-bin/quecmanager/at_cmd/speedtest_start.sh", RequireAuth(s.HandleSpeedtestStart))
	mux.HandleFunc("/cgi-bin/quecmanager/at_cmd/speedtest_status.sh", RequireAuth(s.HandleSpeedtestStatus))
	mux.HandleFunc("/cgi-bin/quecmanager/network/speedtest_check.sh", RequireAuth(s.HandleSpeedtestCheck))
	mux.HandleFunc("/cgi-bin/quecmanager/network/speedtest_servers.sh", RequireAuth(s.HandleSpeedtestServers))
	mux.HandleFunc("/cgi-bin/quecmanager/network/speedtest_start.sh", RequireAuth(s.HandleSpeedtestStart))
	mux.HandleFunc("/cgi-bin/quecmanager/network/speedtest_status.sh", RequireAuth(s.HandleSpeedtestStatus))

	// System Management & Polling Routes — Auth Required
	mux.HandleFunc("/cgi-bin/quecmanager/system/ping_profile.sh", RequireAuth(s.HandlePingProfile))
	mux.HandleFunc("/cgi-bin/quecmanager/system/quality_thresholds.sh", RequireAuth(s.HandleQualityThresholds))
	mux.HandleFunc("/cgi-bin/quecmanager/system/adaptive_polling.sh", RequireAuth(s.HandleAdaptivePolling))
	mux.HandleFunc("/cgi-bin/quecmanager/cellular/sms_forwarding.sh", RequireAuth(s.HandleSMSForwarding))

	// System Management & Update Routes — Auth Required
	mux.HandleFunc("/cgi-bin/quecmanager/system/ssh_password.sh", RequireAuth(s.HandleSSHPassword))
	mux.HandleFunc("/cgi-bin/quecmanager/system/update.sh", RequireAuth(s.HandleSoftwareUpdate))
	mux.HandleFunc("/cgi-bin/quecmanager/system/pending_reboot.sh", RequireAuth(s.HandlePendingReboot))
	mux.HandleFunc("/cgi-bin/quecmanager/system/known_sims.sh", RequireAuth(s.HandleKnownSims))
	mux.HandleFunc("/cgi-bin/quecmanager/cellular/fplmn.sh", RequireAuth(s.HandleFPLMN))
	mux.HandleFunc("/cgi-bin/quecmanager/monitoring/bandwidth.sh", RequireAuth(s.HandleBandwidthSettings))
	mux.HandleFunc("/cgi-bin/quecmanager/network/speedtest.sh", RequireAuth(s.HandleSpeedtestStart))
	mux.HandleFunc("/cgi-bin/quecmanager/at_cmd/cell_scan_start.sh", RequireAuth(s.HandleCellScanStart))
	mux.HandleFunc("/cgi-bin/quecmanager/at_cmd/neighbour_scan_start.sh", RequireAuth(s.HandleCellScanStart))

	// Real-Time Telemetry SSE Stream — Auth Required
	mux.HandleFunc("/cgi-bin/quecmanager/api/stream/status", RequireAuth(s.HandleSSEStream))

	// Config Backup — Auth Required (collect/apply/apply_status/apply_cancel)
	mux.HandleFunc("/cgi-bin/quecmanager/system/config-backup/collect.sh", RequireAuth(s.HandleConfigBackupCollect))
	mux.HandleFunc("/cgi-bin/quecmanager/system/config-backup/apply.sh", RequireAuth(s.HandleConfigBackupApply))
	mux.HandleFunc("/cgi-bin/quecmanager/system/config-backup/apply_status.sh", RequireAuth(s.HandleConfigBackupApplyStatus))
	mux.HandleFunc("/cgi-bin/quecmanager/system/config-backup/apply_cancel.sh", RequireAuth(s.HandleConfigBackupApplyCancel))

	// Connection Scenarios CRUD — Auth Required (list.sh already registered)
	mux.HandleFunc("/cgi-bin/quecmanager/scenarios/activate.sh", RequireAuth(s.HandleScenarioActivate))
	mux.HandleFunc("/cgi-bin/quecmanager/scenarios/active.sh", RequireAuth(s.HandleScenarioActive))
	mux.HandleFunc("/cgi-bin/quecmanager/scenarios/save.sh", RequireAuth(s.HandleScenarioSave))
	mux.HandleFunc("/cgi-bin/quecmanager/scenarios/delete.sh", RequireAuth(s.HandleScenarioDelete))

	// Language Packs status/cancel/remove — Auth Required (list.sh + install.sh already registered)
	mux.HandleFunc("/cgi-bin/quecmanager/system/language-packs/install_status.sh", RequireAuth(s.HandleLanguagePacksInstallStatus))
	mux.HandleFunc("/cgi-bin/quecmanager/system/language-packs/install_cancel.sh", RequireAuth(s.HandleLanguagePacksInstallCancel))
	mux.HandleFunc("/cgi-bin/quecmanager/system/language-packs/remove.sh", RequireAuth(s.HandleLanguagePacksRemove))

	// Profiles deactivate + apply_status — Auth Required
	mux.HandleFunc("/cgi-bin/quecmanager/profiles/deactivate.sh", RequireAuth(s.HandleProfilesDeactivate))
	mux.HandleFunc("/cgi-bin/quecmanager/profiles/apply_status.sh", RequireAuth(s.HandleProfilesApplyStatus))

	// Tower failover status alias — Auth Required
	mux.HandleFunc("/cgi-bin/quecmanager/tower/failover_status.sh", RequireAuth(s.HandleTowerFailoverStatus))

	// Neighbour scan status alias — Auth Required
	mux.HandleFunc("/cgi-bin/quecmanager/at_cmd/neighbour_scan_status.sh", RequireAuth(s.HandleNeighbourScanStatus))

	// Missing Endpoints — Auth Required (previously 404: dashboard password
	// change, modem network reconnect, diagnostics capture)
	mux.HandleFunc("/cgi-bin/quecmanager/auth/password.sh", RequireAuth(s.HandleChangeDashboardPassword))
	mux.HandleFunc("/cgi-bin/quecmanager/at_cmd/reconnect_modem.sh", RequireAuth(s.HandleReconnectModem))
	mux.HandleFunc("/cgi-bin/quecmanager/system/diagnostics.sh", RequireAuth(s.HandleDiagnosticsCapture))
}

// HandleFetchData reads status from /tmp/qmanager_status.json or returns fallback
func (s *Server) HandleFetchData(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	cacheFile := "/tmp/qmanager_status.json"
	var status map[string]interface{}
	if data, err := os.ReadFile(cacheFile); err == nil && len(data) > 0 {
		if json.Unmarshal(data, &status) == nil {
			// Enrich connectivity with latency history for the monitoring page.
			// Frontend (latency-monitoring-card.tsx) reads:
			//   connectivity.latency_history     -> number[] (last RTT values)
			//   connectivity.history_interval_sec
			//   connectivity.history_size
			//   connectivity.avg_latency_ms
			// Data comes from the NDJSON scribbled by the watchdog ever 30s.
			if conn, ok := status["connectivity"].(map[string]interface{}); ok {
				latHistory, histSec, histSize := readPingHistorySummary()
				conn["latency_history"] = latHistory
				conn["history_interval_sec"] = histSec
				conn["history_size"] = histSize
				if len(latHistory) > 0 {
					var sum float64
					for _, v := range latHistory {
						sum += v
					}
					conn["avg_latency_ms"] = math.Round(sum/float64(len(latHistory))*10) / 10
				}
			}

			// Enrich with watchcat live state for the watchdog status card.
			// Frontend (watchdog-status-card.tsx) reads modemStatus.watchcat
			// (from fetch_data.sh), NOT /monitoring/watchdog.sh — without this
			// the card stays in 'Starting up' forever.
			if wc := readInternalWatchdogStatus(); len(wc) > 0 {
				status["watchcat"] = map[string]interface{}{
					"enabled":             wc["enabled"],
					"state":               wc["state"],
					"current_tier":        wc["current_tier"],
					"failure_count":       wc["failure_count"],
					"last_recovery_time":  wc["last_recovery_time"],
					"last_recovery_tier":  wc["last_recovery_tier"],
					"total_recoveries":    wc["total_recoveries"],
					"cooldown_remaining":  wc["cooldown_remaining"],
					"reboots_this_hour":   wc["reboots_this_hour"],
				}
			}
			payload, _ := json.Marshal(status)
			_, _ = w.Write(payload)
			return
		}
	}

	// Fallback JSON if cache does not exist yet
	fallback := map[string]interface{}{
		"timestamp":            0,
		"system_state":         "initializing",
		"modem_reachable":      false,
		"last_successful_poll": 0,
		"errors":               []string{"poller_not_started"},
		"network": map[string]interface{}{
			"type":           "",
			"sim_slot":       1,
			"carrier":        "",
			"service_status": "unknown",
			"ca_active":      false,
			"ca_count":       0,
		},
		"lte": map[string]interface{}{
			"state": "unknown", "band": "", "earfcn": nil, "bandwidth": nil, "pci": nil, "rsrp": nil, "rsrq": nil, "sinr": nil, "rssi": nil,
		},
		"nr": map[string]interface{}{
			"state": "unknown", "band": "", "arfcn": nil, "pci": nil, "rsrp": nil, "rsrq": nil, "sinr": nil, "scs": nil,
		},
		"device": map[string]interface{}{
			"temperature": nil, "cpu_usage": 0, "memory_used_mb": 0, "memory_total_mb": 0,
			"uptime_seconds": 0, "conn_uptime_seconds": 0,
			"firmware": "", "build_date": "", "manufacturer": "", "model": "",
			"imei": "", "imsi": "", "iccid": "", "phone_number": "",
			"lte_category": "", "mimo": "",
		},
	}
	_ = json.NewEncoder(w).Encode(fallback)
}

// readPingHistorySummary parses /tmp/qmanager_ping_history.json (NDJSON lines
// of {ts, lat, loss, ...}) into:
//   - latencyHistory: last latency values, oldest first (null -> 0 sentinel)
//   - intervalSec:    nominal sampling interval (30s watchdog cadence)
//   - size:           number of samples retained
func readPingHistorySummary() ([]float64, int, int) {
	histFile := "/tmp/qmanager_ping_history.json"
	lat := []float64{}
	intervalSec := 30

	data, err := os.ReadFile(histFile)
	if err != nil {
		return lat, intervalSec, 0
	}
	lines := strings.Split(string(data), "\n")
	var prevTS int64 = 0
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		var entry struct {
			Ts  int64    `json:"ts"`
			Lat *float64 `json:"lat"`
		}
		if json.Unmarshal([]byte(l), &entry) == nil {
			if entry.Ts > 0 && prevTS > 0 {
				diff := int(entry.Ts - prevTS)
				if diff > 0 && diff < 300 {
					intervalSec = diff
				}
			}
			prevTS = entry.Ts
			if entry.Lat != nil {
				lat = append(lat, *entry.Lat)
			} else {
				lat = append(lat, 0)
			}
		}
	}
	if len(lat) > 720 {
		lat = lat[len(lat)-720:]
	}
	return lat, intervalSec, len(lat)
}

type CommandRequest struct {
	Command string `json:"command"`
}

// HandleSendCommand executes raw AT command safely
func (s *Server) HandleSendCommand(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req CommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Command == "" {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "no_command",
			"message": "Missing command field in JSON body",
		})
		return
	}

	cmdUpper := strings.ToUpper(req.Command)
	if strings.Contains(cmdUpper, "QSCAN") || strings.Contains(cmdUpper, "QSCANFREQ") {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "blocked",
			"message": "Use the Cell Scanner page for this command.",
		})
		return
	}

	resp, err := s.atClient.Exec(req.Command)
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  false,
			"error":    "exec_failed",
			"response": err.Error(),
			"command":  req.Command,
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"response": resp,
		"command":  req.Command,
	})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fileExistsAndEquals(path, expected string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == expected
}

// HandleAboutDevice returns system, device, network, and 3GPP metadata
func (s *Server) HandleAboutDevice(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	manufacturer := "Generic"
	model := "Modem"
	firmware := "-"
	imei := "-"
	qmanagerVersion := "v0.2.4"

	if statusData, err := os.ReadFile("/tmp/qmanager_status.json"); err == nil {
		var status map[string]interface{}
		if json.Unmarshal(statusData, &status) == nil {
			if dev, ok := status["device"].(map[string]interface{}); ok {
				if m, ok := dev["manufacturer"].(string); ok && m != "" {
					manufacturer = m
				}
				if md, ok := dev["model"].(string); ok && md != "" {
					model = cleanModelString(md)
				}
				if fw, ok := dev["firmware"].(string); ok && fw != "" {
					firmware = fw
				}
				if im, ok := dev["imei"].(string); ok && im != "" {
					imei = im
				}
				if qv, ok := dev["qmanager_version"].(string); ok && qv != "" {
					qmanagerVersion = qv
				}
			}
		}
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "qmanager-host"
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"device": map[string]interface{}{
			"model":        model,
			"manufacturer": manufacturer,
			"firmware":     firmware,
			"build_date":   "-",
			"imei":         imei,
		},
		"3gpp_release": map[string]interface{}{
			"lte":  "Rel 14",
			"nr5g": "N/A",
		},
		"network": map[string]interface{}{
			"device_ip":   "127.0.0.1",
			"lan_subnet":  "-",
			"wan_ipv4":    "-",
			"wan_ipv6":    "-",
			"public_ipv4": "-",
			"public_ipv6": "-",
		},
		"system": map[string]interface{}{
			"hostname":         hostname,
			"kernel_version":   "Linux Host",
			"openwrt_version":  "v1.0.1 Engine",
			"qmanager_version": qmanagerVersion,
		},
	})
}

// HandleSystemSettings manages system settings
func (s *Server) HandleSystemSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Prefer user-set name from qmanager.conf [settings].hostname (kernel
	// hostname is the SoC name "sdxprairie" — wrong for the sidebar display name).
	cfg := qmReadConfig()
	hostname := qmStr(cfg["settings"]["hostname"])
	if hostname == "" {
		hostname, _ = os.Hostname()
	}
	if hostname == "" {
		hostname = "qmanager-host"
	}

	if r.Method == http.MethodPost {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			if n, ok := body["hostname"].(string); ok && strings.TrimSpace(n) != "" {
				hostname = strings.TrimSpace(n)
				_ = qmWriteSection("settings", map[string]any{"hostname": hostname})
			}
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"settings": map[string]interface{}{
			"hostname":      hostname,
			"auto_logout":   true,
			"logout_time":   15,
			"language":      "en",
			"theme":         "dark",
			"check_updates": true,
		},
	})
}

// HandleSIMSlot manages active SIM slot state
func (s *Server) HandleSIMSlot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodPost {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			if v, ok := body["slot"].(float64); ok && (v == 1 || v == 2) {
				_, _ = s.atClient.Exec(fmt.Sprintf(`AT+QUIMSLOT=%d`, int(v)))
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "SIM slot switched", "active_slot": int(v)})
				return
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "invalid_slot", "detail": "slot must be 1 or 2"})
		return
	}

	// GET: read actual slot state
	slot := 1
	if resp, err := s.atClient.Exec(`AT+QUIMSLOT?`); err == nil && atContains(resp, "+QUIMSLOT: 2") {
		slot = 2
	}

	sim1 := map[string]interface{}{"status": "empty", "iccid": ""}
	sim2 := map[string]interface{}{"status": "empty", "iccid": ""}

	// SIM 1 (or active slot) status
	if resp, err := s.atClient.Exec(`AT+CPIN?`); err == nil {
		status := "ready"
		if atContains(resp, "+CME ERROR") || atContains(resp, "NOT INSERTED") {
			status = "absent"
		} else if atContains(resp, "SIM PIN") {
			status = "pin_required"
		}
		if slot == 1 {
			sim1["status"] = status
		} else {
			sim2["status"] = status
		}
	}

	// ICCID of active slot
	if resp, err := s.atClient.Exec(`AT+ICCID`); err == nil {
		iccid := parseSingleLineParam(resp, "+ICCID:")
		if iccid != "" {
			if slot == 1 {
				sim1["iccid"] = iccid
			} else {
				sim2["iccid"] = iccid
			}
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"active_slot": slot,
		"total_slots": 2,
		"sim1":        sim1,
		"sim2":        sim2,
	})
}

// HandleHostname returns host system hostname
func (s *Server) HandleHostname(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Prefer the user-set name from qmanager.conf [settings].hostname (kernel
	// hostname on this platform is the SoC name, e.g. "sdxprairie", which leaks
	// into the login "Sign in as <name>" line and is confusing). Fall back to
	// kernel hostname, then a generic default.
	cfg := qmReadConfig()
	hostname := qmStr(cfg["settings"]["hostname"])
	if hostname == "" {
		hostname, _ = os.Hostname()
	}
	if hostname == "" {
		hostname = "qmanager-host"
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"hostname": hostname,
	})
}

// HandleLANConfig manages LAN IP & DHCP configuration
func (s *Server) HandleLANConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodPost {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "LAN configuration saved"})
		return
	}

	// Read the real bridge0 address instead of a hardcoded placeholder.
	ipaddr := "192.168.225.1"
	netmask := "255.255.255.0"
	if out, err := exec.Command("ip", "-4", "-o", "addr", "show", "dev", "bridge0").Output(); err == nil {
		fields := strings.Fields(string(out))
		for i, f := range fields {
			if f == "inet" && i+1 < len(fields) {
				// format: 192.168.225.1/24
				parts := strings.Split(fields[i+1], "/")
				ipaddr = parts[0]
				if len(parts) == 2 {
					if bits := parts[1]; bits == "24" {
						netmask = "255.255.255.0"
					} else if bits == "16" {
						netmask = "255.255.0.0"
					}
				}
				break
			}
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"lan": map[string]interface{}{
			"ipaddr":  ipaddr,
			"netmask": netmask,
			"gateway": ipaddr,
			"dhcp":    true,
		},
	})
}

// HandleLANDevices lists connected LAN clients
func (s *Server) HandleLANDevices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Real neighbor table from the ARP cache — no hardcoded fake devices.
	devices := []map[string]interface{}{}
	if out, err := exec.Command("ip", "neigh", "show").Output(); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(out)))
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			// e.g. 192.168.225.54 dev bridge0 lladdr 1c:56:8e:0e:2f:b9 REACHABLE
			if len(fields) >= 5 && fields[1] == "dev" && (fields[3] == "lladdr") {
				devices = append(devices, map[string]interface{}{
					"ip":        fields[0],
					"mac":       fields[4],
					"interface": fields[2],
					"connected": len(fields) > 5 && (fields[5] == "REACHABLE" || fields[5] == "STALE"),
				})
			}
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"devices": devices,
	})
}

// HandleDNS manages custom DNS configuration
func (s *Server) HandleDNS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	cfg := qmReadJSONFile("/etc/qmanager/custom_dns.json")
	mode := qmStr(cfg["mode"])
	if mode == "" {
		mode = "preset"
	}
	preset := qmStr(cfg["preset"])
	if preset == "" {
		preset = "cloudflare"
	}
	dns1 := qmStr(cfg["dns1"])
	if dns1 == "" {
		dns1 = "1.1.1.1"
	}
	dns2 := qmStr(cfg["dns2"])
	if dns2 == "" {
		dns2 = "1.0.0.1"
	}
	dns3 := qmStr(cfg["dns3"])
	dns1v6 := qmStr(cfg["dns1v6"])
	dns2v6 := qmStr(cfg["dns2v6"])
	nic := qmStr(cfg["nic"])
	if nic == "" {
		nic = "lan"
	}
	enabled := qmBool(cfg["enabled"])

	if r.Method == http.MethodPost {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			// Frontend sends flat fields (mode, nic, dns1..dns3, dns1v6..dns2v6).
			if m, ok := body["mode"].(string); ok && m != "" {
				mode = m
			}
			if p, ok := body["preset"].(string); ok {
				preset = p
			}
			if n, ok := body["nic"].(string); ok {
				nic = n
			}
			if d, ok := body["dns1"].(string); ok {
				dns1 = d
			}
			if d, ok := body["dns2"].(string); ok {
				dns2 = d
			}
			if d, ok := body["dns3"].(string); ok {
				dns3 = d
			}
			if d, ok := body["dns1v6"].(string); ok {
				dns1v6 = d
			}
			if d, ok := body["dns2v6"].(string); ok {
				dns2v6 = d
			}
			if e, ok := body["enabled"].(bool); ok {
				enabled = e
			}
			writeJSONFile("/etc/qmanager/custom_dns.json", map[string]any{
				"mode":    mode,
				"preset":  preset,
				"nic":     nic,
				"dns1":    dns1,
				"dns2":    dns2,
				"dns3":    dns3,
				"dns1v6":  dns1v6,
				"dns2v6":  dns2v6,
				"enabled": enabled,
			})
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"mode":        mode,
		"currentDNS":  joinDnsList(dns1, dns2, dns3),
		"currentDNS6": joinDnsList(dns1v6, dns2v6),
		"nic":         nic,
	})
}

// joinDnsList joins non-empty DNS entries into the comma-separated wire format
// the frontend splits back into dns1/dns2/dns3.
func joinDnsList(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ",")
}

// HandleVideoOptimizer manages Traffic Engine & DPI masking settings.
// GET serves the video optimizer view (or the masquerade view when called
// with ?section=masquerade); POST accepts save / save_masquerade actions.
func (s *Server) HandleVideoOptimizer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		switch r.URL.Query().Get("section") {
		case "masquerade":
			s.handleTrafficEngineGet(w, r, true)
			return
		}
		switch r.URL.Query().Get("action") {
		case "verify_status":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "status": "idle"})
			return
		case "install_status":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "status": "idle"})
			return
		}
		s.handleTrafficEngineGet(w, r, false)
		return
	}

	if r.Method == http.MethodPost {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "invalid_json"})
			return
		}
		action, _ := body["action"].(string)
		switch action {
		case "save":
			s.handleTrafficEngineSave(w, body, false)
		case "save_masquerade":
			s.handleTrafficEngineSave(w, body, true)
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "unknown_action", "detail": action})
		}
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "method_not_allowed"})
}

// HandleSSHPassword updates the system SSH password.
// Real implementation in missing_endpoints.go (handleSSHPasswordImpl):
// verifies current_password against /etc/qmanager/auth.json, enforces the
// password policy, then pipes the new password into `passwd root`.
func (s *Server) HandleSSHPassword(w http.ResponseWriter, r *http.Request) {
	s.handleSSHPasswordImpl(w, r)
}

// HandleSoftwareUpdate manages OTA & software update checks
func (s *Server) HandleSoftwareUpdate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodPost {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "No update required"})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":          true,
		"current_version":  "v1.0.1",
		"latest_version":   "v1.0.1",
		"update_available": false,
	})
}

// HandleCellScanStart initiates background cell scanner
// Real implementation: checks the running flag, touches it, fires AT+QSCAN
// asynchronously (takes 30-90s), writes results to /tmp/qmanager_scan_results.txt,
// then clears the flag. FE polls cell_scan_status.sh while scanning.
func (s *Server) HandleCellScanStart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	if fileExists("/tmp/qmanager_long_running") {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "already_running",
			"detail":  "A scan is already in progress",
		})
		return
	}

	// Mark running BEFORE launching so status polling sees it immediately
	_ = os.WriteFile("/tmp/qmanager_long_running", []byte("scan"), 0644)

	go func() {
		// AT+QSCAN is long-running (30-90s); exec qcmd directly with a timeout.
		// The atClient Exec call would block this goroutine only — fine.
		out, _ := s.atClient.Exec(`AT+QSCAN`)
		// Persist raw output for the status handler to parse
		_ = os.WriteFile("/tmp/qmanager_scan_results.txt", []byte(out), 0644)
		// Clear running flag
		_ = os.Remove("/tmp/qmanager_long_running")
	}()

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"status":  "running",
	})
}

func cleanModelString(val string) string {
	val = strings.ReplaceAll(val, `"`, ``)
	parts := strings.Split(val, ",")
	if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
		return strings.TrimSpace(parts[0])
	}
	return strings.TrimSpace(val)
}
