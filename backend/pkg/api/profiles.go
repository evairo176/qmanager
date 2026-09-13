package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ScenarioScheduleBlock struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	Days     []int  `json:"days"`
	Scenario string `json:"scenario"`
}

type ScenarioSchedule struct {
	Enabled bool                    `json:"enabled"`
	Blocks  []ScenarioScheduleBlock `json:"blocks"`
}

type ScenarioBinding struct {
	Default  string           `json:"default"`
	Schedule ScenarioSchedule `json:"schedule"`
}

type ProfileApnSettings struct {
	CID     int    `json:"cid"`
	Name    string `json:"name"`
	PDPType string `json:"pdp_type"`
}

type ProfileSettings struct {
	APN  ProfileApnSettings `json:"apn"`
	IMEI string             `json:"imei"`
	TTL  int                `json:"ttl"`
	HL   int                `json:"hl"`
}

type SimProfile struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	MNO       string          `json:"mno"`
	SimICCID  string          `json:"sim_iccid"`
	CreatedAt int64           `json:"created_at"`
	UpdatedAt int64           `json:"updated_at"`
	Settings  ProfileSettings `json:"settings"`
	Scenario  ScenarioBinding `json:"scenario"`
}

type ProfileSummaryItem struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	MNO       string          `json:"mno"`
	SimICCID  string          `json:"sim_iccid"`
	CreatedAt int64           `json:"created_at"`
	UpdatedAt int64           `json:"updated_at"`
	Settings  ProfileSettings `json:"settings"`
	Scenario  ScenarioBinding `json:"scenario"`
}

const profilesDir = "/etc/qmanager/profiles"
const activeProfileFile = "/etc/qmanager/active_profile"

// Default profile instance for fallbacks
var defaultProfile = SimProfile{
	ID:        "default",
	Name:      "Default Carrier Profile",
	MNO:       "Custom",
	SimICCID:  "",
	CreatedAt: 0,
	UpdatedAt: 0,
	Settings: ProfileSettings{
		APN: ProfileApnSettings{
			CID:     1,
			Name:    "internet",
			PDPType: "IPV4V6",
		},
		IMEI: "",
		TTL:  0,
		HL:   0,
	},
	Scenario: ScenarioBinding{
		Default: "balanced",
		Schedule: ScenarioSchedule{
			Enabled: false,
			Blocks:  []ScenarioScheduleBlock{},
		},
	},
}

func profileFilePath(id string) string {
	return filepath.Join(profilesDir, id+".json")
}

// loadProfiles reads all stored profile JSON files from /etc/qmanager/profiles.
func loadProfiles() []SimProfile {
	var out []SimProfile
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(profilesDir, e.Name()))
		if err != nil {
			continue
		}
		var p SimProfile
		if json.Unmarshal(data, &p) == nil && p.ID != "" {
			// Normalize scenario onto legacy profiles
			if p.Scenario.Default == "" {
				p.Scenario.Default = "balanced"
			}
			if p.Scenario.Schedule.Blocks == nil {
				p.Scenario.Schedule.Blocks = []ScenarioScheduleBlock{}
			}
			out = append(out, p)
		}
	}
	return out
}

func activeProfileID() string {
	data, err := os.ReadFile(activeProfileFile)
	if err != nil {
		return "default"
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return "default"
	}
	return id
}

// HandleProfilesList lists stored SIM profiles
func (s *Server) HandleProfilesList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	active := activeProfileID()
	profiles := loadProfiles()
	summaries := make([]ProfileSummaryItem, 0, len(profiles)+1)
	// Always include the default profile first
	summaries = append(summaries, ProfileSummaryItem(defaultProfile))
	for _, p := range profiles {
		summaries = append(summaries, ProfileSummaryItem(p))
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":           true,
		"profiles":          summaries,
		"active_profile_id": active,
	})
}

// HandleProfilesGet gets full details for a profile
// Frontend hook getProfile() expects the FULL profile object as the response
// body (not wrapped in {profile: ...}): "The get endpoint returns the full
// profile on success, or { success: false, error } on failure."
func (s *Server) HandleProfilesGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id := r.URL.Query().Get("id")
	if id == "" || id == "default" {
		_ = json.NewEncoder(w).Encode(defaultProfile)
		return
	}

	for _, p := range loadProfiles() {
		if p.ID == id {
			_ = json.NewEncoder(w).Encode(p)
			return
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"error":   "profile_not_found",
		"detail":  "Profile " + id + " does not exist",
	})
}

