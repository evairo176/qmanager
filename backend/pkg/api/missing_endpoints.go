package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// =============================================================================
// missing_endpoints.go — Handlers for frontend features that were previously
// 404 (no backend route): config-backup, scenario CRUD, language-pack status,
// profile deactivate/apply-status aliases, tower failover status, neighbour
// scan status, and the real SSH password change.
//
// Conventions (same as the rest of the package):
//   - JSON responses via json.NewEncoder(w).Encode
//   - errors as {success:false, error:"...", detail?:"..."}
//   - file writes are atomic (tmp + rename)
// =============================================================================

// -----------------------------------------------------------------------------
// Config backup: collect / apply / apply_status / apply_cancel
// -----------------------------------------------------------------------------

const configBackupDir = "/etc/qmanager"

// qmConfigSectionFile maps a backup section key to the /etc/qmanager file that
// backs it. Only files here are ever written by apply.sh — the safe subset.
var qmConfigSectionFile = map[string]string{
	"sms_alerts":  "alert_routing.json",
	"tower_lock":  "tower_lock.json",
	"watchdog":    "", // special-cased: ping_profile.json + quality_thresholds.json
	"ping_profile": "ping_profile.json",
}

// configApplyState tracks the (fake, synchronous) apply job for the frontend
// poller. apply.sh runs inline and marks the job done before returning, so the
// first apply_status.sh poll reports "done" immediately.
var (
	configApplyMu     sync.Mutex
	configApplyStatus = "idle" // idle | running | done
	configApplyProg   = 0
)

func atomicWriteJSON(path string, data map[string]any) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// redactSecrets replaces sensitive values (hash/salt/password/token/secret)
// with placeholders so backups never leak credentials. Applied recursively.
func redactSecrets(v any, depth int) any {
	if depth > 8 {
		return v
	}
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			switch strings.ToLower(k) {
			case "hash", "salt", "password", "token", "secret", "api_key", "apikey":
				out[k] = "REDACTED"
				continue
			}
			out[k] = redactSecrets(val, depth+1)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = redactSecrets(val, depth+1)
		}
		return out
	}
	return v
}

func readQmJSON(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{}
	}
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return map[string]any{}
	}
	return m
}

func deviceMetaFromStatus() (model, firmware, imei, qmVersion string) {
	model, firmware, imei, qmVersion = "QManager", "-", "-", "v0.2.4-stable"
	if data, err := os.ReadFile("/tmp/qmanager_status.json"); err == nil {
		var status map[string]any
		if json.Unmarshal(data, &status) == nil {
			if dev, ok := status["device"].(map[string]any); ok {
				if m, ok := dev["model"].(string); ok && m != "" {
					model = cleanModelString(m)
				}
				if f, ok := dev["firmware"].(string); ok && f != "" {
					firmware = f
				}
				if im, ok := dev["imei"].(string); ok && im != "" {
					imei = im
				}
			}
		}
	}
	return
}

// HandleConfigBackupCollect builds the plaintext payload for the backup
// envelope: GET ?sections=a,b,c -> {schema:1, header:{...}, payload:{...}}.
// Only safe /etc/qmanager files are read; auth/session files are never
// included and every payload is secret-redacted.
func (s *Server) HandleConfigBackupCollect(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "method_not_allowed"})
		return
	}

	sections := []string{}
	for _, part := range strings.Split(r.URL.Query().Get("sections"), ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			sections = append(sections, part)
		}
	}
	if len(sections) == 0 {
		sections = []string{"network_mode_apn", "bands", "tower_lock", "ttl_hl", "imei", "profiles", "sms_alerts", "watchdog"}
	}

	payload := map[string]any{}
	for _, sec := range sections {
		var val any
		file, ok := qmConfigSectionFile[sec]
		if sec == "watchdog" {
			val = map[string]any{
				"ping_profile":        readQmJSON(filepath.Join(configBackupDir, "ping_profile.json")),
				"quality_thresholds":  readQmJSON(filepath.Join(configBackupDir, "quality_thresholds.json")),
			}
		} else if ok && file != "" {
			val = readQmJSON(filepath.Join(configBackupDir, file))
		} else {
			// No safe file backing this section — represent as empty so the
			// envelope stays valid JSON without exposing anything.
			val = map[string]any{}
		}
		payload[sec] = redactSecrets(val, 0)
	}

	model, firmware, imei, qmVersion := deviceMetaFromStatus()
	header := map[string]any{
		"magic":     "QMBACKUP",
		"version":   1,
		"created_at": time.Now().UTC().Format(time.RFC3339),
		"device": map[string]any{
			"model":           model,
			"firmware":        firmware,
			"imei":            imei,
			"qmanager_version": qmVersion,
		},
		"sections_included": sections,
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"schema":  1,
		"header":  header,
		"payload": map[string]any{"schema": 1, "sections": payload},
	})
}

