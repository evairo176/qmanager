package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"qmanager-backend/pkg/at"
)

func TestAPIWithMockClient(t *testing.T) {
	mockAT := at.NewMockClient()
	server := NewServer(mockAT)

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	// Bypass RequireAuth (qmanager_session cookie) — all protected endpoints need
	// a valid session after the 2026-08-29 zero-auth audit. Login once with the
	// first-boot default password ("admin") and reuse the session cookie.
	loginRec := httptest.NewRecorder()
	loginReq := httptest.NewRequest(http.MethodPost, "/cgi-bin/quecmanager/auth/login.sh",
		bytes.NewBuffer([]byte(`{"password":"admin","remember":true}`)))
	mux.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("test setup: login failed: %d", loginRec.Code)
	}
	sessionCookie := ""
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == "qmanager_session" {
			sessionCookie = c.Value
			break
		}
	}
	if sessionCookie == "" {
		t.Fatal("test setup: no qmanager_session cookie issued")
	}

	// authedRequest builds a request carrying the session cookie.
	authedRequest := func(method, target string, body []byte) *http.Request {
		var r *http.Request
		if body == nil {
			r = httptest.NewRequest(method, target, nil)
		} else {
			r = httptest.NewRequest(method, target, bytes.NewBuffer(body))
		}
		r.AddCookie(&http.Cookie{Name: "qmanager_session", Value: sessionCookie})
		return r
	}

	t.Run("HandleBandsCurrent", func(t *testing.T) {
		req := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/bands/current.sh", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200, got %d", rec.Code)
		}

		var res map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to unmarshal JSON response: %v", err)
		}

		if res["success"] != true {
			t.Errorf("expected success: true, got %v", res["success"])
		}

		current, ok := res["current"].(map[string]interface{})
		if !ok || current["lte_bands"] != "1:3:7:28" {
			t.Errorf("expected lte_bands '1:3:7:28', got %v", current["lte_bands"])
		}
	})

	t.Run("HandleBandsLock LTE and NR5G", func(t *testing.T) {
		payload := []byte(`{"band_type":"lte","bands":"1:3:7"}`)
		req := authedRequest(http.MethodPost, "/cgi-bin/quecmanager/bands/lock.sh", payload)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200, got %d", rec.Code)
		}

		// Invalid band type
		badPayload := []byte(`{"band_type":"invalid","bands":"1"}`)
		reqBad := authedRequest(http.MethodPost, "/cgi-bin/quecmanager/bands/lock.sh", badPayload)
		recBad := httptest.NewRecorder()
		mux.ServeHTTP(recBad, reqBad)
		if recBad.Code != http.StatusOK {
			t.Errorf("expected 200 with error payload, got %d", recBad.Code)
		}
	})

	t.Run("HandleTowerLock 5G NR and Unlock", func(t *testing.T) {
		payload := []byte(`{"type":"nr_sa","action":"lock","pci":108,"arfcn":627464,"scs":30,"band":78}`)
		req := authedRequest(http.MethodPost, "/cgi-bin/quecmanager/tower/lock.sh", payload)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200, got %d", rec.Code)
		}

		var res map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}

		if res["success"] != true || res["locked"] != true {
			t.Errorf("expected lock success, got %v", res)
		}

		// Unlock test
		unlockPayload := []byte(`{"type":"nr_sa","action":"unlock"}`)
		reqUnlock := authedRequest(http.MethodPost, "/cgi-bin/quecmanager/tower/lock.sh", unlockPayload)
		recUnlock := httptest.NewRecorder()
		mux.ServeHTTP(recUnlock, reqUnlock)

		if recUnlock.Code != http.StatusOK {
			t.Errorf("expected unlock HTTP 200, got %d", recUnlock.Code)
		}
	})

	t.Run("HandleCellularSettings GET & POST", func(t *testing.T) {
		req := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/cellular/settings.sh", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200, got %d", rec.Code)
		}

		postPayload := []byte(`{"sim_slot":2,"pref_mode":"AUTO"}`)
		reqPost := authedRequest(http.MethodPost, "/cgi-bin/quecmanager/cellular/settings.sh", postPayload)
		recPost := httptest.NewRecorder()
		mux.ServeHTTP(recPost, reqPost)

		if recPost.Code != http.StatusOK {
			t.Errorf("expected POST 200, got %d", recPost.Code)
		}
	})

	t.Run("HandleIMEISettings GET & POST", func(t *testing.T) {
		req := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/cellular/imei.sh", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200, got %d", rec.Code)
		}

		postPayload := []byte(`{"imei":"864201050123456"}`)
		reqPost := authedRequest(http.MethodPost, "/cgi-bin/quecmanager/cellular/imei.sh", postPayload)
		recPost := httptest.NewRecorder()
		mux.ServeHTTP(recPost, reqPost)

		if recPost.Code != http.StatusOK {
			t.Errorf("expected IMEI POST 200, got %d", recPost.Code)
		}
	})

	t.Run("HandleTTLSettings GET & POST", func(t *testing.T) {
		req := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/network/ttl.sh", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200, got %d", rec.Code)
		}

		postPayload := []byte(`{"enabled":true,"value":65}`)
		reqPost := authedRequest(http.MethodPost, "/cgi-bin/quecmanager/network/ttl.sh", postPayload)
		recPost := httptest.NewRecorder()
		mux.ServeHTTP(recPost, reqPost)

		if recPost.Code != http.StatusOK {
			t.Errorf("expected TTL POST 200, got %d", recPost.Code)
		}
	})

	t.Run("HandleAPN GET & POST", func(t *testing.T) {
		req := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/cellular/apn.sh", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200, got %d", rec.Code)
		}

		postPayload := []byte(`{"cid":1,"apn":"internet","pdp_type":"IPV4V6"}`)
		reqPost := authedRequest(http.MethodPost, "/cgi-bin/quecmanager/cellular/apn.sh", postPayload)
		recPost := httptest.NewRecorder()
		mux.ServeHTTP(recPost, reqPost)

		if recPost.Code != http.StatusOK {
			t.Errorf("expected APN POST 200, got %d", recPost.Code)
		}
	})

	t.Run("HandleMBN", func(t *testing.T) {
		req := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/cellular/mbn.sh", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected MBN HTTP 200, got %d", rec.Code)
		}
	})

	t.Run("HandleAuth Login Check Logout", func(t *testing.T) {
		loginPayload := []byte(`{"password":"admin"}`)
		reqLogin := httptest.NewRequest(http.MethodPost, "/cgi-bin/quecmanager/auth/login.sh", bytes.NewBuffer(loginPayload))
		recLogin := httptest.NewRecorder()
		mux.ServeHTTP(recLogin, reqLogin)

		if recLogin.Code != http.StatusOK {
			t.Fatalf("expected Login 200, got %d", recLogin.Code)
		}

		reqCheck := httptest.NewRequest(http.MethodGet, "/cgi-bin/quecmanager/auth/check.sh", nil)
		recCheck := httptest.NewRecorder()
		mux.ServeHTTP(recCheck, reqCheck)
		if recCheck.Code != http.StatusOK {
			t.Fatalf("expected Auth Check 200, got %d", recCheck.Code)
		}

		reqLogout := httptest.NewRequest(http.MethodPost, "/cgi-bin/quecmanager/auth/logout.sh", nil)
		recLogout := httptest.NewRecorder()
		mux.ServeHTTP(recLogout, reqLogout)
		if recLogout.Code != http.StatusOK {
			t.Fatalf("expected Logout 200, got %d", recLogout.Code)
		}
	})

	t.Run("HandleSMS GET & POST", func(t *testing.T) {
		reqGet := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/cellular/sms.sh", nil)
		recGet := httptest.NewRecorder()
		mux.ServeHTTP(recGet, reqGet)

		if recGet.Code != http.StatusOK {
			t.Fatalf("expected SMS GET 200, got %d", recGet.Code)
		}

		sendPayload := []byte(`{"action":"send","phone":"08123456789","message":"Test"}`)
		reqSend := authedRequest(http.MethodPost, "/cgi-bin/quecmanager/cellular/sms.sh", sendPayload)
		recSend := httptest.NewRecorder()
		mux.ServeHTTP(recSend, reqSend)

		if recSend.Code != http.StatusOK {
			t.Fatalf("expected SMS Send 200, got %d", recSend.Code)
		}
	})

	t.Run("HandlePublicOverview and Logs and Reconnect", func(t *testing.T) {
		reqOverview := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/public/overview.sh", nil)
		recOverview := httptest.NewRecorder()
		mux.ServeHTTP(recOverview, reqOverview)
		if recOverview.Code != http.StatusOK {
			t.Errorf("expected Overview 200, got %d", recOverview.Code)
		}

		reconnectPayload := []byte(`{"action":"reconnect"}`)
		reqReconnect := authedRequest(http.MethodPost, "/cgi-bin/quecmanager/system/reboot.sh", reconnectPayload)
		recReconnect := httptest.NewRecorder()
		mux.ServeHTTP(recReconnect, reqReconnect)
		if recReconnect.Code != http.StatusOK {
			t.Errorf("expected Reconnect 200, got %d", recReconnect.Code)
		}

		reqLogs := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/system/logs.sh", nil)
		recLogs := httptest.NewRecorder()
		mux.ServeHTTP(recLogs, reqLogs)
		if recLogs.Code != http.StatusOK {
			t.Errorf("expected Logs 200, got %d", recLogs.Code)
		}
	})

	t.Run("HandleDataUsed and CellScanStatus and FetchData and SendCommand", func(t *testing.T) {
		reqData := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/network/data_used.sh", nil)
		recData := httptest.NewRecorder()
		mux.ServeHTTP(recData, reqData)
		if recData.Code != http.StatusOK {
			t.Errorf("expected DataUsed 200, got %d", recData.Code)
		}

		reqScan := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/at_cmd/cell_scan_status.sh", nil)
		recScan := httptest.NewRecorder()
		mux.ServeHTTP(recScan, reqScan)
		if recScan.Code != http.StatusOK {
			t.Errorf("expected CellScanStatus 200, got %d", recScan.Code)
		}

		reqFD := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/at_cmd/fetch_data.sh", nil)
		recFD := httptest.NewRecorder()
		mux.ServeHTTP(recFD, reqFD)
		if recFD.Code != http.StatusOK {
			t.Errorf("expected FetchData 200, got %d", recFD.Code)
		}

		cmdPayload := []byte(`{"command":"ATI"}`)
		reqCmd := authedRequest(http.MethodPost, "/cgi-bin/quecmanager/at_cmd/send_command.sh", cmdPayload)
		recCmd := httptest.NewRecorder()
		mux.ServeHTTP(recCmd, reqCmd)
		if recCmd.Code != http.StatusOK {
			t.Errorf("expected SendCommand 200, got %d", recCmd.Code)
		}
	})

	t.Run("HandleSignalHistory PingHistory FrequencyLock", func(t *testing.T) {
		reqSig := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/at_cmd/fetch_signal_history.sh", nil)
		recSig := httptest.NewRecorder()
		mux.ServeHTTP(recSig, reqSig)
		if recSig.Code != http.StatusOK {
			t.Errorf("expected FetchSignalHistory 200, got %d", recSig.Code)
		}

		reqPing := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/at_cmd/fetch_ping_history.sh", nil)
		recPing := httptest.NewRecorder()
		mux.ServeHTTP(recPing, reqPing)
		if recPing.Code != http.StatusOK {
			t.Errorf("expected FetchPingHistory 200, got %d", recPing.Code)
		}

		reqFreq := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/frequency/lock.sh", nil)
		recFreq := httptest.NewRecorder()
		mux.ServeHTTP(recFreq, reqFreq)
		if recFreq.Code != http.StatusOK {
			t.Errorf("expected FrequencyLock 200, got %d", recFreq.Code)
		}
	})

	t.Run("HandleProfilesList and HandleScenariosList", func(t *testing.T) {
		reqP := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/profiles/list.sh", nil)
		recP := httptest.NewRecorder()
		mux.ServeHTTP(recP, reqP)
		if recP.Code != http.StatusOK {
			t.Errorf("expected ProfilesList 200, got %d", recP.Code)
		}

		reqS := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/scenarios/list.sh", nil)
		recS := httptest.NewRecorder()
		mux.ServeHTTP(recS, reqS)
		if recS.Code != http.StatusOK {
			t.Errorf("expected ScenariosList 200, got %d", recS.Code)
		}

		applyPayload := []byte(`{"iccid":"89860100000000000001"}`)
		reqApply := authedRequest(http.MethodPost, "/cgi-bin/quecmanager/profiles/apply.sh", applyPayload)
		recApply := httptest.NewRecorder()
		mux.ServeHTTP(recApply, reqApply)
		if recApply.Code != http.StatusOK {
			t.Errorf("expected ProfilesApply 200, got %d", recApply.Code)
		}
	})

	t.Run("HandleHealthCheck and LanguagePacks", func(t *testing.T) {
		reqHC := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/system/health-check/status.sh", nil)
		recHC := httptest.NewRecorder()
		mux.ServeHTTP(recHC, reqHC)
		if recHC.Code != http.StatusOK {
			t.Errorf("expected HealthCheckStatus 200, got %d", recHC.Code)
		}

		reqHCRun := authedRequest(http.MethodPost, "/cgi-bin/quecmanager/system/health-check/run.sh", nil)
		recHCRun := httptest.NewRecorder()
		mux.ServeHTTP(recHCRun, reqHCRun)
		if recHCRun.Code != http.StatusOK {
			t.Errorf("expected HealthCheckRun 200, got %d", recHCRun.Code)
		}

		reqLang := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/system/language-packs/list.sh", nil)
		recLang := httptest.NewRecorder()
		mux.ServeHTTP(recLang, reqLang)
		if recLang.Code != http.StatusOK {
			t.Errorf("expected LanguagePacksList 200, got %d", recLang.Code)
		}

		reqLangInst := authedRequest(http.MethodPost, "/cgi-bin/quecmanager/system/language-packs/install.sh", []byte(`{"code":"id"}`))
		recLangInst := httptest.NewRecorder()
		mux.ServeHTTP(recLangInst, reqLangInst)
		if recLangInst.Code != http.StatusOK {
			t.Errorf("expected LanguagePacksInstall 200, got %d", recLangInst.Code)
		}
	})

	t.Run("HandleMonitoringAlerts Watchdog Tailscale", func(t *testing.T) {
		reqA := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/monitoring/alerts.sh", nil)
		recA := httptest.NewRecorder()
		mux.ServeHTTP(recA, reqA)
		if recA.Code != http.StatusOK {
			t.Errorf("expected Alerts 200, got %d", recA.Code)
		}

		reqW := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/monitoring/watchdog.sh", nil)
		recW := httptest.NewRecorder()
		mux.ServeHTTP(recW, reqW)
		if recW.Code != http.StatusOK {
			t.Errorf("expected Watchdog 200, got %d", recW.Code)
		}

		reqV := authedRequest(http.MethodGet, "/cgi-bin/quecmanager/vpn/tailscale.sh", nil)
		recV := httptest.NewRecorder()
		mux.ServeHTTP(recV, reqV)
		if recV.Code != http.StatusOK {
			t.Errorf("expected Tailscale 200, got %d", recV.Code)
		}
	})
}
