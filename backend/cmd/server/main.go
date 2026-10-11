package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"

	"qmanager-backend/pkg/api"
	"qmanager-backend/pkg/at"
	"qmanager-backend/pkg/daemon"
	"qmanager-backend/pkg/tlsgen"
	"qmanager-backend/web"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "80"
	}

	tlsPort := os.Getenv("TLS_PORT")
	if tlsPort == "" {
		tlsPort = "443"
	}

	webRoot := os.Getenv("WEB_ROOT")
	tlsEnabled := os.Getenv("TLS_ENABLED") != "false" // enabled by default unless explicitly disabled

	mux := http.NewServeMux()

	// Initialize AT Client
	atClient := at.NewClient("/dev/smd11")

	// Start Background Poller Daemon (runs continuously without spawning subshells)
	poller := daemon.NewPoller(atClient, 10*time.Second)
	poller.Start()
	log.Println("[Daemon] Background Status Poller started (10s interval)")

	// Start Background Watchdog Daemon (monitors connectivity natively)
	// v1.5.3: fix komprehensif auto-reconnect.
	//  - probe 15s, multi-host (1.1.1.1 + 8.8.8.8 via defaultPing)
	//  - soft recover di 2 gagal, hard reboot di 5 gagal
	//  - single-flight (recovery gak numpuk - sebelumnya `go recoveryFunc`
	//    tiap tick bisa jalanin CFUN barengan = chaos lambat)
	//  - cooldown 90s antar hard reboot
	watchdog := daemon.NewWatchdog("1.1.1.1", 15*time.Second, 2)
	var (
		recovering    atomic.Bool
		lastHardReboot time.Time
	)
	watchdog.SetRecoveryFunc(func(fails int) {
		if !recovering.CompareAndSwap(false, true) {
			return // recovery udah jalan, gak boleh numpuk
		}
		defer recovering.Store(false)

		// Hard reboot: kasih cooldown 90s biar gak reboot-storm.
		if fails >= 5 {
			if time.Since(lastHardReboot) < 90*time.Second {
				log.Printf("[Watchdog] RECOVERY: hard reboot ditahan (cooldown 90s), fails=%d", fails)
				return
			}
			lastHardReboot = time.Now()
			log.Printf("[Watchdog] RECOVERY: consecutive fails=%d -> HARD REBOOT", fails)
			daemon.RecordWatchcatEvent(true, fmt.Sprintf("%d consecutive ping failures", fails))
			_ = exec.Command("/bin/sh", "-c", "sync; (sleep 2; busybox reboot -f) >/dev/null 2>&1 &").Start()
			return
		}

		// SOFT: COPS detach + CFUN radio reset + PDP reactivate.
		log.Printf("[Watchdog] RECOVERY: consecutive fails=%d -> soft (COPS detach + CFUN 0/1 + PDP)", fails)
		daemon.RecordWatchcatEvent(false, fmt.Sprintf("%d consecutive ping failures", fails))
		_, _ = atClient.Exec("AT+COPS=2")
		time.Sleep(2 * time.Second)
		_, _ = atClient.Exec("AT+CFUN=0")
		time.Sleep(4 * time.Second)
		_, _ = atClient.Exec("AT+CFUN=1")
		time.Sleep(12 * time.Second)
		_, _ = atClient.Exec("AT+CGACT=1,1")
		time.Sleep(5 * time.Second)
	})
	watchdog.Start()
	log.Println("[Daemon] Background Watchdog started (15s interval, multi-host, fast & single-flight)")

	// Register API Routes
	server := api.NewServer(atClient)
	server.RegisterRoutes(mux)

	// Restore persisted "remember me" sessions from disk (survives reboots).
	api.LoadSessionsFromDisk()

	// API Health Check
	mux.HandleFunc("/cgi-bin/quecmanager/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","arch":"%s","os":"%s"}`, runtime.GOARCH, runtime.GOOS)
	})

	// Static File Server: Embedded FS or Disk Fallback
	if webRoot != "" {
		log.Printf("[Web] Serving static assets from disk path: %s", webRoot)
		diskFS := http.Dir(webRoot)
		fileServer := http.FileServer(diskFS)
		mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := filepath.Join(webRoot, filepath.Clean(r.URL.Path))
			info, err := os.Stat(path)
			if err != nil && os.IsNotExist(err) {
				http.ServeFile(w, r, filepath.Join(webRoot, "index.html"))
				return
			}
			if err == nil && info.IsDir() {
				http.ServeFile(w, r, filepath.Join(path, "index.html"))
				return
			}
			fileServer.ServeHTTP(w, r)
		}))
	} else {
		log.Println("[Web] Serving static assets from embedded binary (embed.FS all:out)")
		mux.Handle("/", web.ServeEmbeddedWeb())
	}

	httpServer := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	// Live bandwidth WebSocket server (Go-native, replaces websocat bridge).
	// Reads ws_port from qmanager.conf bridge_monitor; defaults to 8838.
	server.StartBandwidthWebSocket(0)

	var tlsServer *http.Server

	// Setup HTTPS TLS Certificates if enabled
	if tlsEnabled {
		certPath, keyPath, err := tlsgen.EnsureCertificates("")
		if err != nil {
			log.Printf("[TLS] Warning: Failed to initialize TLS certs: %v", err)
		} else {
			log.Printf("[TLS] HTTPS Enabled on port %s (cert: %s)", tlsPort, certPath)
			tlsServer = &http.Server{
				Addr:    ":" + tlsPort,
				Handler: mux,
			}
			go func() {
				if err := tlsServer.ListenAndServeTLS(certPath, keyPath); err != nil && err != http.ErrServerClosed {
					log.Printf("[TLS] HTTPS server stopped: %v", err)
				}
			}()
		}
	}

	// Channel to listen for OS signals (SIGINT, SIGTERM)
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("QManager Go Backend listening on HTTP port %s\n", port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Block until signal is received
	sig := <-stopChan
	log.Printf("[System] Received OS signal: %v. Initiating graceful shutdown...", sig)

	// Create a deadline context for shutdown (5 seconds max)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Stop background daemons
	watchdog.Stop()

	// Shutdown HTTPS server if running
	if tlsServer != nil {
		if err := tlsServer.Shutdown(ctx); err != nil {
			log.Printf("[TLS] Warning: HTTPS shutdown error: %v", err)
		}
	}

	// Shutdown HTTP server
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("[HTTP] Warning: HTTP shutdown error: %v", err)
	}

	log.Println("[System] QManager Go Core Daemon stopped cleanly.")
}