// HandleConfigBackupApply writes back only the safe /etc/qmanager files listed
// in qmConfigSectionFile. Body: {schema:1, sections:{...}} (the decrypted
// payload). Returns applied + skipped section lists.
func (s *Server) HandleConfigBackupApply(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "method_not_allowed"})
		return
	}

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "invalid_json"})
		return
	}

	sections, _ := body["sections"].(map[string]any)

	configApplyMu.Lock()
	configApplyStatus = "running"
	configApplyProg = 10
	configApplyMu.Unlock()

	var applied []string
	var skipped []string

	// Deterministic order: iterate the safe section list (not map iteration).
	applyOne := func(sec string) {
		val, ok := sections[sec]
		if !ok {
			skipped = append(skipped, sec)
			return
		}
		m, ok := val.(map[string]any)
		if !ok {
			skipped = append(skipped, sec)
			return
		}
		switch sec {
		case "watchdog":
			if p, ok := m["ping_profile"].(map[string]any); ok {
				_ = atomicWriteJSON(filepath.Join(configBackupDir, "ping_profile.json"), p)
			}
			if q, ok := m["quality_thresholds"].(map[string]any); ok {
				_ = atomicWriteJSON(filepath.Join(configBackupDir, "quality_thresholds.json"), q)
			}
			applied = append(applied, sec)
		default:
			file := qmConfigSectionFile[sec]
			if file == "" {
				skipped = append(skipped, sec)
				return
			}
			if err := atomicWriteJSON(filepath.Join(configBackupDir, file), m); err != nil {
				skipped = append(skipped, sec)
				return
			}
			applied = append(applied, sec)
		}
	}

	for _, sec := range []string{"sms_alerts", "watchdog", "tower_lock"} {
		applyOne(sec)
	}

	configApplyMu.Lock()
	configApplyStatus = "done"
	configApplyProg = 100
	configApplyMu.Unlock()

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":          true,
		"applied_sections": applied,
		"skipped_sections": skipped,
	})
}

// HandleConfigBackupApplyStatus reports the apply job state to the restore
// poller: {status, progress}.
func (s *Server) HandleConfigBackupApplyStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	configApplyMu.Lock()
	defer configApplyMu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":   configApplyStatus,
		"progress": configApplyProg,
	})
}

// HandleConfigBackupApplyCancel resets the apply job.
func (s *Server) HandleConfigBackupApplyCancel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	configApplyMu.Lock()
	configApplyStatus = "idle"
	configApplyProg = 0
	configApplyMu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

// -----------------------------------------------------------------------------
// Connection scenarios: activate / save / delete (list.sh already exists)
// -----------------------------------------------------------------------------

const scenariosCustomFile = "/etc/qmanager/scenarios_custom.json"
const activeScenarioFile = "/etc/qmanager/active_scenario"

// loadCustomScenarios reads custom scenario definitions (additive to the
// built-in defaults served by HandleScenariosList).
func loadCustomScenarios() []map[string]any {
	data, err := os.ReadFile(scenariosCustomFile)
	if err != nil {
		return []map[string]any{}
	}
	var list []map[string]any
	if json.Unmarshal(data, &list) != nil || list == nil {
		return []map[string]any{}
	}
	return list
}

func saveCustomScenarios(list []map[string]any) error {
	if list == nil {
		list = []map[string]any{}
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := scenariosCustomFile + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, scenariosCustomFile)
}