// HandleProfilesSave creates or updates a profile
func (s *Server) HandleProfilesSave(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var p SimProfile
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "invalid_json"})
		return
	}

	now := time.Now().Unix()
	if p.ID == "" {
		// Generate id: p_<unix_ts>_<3-char-hex>
		randHex := strconv.FormatInt(now%0xffff, 16)
		if len(randHex) > 3 {
			randHex = randHex[:3]
		}
		for len(randHex) < 3 {
			randHex = "0" + randHex
		}
		p.ID = "p_" + strconv.FormatInt(now, 10) + "_" + randHex
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	if p.Scenario.Default == "" {
		p.Scenario.Default = "balanced"
	}
	if p.Scenario.Schedule.Blocks == nil {
		p.Scenario.Schedule.Blocks = []ScenarioScheduleBlock{}
	}

	_ = os.MkdirAll(profilesDir, 0755)
	data, _ := json.MarshalIndent(p, "", "  ")
	if err := os.WriteFile(profileFilePath(p.ID), data, 0644); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "write_failed", "detail": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"profile_id": p.ID,
		"message":    "Profile saved successfully",
	})
}

// HandleProfilesDelete deletes a profile
func (s *Server) HandleProfilesDelete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.ID == "" {
		req.ID = r.URL.Query().Get("id")
	}
	if req.ID == "" || req.ID == "default" {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "cannot_delete_default"})
		return
	}

	if err := os.Remove(profileFilePath(req.ID)); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "not_found", "detail": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Profile deleted successfully"})
}

// HandleProfilesApply applies a selected profile
func (s *Server) HandleProfilesApply(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.ID == "" {
		req.ID = r.URL.Query().Get("id")
	}
	if req.ID == "" {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "missing_profile_id"})
		return
	}

	// Validate profile exists (default is always valid)
	if req.ID != "default" {
		found := false
		for _, p := range loadProfiles() {
			if p.ID == req.ID {
				found = true
				break
			}
		}
		if !found {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "profile_not_found"})
			return
		}
	}

	// Persist active profile marker
	_ = os.MkdirAll("/etc/qmanager", 0755)
	if err := os.WriteFile(activeProfileFile, []byte(req.ID), 0644); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "write_failed"})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"detail":  "Profile applied successfully",
	})
}

// HandleProfilesCurrentSettings returns current live modem profile settings
func (s *Server) HandleProfilesCurrentSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Read live values from the modem
	iccid := ""
	if resp, err := s.atClient.Exec(`AT+ICCID`); err == nil {
		iccid = parseSingleLineParam(resp, "+ICCID:")
	}
	imei := ""
	if resp, err := s.atClient.Exec(`AT+GSN`); err == nil {
		for _, line := range strings.Split(resp, "\n") {
			line = strings.TrimSpace(line)
			if len(line) == 15 && isDigits(line) {
				imei = line
				break
			}
		}
	}
	apn := "internet"
	pdpType := "IPV4V6"
	if resp, err := s.atClient.Exec(`AT+CGDCONT?`); err == nil {
		for _, line := range strings.Split(resp, "\n") {
			if strings.HasPrefix(line, "+CGDCONT: 1,") {
				parts := strings.Split(line, ",")
				if len(parts) >= 3 {
					pdpType = strings.Trim(parts[1], `" `)
					apn = strings.Trim(parts[2], `" `)
				}
				break
			}
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"settings": map[string]interface{}{
			"iccid":      iccid,
			"imei":       imei,
			"active_cid": 1,
			"apn_profiles": []map[string]interface{}{
				{"cid": 1, "apn": apn, "pdp_type": pdpType},
			},
		},
	})
}

// HandleScenariosList lists connection scenarios
// Frontend contract (types/connection-scenario.ts StoredScenario):
//   { id, name, description, gradient, config: {atModeValue, mode, optimization, lte_bands, nsa_nr_bands, sa_nr_bands} }
// Missing gradient/config crashes connection-scenario-card.tsx (spread ...s then
// ScenarioItem reads scenario.gradient).
func (s *Server) HandleScenariosList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	scenarios := []map[string]interface{}{
		{
			"id": "balanced", "name": "Balanced", "description": "Balanced speed and stability",
			"gradient": "from-emerald-500 via-teal-500 to-cyan-500",
			"config": map[string]interface{}{
				"atModeValue": "AUTO", "mode": "Auto", "optimization": "Balanced",
				"lte_bands": "", "nsa_nr_bands": "", "sa_nr_bands": "",
			},
		},
		{
			"id": "gaming", "name": "Gaming", "description": "Low latency for gaming and VoIP",
			"gradient": "from-violet-600 via-purple-600 to-indigo-700",
			"config": map[string]interface{}{
				"atModeValue": "AUTO", "mode": "Auto", "optimization": "Latency",
				"lte_bands": "", "nsa_nr_bands": "", "sa_nr_bands": "",
			},
		},
		{
			"id": "streaming", "name": "Streaming", "description": "High throughput for streaming",
			"gradient": "from-rose-500 via-pink-500 to-orange-400",
			"config": map[string]interface{}{
				"atModeValue": "AUTO", "mode": "Auto", "optimization": "Throughput",
				"lte_bands": "", "nsa_nr_bands": "", "sa_nr_bands": "",
			},
		},
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"scenarios": scenarios,
	})
}