func activeScenarioID() string {
	data, err := os.ReadFile(activeScenarioFile)
	if err != nil {
		return "balanced"
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return "balanced"
	}
	return id
}

// findCustomScenario returns the custom scenario with the given id.
func findCustomScenario(id string) (map[string]any, bool) {
	for _, sc := range loadCustomScenarios() {
		if scID, _ := sc["id"].(string); scID == id {
			return sc, true
		}
	}
	return nil, false
}

func normalizeScenarioConfig(conf map[string]any) map[string]any {
	out := map[string]any{}
	if conf == nil {
		conf = map[string]any{}
	}
	for _, k := range []string{"atModeValue", "mode", "optimization", "lte_bands", "nsa_nr_bands", "sa_nr_bands"} {
		v, _ := conf[k].(string)
		out[k] = v
	}
	if out["atModeValue"] == "" {
		out["atModeValue"] = "AUTO"
	}
	if out["mode"] == "" {
		out["mode"] = "Auto"
	}
	return out
}

// HandleScenarioActivate applies a scenario (write marker file + best-effort
// AT band/mode commands). Body: {id, mode?, lte_bands?, nsa_nr_bands?,
// sa_nr_bands?} — custom scenarios send the full config.
func (s *Server) HandleScenarioActivate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "method_not_allowed"})
		return
	}

	var req struct {
		ID         string `json:"id"`
		Mode       string `json:"mode"`
		LteBands   string `json:"lte_bands"`
		NsaBands   string `json:"nsa_nr_bands"`
		SaBands    string `json:"sa_nr_bands"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "missing_fields", "detail": "id is required"})
		return
	}

	// SECURITY: strict input validation before any value reaches an AT command.
	// lte/nsa/sa band strings may only contain band chars; mode is mapped from a
	// fixed enum. Without this, a malicious body could inject additional AT
	// commands (e.g. ";AT+CFUN=0") that bypass the send_command blocklist and
	// rate limit (bug found in 2026-08-29 audit).
	mode := strings.ToUpper(strings.TrimSpace(req.Mode))
	switch mode {
	case "", "AUTO", "2G", "3G", "4G", "5G", "LTE", "NR":
		// allowed
	default:
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "invalid_mode", "detail": "mode must be AUTO/2G/3G/4G/5G/LTE/NR"})
		return
	}
	bandRe := regexp.MustCompile(`^[0-9,:-]*$`)
	for field, val := range map[string]string{"lte_bands": req.LteBands, "nsa_nr_bands": req.NsaBands, "sa_nr_bands": req.SaBands} {
		if val != "" && !bandRe.MatchString(val) {
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "invalid_bands", "detail": field + " may only contain digits, commas, colons and dashes"})
			return
		}
	}

	// Scenario IDs are persisted verbatim to the active-scenario marker file —
	// keep them to a safe charset so a malicious id can't smuggle path-like
	// content or break the marker file format.
	idRe := regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	if !idRe.MatchString(req.ID) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "invalid_id", "detail": "Scenario id may only contain letters, digits, dot, dash and underscore"})
		return
	}

	// Resolve config: custom scenarios carry it in the body; for built-ins the
	// commands are sent with the marker only (empty bands = auto).
	conf := map[string]any{
		"atModeValue": req.Mode,
		"lte_bands":   req.LteBands,
		"nsa_nr_bands": req.NsaBands,
		"sa_nr_bands":  req.SaBands,
	}
	if strings.HasPrefix(req.ID, "custom-") {
		if stored, ok := findCustomScenario(req.ID); ok {
			if c, ok := stored["config"].(map[string]any); ok {
				if req.Mode == "" {
					conf["atModeValue"] = c["atModeValue"]
				}
				if req.LteBands == "" {
					conf["lte_bands"] = c["lte_bands"]
				}
				if req.NsaBands == "" {
					conf["nsa_nr_bands"] = c["nsa_nr_bands"]
				}
				if req.SaBands == "" {
					conf["sa_nr_bands"] = c["sa_nr_bands"]
				}
			}
		}
	}

	// Persist the active marker (source of truth for the UI).
	_ = os.MkdirAll(configBackupDir, 0755)
	if err := os.WriteFile(activeScenarioFile, []byte(req.ID), 0644); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "write_failed", "detail": err.Error()})
		return
	}

	// Best-effort AT application — never fail the request on modem errors.
	applyMode, _ := conf["atModeValue"].(string)
	if applyMode != "" && applyMode != "AUTO" {
		_, _ = s.atClient.Exec(fmt.Sprintf(`AT+QNWPREFCFG="mode_pref",%s`, applyMode))
	}
	lte, _ := conf["lte_bands"].(string)
	if lte != "" {
		_, _ = s.atClient.Exec(fmt.Sprintf(`AT+QNWPREFCFG="lte_band",%s`, lte))
	}
	nsa, _ := conf["nsa_nr_bands"].(string)
	if nsa != "" {
		_, _ = s.atClient.Exec(fmt.Sprintf(`AT+QNWPREFCFG="nsa_nr5g_band",%s`, nsa))
	}
	sa, _ := conf["sa_nr_bands"].(string)
	if sa != "" {
		_, _ = s.atClient.Exec(fmt.Sprintf(`AT+QNWPREFCFG="nr5g_band",%s`, sa))
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"id":      req.ID,
		"detail":  "Scenario activated",
	})
}

// HandleScenarioActive returns the currently active scenario id (GET).
func (s *Server) HandleScenarioActive(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"active_scenario_id": activeScenarioID()})
}

// HandleScenarioSave appends/updates a custom scenario. Body: {id?, name,
// description, gradient?, config}.
func (s *Server) HandleScenarioSave(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "method_not_allowed"})
		return
	}

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "invalid_json"})
		return
	}

	name, _ := body["name"].(string)
	if name == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "missing_fields", "detail": "name is required"})
		return
	}

	id, _ := body["id"].(string)
	if id == "" {
		id = fmt.Sprintf("custom-%d", time.Now().Unix())
	}

	description, _ := body["description"].(string)
	gradient, _ := body["gradient"].(string)
	if gradient == "" {
		gradient = "from-slate-600 via-slate-700 to-slate-800"
	}
	config, _ := body["config"].(map[string]any)

	list := loadCustomScenarios()
	replaced := false
	for i, sc := range list {
		if scID, _ := sc["id"].(string); scID == id {
			list[i] = map[string]any{
				"id":          id,
				"name":        name,
				"description": description,
				"gradient":    gradient,
				"config":      normalizeScenarioConfig(config),
			}
			replaced = true
			break
		}
	}
	if !replaced {
		list = append(list, map[string]any{
			"id":          id,
			"name":        name,
			"description": description,
			"gradient":    gradient,
			"config":      normalizeScenarioConfig(config),
		})
	}

	if err := saveCustomScenarios(list); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "write_failed", "detail": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "id": id})
}

// HandleScenarioDelete removes a custom scenario. Body: {id}.
func (s *Server) HandleScenarioDelete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "method_not_allowed"})
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "missing_fields", "detail": "id is required"})
		return
	}

	list := loadCustomScenarios()
	out := list[:0]
	found := false
	for _, sc := range list {
		if scID, _ := sc["id"].(string); scID == req.ID {
			found = true
			continue
		}
		out = append(out, sc)
	}
	if !found {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "not_found", "detail": "Scenario " + req.ID + " does not exist"})
		return
	}

	if err := saveCustomScenarios(out); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "write_failed", "detail": err.Error()})
		return
	}

	// If the deleted scenario was active, fall back to balanced.
	if activeScenarioID() == req.ID {
		_ = os.Remove(activeScenarioFile)
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

// -----------------------------------------------------------------------------
// Language packs: install_status / install_cancel / remove
// -----------------------------------------------------------------------------

const localesDir = "/usrdata/qmanager/web/locales"

// HandleLanguagePacksInstallStatus reports the (synchronous) install job state
// using the shape the frontend LanguagePackInstallState expects: {state, code,
// progress, message} — plus a "status" alias for compatibility.
func (s *Server) HandleLanguagePacksInstallStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"state":    "idle",
		"status":   "idle",
		"progress": 0,
		"message":  "",
	})
}

// HandleLanguagePacksInstallCancel cancels the (no-op) install job.
func (s *Server) HandleLanguagePacksInstallCancel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

// HandleLanguagePacksRemove removes an installed language pack directory.
// Body: {code}. The "en" pack is bundled and cannot be removed.
func (s *Server) HandleLanguagePacksRemove(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "method_not_allowed"})
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "missing_fields", "detail": "code is required"})
		return
	}

	// Path-traversal guard: code must be a plain directory name.
	if req.Code == "en" || strings.Contains(req.Code, "/") || strings.Contains(req.Code, "..") || strings.Contains(req.Code, "\\") {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "invalid_code", "detail": "Invalid or protected language code"})
		return
	}

	dir := filepath.Join(localesDir, req.Code)
	if _, err := os.Stat(dir); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "not_found", "detail": "Language pack " + req.Code + " is not installed"})
		return
	}

	if err := os.RemoveAll(dir); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "remove_failed", "detail": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "code": req.Code})
}

// -----------------------------------------------------------------------------
// Profiles: deactivate + apply_status
// -----------------------------------------------------------------------------

// HandleProfilesDeactivate clears the active profile marker. No modem changes.
func (s *Server) HandleProfilesDeactivate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "method_not_allowed"})
		return
	}

	if err := os.Remove(activeProfileFile); err != nil && !os.IsNotExist(err) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "write_failed", "detail": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":           true,
		"active_profile_id": "",
		"requires_reboot":   false,
	})
}

// HandleProfilesApplyStatus reports the apply lifecycle state. The apply.sh
// handler is synchronous, so a leftover active profile marker means the last
// apply completed. Shape matches ProfileApplyState in the frontend.
func (s *Server) HandleProfilesApplyStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	startedAt := int64(0)
	if fi, err := os.Stat(activeProfileFile); err == nil {
		startedAt = fi.ModTime().Unix()
	}

	if id := activeProfileID(); id != "default" {
		name := "Active Profile"
		for _, p := range loadProfiles() {
			if p.ID == id {
				name = p.Name
				break
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":          "complete",
			"progress":        100,
			"profile_id":      id,
			"profile_name":    name,
			"started_at":      startedAt,
			"requires_reboot": false,
			"error":           nil,
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":          "idle",
		"progress":        0,
		"profile_id":      "",
		"profile_name":    "",
		"started_at":      0,
		"requires_reboot": false,
		"error":           nil,
	})
}

// -----------------------------------------------------------------------------
// Tower failover status alias (same shape as bands/failover_status.sh)
// -----------------------------------------------------------------------------

// HandleTowerFailoverStatus checks tower failover flag files without touching
// the modem. Mirrors HandleBandsFailoverStatus JSON shape.
func (s *Server) HandleTowerFailoverStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	failoverEnabled := fileExists("/etc/qmanager/tower_failover.enabled") || fileExistsAndEquals("/etc/qmanager/tower_failover_enabled", "1")
	failoverActivated := fileExists("/tmp/qmanager_tower_failover")
	watcherRunning := fileExists("/tmp/qmanager_tower_failover.pid")

	_ = json.NewEncoder(w).Encode(map[string]any{
		"enabled":         failoverEnabled,
		"activated":       failoverActivated,
		"watcher_running": watcherRunning,
	})
}

// -----------------------------------------------------------------------------
// Neighbour scan status alias for the cell-scanner neighbour view
// -----------------------------------------------------------------------------

// neighbourScanCell is the shape the neighbour scanner UI renders
// (NeighbourCellResult): camelCase fields.
type neighbourScanCell struct {
	ID             string `json:"id"`
	NetworkType    string `json:"networkType"`
	CellType       string `json:"cellType"`
	Frequency      int    `json:"frequency"`
	PCI            int    `json:"pci"`
	SignalStrength int    `json:"signalStrength"`
	RSRQ           int    `json:"rsrq,omitempty"`
	SINR           int    `json:"sinr,omitempty"`
}

// HandleNeighbourScanStatus serves the same underlying scan state as
// HandleCellScanStatus, translated into the neighbour-scanner response shape
// ({status, results}) plus the cell-scan fields ({success, scanning}).
func (s *Server) HandleNeighbourScanStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	scanning := fileExists("/tmp/qmanager_long_running")

	cells := []QScanCellResult{}
	if data, err := os.ReadFile("/tmp/qmanager_scan_results.txt"); err == nil {
		cells = ParseQScanOutput(string(data))
	}

	status := "idle"
	if scanning {
		status = "running"
	} else if len(cells) > 0 {
		status = "complete"
	}

	results := make([]neighbourScanCell, 0, len(cells))
	for _, c := range cells {
		results = append(results, neighbourScanCell{
			ID:             fmt.Sprintf("%s-%d-%d", c.Tech, c.ARFCN, c.PCI),
			NetworkType:    c.Tech,
			CellType:       "neighbour",
			Frequency:      c.ARFCN,
			PCI:            c.PCI,
			SignalStrength: c.RSRP,
			RSRQ:           c.RSRQ,
			SINR:           c.SINR,
		})
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":  true,
		"scanning": scanning,
		"status":   status,
		"results":  results,
	})
}

// -----------------------------------------------------------------------------
// SSH password change (real implementation replacing the fake success stub)
// -----------------------------------------------------------------------------

// handleSSHPasswordImpl verifies the current password against the QManager
// auth file (sha256(salt+password)) and pipes the new password into `passwd
// root`. Never echoes the password.
func (s *Server) handleSSHPasswordImpl(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "method_not_allowed",
			"detail":  "Unexpected request method.",
		})
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
		EnforceStrong   bool   `json:"enforce_strong"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "invalid_json",
			"detail":  "Malformed JSON body.",
		})
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "missing_fields",
			"detail":  "Both current and new password are required.",
		})
		return
	}

	// Verify the current password via the same scheme as login. A wrong
	// current password must never reach passwd.
	if !s.verifyPassword(req.CurrentPassword) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "invalid_password",
			"detail":  "Current password is incorrect.",
		})
		return
	}

	// Password policy: strong mode requires >= 8 chars with letter+digit;
	// relaxed mode requires >= 8 chars (passwords shorter than 8 are refused
	// regardless — avoids trivially weak root passwords).
	if len(req.NewPassword) < 8 {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "password_weak",
			"detail":  "New password must be at least 8 characters long.",
		})
		return
	}
	if req.EnforceStrong {
		hasLetter, hasDigit := false, false
		for _, c := range req.NewPassword {
			switch {
			case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
				hasLetter = true
			case c >= '0' && c <= '9':
				hasDigit = true
			}
		}
		if !hasLetter || !hasDigit {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"error":   "password_weak",
				"detail":  "New password must contain at least one letter and one digit.",
			})
			return
		}
	}

	// Pipe the new password into `passwd root` (twice, as it prompts for
	// confirmation). Using stdin avoids exposing the password in argv or in a
	// shell command string.
	cmd := exec.Command("passwd", "root")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "passwd_failed",
			"detail":  "Failed to open password pipe: " + err.Error(),
		})
		return
	}
	if err := cmd.Start(); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "passwd_failed",
			"detail":  err.Error(),
		})
		return
	}
	_, _ = stdin.Write([]byte(req.NewPassword + "\n" + req.NewPassword + "\n"))
	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "passwd_failed",
			"detail":  "passwd exited with an error: " + err.Error(),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "SSH password updated",
	})
}

// -----------------------------------------------------------------------------
// Dashboard admin password change (auth/password.sh)
// FE contract (use-auth.ts changePassword):
//   POST {current_password, new_password, enforce_strong} -> {success}
// Value verified against /etc/qmanager/auth.json (sha256(salt+password)) —
// the SAME scheme as login (verifyPassword in auth.go). This only changes the
// QManager dashboard login, NOT the system SSH/shadow password.
// -----------------------------------------------------------------------------

func (s *Server) HandleChangeDashboardPassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
		EnforceStrong   bool   `json:"enforce_strong"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NewPassword == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "missing_password"})
		return
	}

	if req.NewPassword != "" && len(req.NewPassword) < 5 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "password_too_short"})
		return
	}

	// Load current auth config — must exist (dashboard password set at first boot).
	data, err := os.ReadFile(authConfigPath)
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "setup_first", "detail": "Dashboard password not configured yet."})
		return
	}
	var auth struct {
		Hash string `json:"hash"`
		Salt string `json:"salt"`
	}
	if json.Unmarshal(data, &auth) != nil || auth.Salt == "" || auth.Hash == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "setup_first", "detail": "Dashboard password not configured yet."})
		return
	}

	// Verify current password using the same sha256(salt+password) scheme as login.
	if req.CurrentPassword == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "missing_password", "detail": "Current password is required."})
		return
	}
	sum := sha256.Sum256([]byte(auth.Salt + req.CurrentPassword))
	if hex.EncodeToString(sum[:]) != auth.Hash {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "invalid_password"})
		return
	}

	// Enforce strong policy when requested (mirrors frontend password-requirements).
	if req.EnforceStrong {
		pw := req.NewPassword
		hasLetter, hasDigit := false, false
		for _, c := range pw {
			switch {
			case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
				hasLetter = true
			case c >= '0' && c <= '9':
				hasDigit = true
			}
		}
		if !hasLetter || !hasDigit {
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "password_weak", "detail": "New password must contain at least one letter and one digit."})
			return
		}
	}

	// Rotate salt + write new hash atomically.
	newSalt := make([]byte, 16)
	if _, err := rand.Read(newSalt); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "internal_error"})
		return
	}
	saltHex := hex.EncodeToString(newSalt)
	sum = sha256.Sum256([]byte(saltHex + req.NewPassword))
	newAuth := map[string]any{"hash": hex.EncodeToString(sum[:]), "salt": saltHex, "version": 1}
	payload, _ := json.MarshalIndent(newAuth, "", "  ")

	tmp := authConfigPath + ".tmp"
	if err := os.WriteFile(tmp, payload, 0600); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "write_failed"})
		return
	}
	if err := os.Rename(tmp, authConfigPath); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "write_failed"})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": "Password updated"})
}

// -----------------------------------------------------------------------------
// Modem network reconnect (at_cmd/reconnect_modem.sh)
// FE contract (use-modem-reconnect.ts): POST {} -> {success, detail}
// Same flow as HandleSystemReboot's reconnect branch: AT+COPS=2 then AT+COPS=0.
// -----------------------------------------------------------------------------

func (s *Server) HandleReconnectModem(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	_, _ = s.atClient.Exec("AT+COPS=2")
	_, _ = s.atClient.Exec("AT+COPS=0")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"detail":  "Network reconnect initiated",
	})
}

// -----------------------------------------------------------------------------
// Diagnostics capture (system/diagnostics.sh)
// FE contract (use-diagnostics.ts): POST {action:"capture"} ->
//   {success, filename, content} — content is downloaded as a text file.
// -----------------------------------------------------------------------------

func (s *Server) HandleDiagnosticsCapture(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Action string `json:"action"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Action != "capture" {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "missing_action"})
		return
	}

	var sb strings.Builder
	run := func(label, cmdStr string) {
		out, err := exec.Command("/bin/sh", "-c", cmdStr).Output()
		if err != nil {
			out = []byte("[error: " + err.Error() + "]\n")
		}
		fmt.Fprintf(&sb, "===== %s =====\n%s\n", label, string(out))
	}

	run("uname", "uname -a")
	run("uptime", "uptime")
	run("storage", "df -h 2>/dev/null")
	run("mounts", "mount | head -30")
	run("dmesg-tail", "dmesg 2>/dev/null | tail -40")
	run("qmanager-status", "cat /tmp/qmanager_status.json 2>/dev/null || echo 'no status file'")
	run("iptables", "iptables -L INPUT -n 2>/dev/null")
	run("failed-units", "systemctl --failed --no-pager 2>/dev/null")
	run("services", "systemctl is-active qmanager-core qmanager-firewall sshd 2>/dev/null")
	run("listen", "netstat -tlnp 2>/dev/null | head -20 || ss -tln 2>/dev/null | head -20")

	filename := "qmanager-diagnostics-" + time.Now().Format("20060102-150405") + ".txt"
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":  true,
		"filename": filename,
		"content":  sb.String(),
	})
}