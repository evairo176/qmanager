# 📜 Changelog

All notable changes to the **QManager Go Edition** project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [v0.2.4-stable] - 2026-09-11

**🎉 First stable release of QManager Go Edition.** Versi ini adalah yang sedang
berjalan di Quectel RM500Q-GL (dashboard 192.168.225.1). Semua fitur di bawah
digabung dari v0.2.1-go → v0.2.4-beta.1 dan diverifikasi stabil di produksi.

### 🚀 Penggabungan Fitur (v0.2.1 → v0.2.4)
- **100% Go Native API Handlers** (`pkg/api/`): semua CGI handler shell legacy
  diganti handler Go in-memory (ethernet, frequency lock, tower lock, speedtest,
  apn, imei, network priority, fplmn, known sims, pending reboot, bandwidth).
- **Go Poller Engine** (`pkg/daemon/poller.go`): parser 5G NR5G-SA & NSA
  (`+QENG="servingcell"` → ARFCN, 5G PCI, band N12/N28/N41/N77/N78, RSRP, RSRQ, SINR).
- **Universal Modem Auto-Discovery**: model, firmware, IMEI, ICCID, IMSI, operator,
  WAN IPv4/IPv6 via 3GPP/Quectel AT.
- **SSE Real-Time Telemetry** (`pkg/api/sse.go`): push stream status tanpa polling.
- **Speedtest Engine native Go** (latency, download, upload) + **nftables DPI manager**.
- **Dual-SIM/eSIM manager** (`pkg/modem/sim.go`): slot query, switch, ICCID.
- **Watchdog & SIM Failover**: recovery lifecycle log ke network feed, tier 1-4,
  backup SIM slot, max reboots/hour.
- **Design System Overhaul**: glass surfaces, gradient buttons, glow cards,
  sidebar active pills, cockpit gauges, Animated Gear, Realtime toggle (1 toggle
  stop semua polling - hemat CPU pada modem 1-core).
- **Bandwidth**: WebSocket go-native (`ws://host:8838/`) menggantikan websocat.
- **SMS Center**: native `sms_tool` backend + normalize shape, fix crash.
- **IPA offload, custom DNS, MTU, ping profile, adaptive polling, hostname**
  (baca dari qmanager.conf `[settings].hostname`, bukan os.Hostname "sdxprairie").
- **Config Backup/Restore** (`config-backup-sections.sh`), **Python diagnostic
  toolkit** (`tests/py_modem/`, 23 tools).

### 📦 Artifacts
`qmanager-core-armv7.tar.gz` (RM500Q-GL / SDX55-65) · `qmanager-core-arm64.tar.gz`
(SDX72/75, RPi) · `qmanager-core-amd64.tar.gz` (PC/router x86) — workflow
`.github/workflows/release.yml` build otomatis saat tag `v*`.

---

## [v0.2.4-beta.1] - 2026-08-27

### 🚀 Added & Enhanced
- **Full Go Native API Coverage**: Replaced all remaining legacy shell CGI handlers with 100% Go Native in-memory HTTP handlers (`pkg/api/`):
  - Ethernet Physical Link Status & Auto-Negotiation (`/cgi-bin/quecmanager/network/ethernet.sh`).
  - Frequency Channel Locking (`/cgi-bin/quecmanager/frequency/status.sh` & `lock.sh`).
  - Tower Cell Locking Status, Settings, and Scheduling (`/cgi-bin/quecmanager/tower/status.sh`, `settings.sh`, `schedule.sh`).
  - Built-in Speedtest Engine (`speedtest_check.sh`, `speedtest_servers.sh`, `speedtest_start.sh`).
  - APN & IMEI Management (`cellular/apn.sh`, `cellular/imei.sh`).
  - Network Priority Order (`cellular/network_priority.sh`).
  - Forbidden PLMN (`cellular/fplmn.sh`), Known SIMs (`system/known_sims.sh`), Pending Reboot (`system/pending_reboot.sh`), and Bandwidth Settings (`monitoring/bandwidth.sh`).
- **5G NR5G-SA & NR5G-NSA Poller Engine** (`pkg/daemon/poller.go`): Enhanced `+QENG="servingcell"` parser for 5G Standalone and Non-Standalone modes, mapping ARFCN, 5G PCI, Band (N12/N28/N41/N77/N78), RSRP, RSRQ, and SINR cleanly into state cache.
- **Universal Modem Hardware Auto-Discovery**: Dynamic non-hardcoded query of Manufacturer, Model, Firmware, IMEI, ICCID, IMSI, Carrier Operator, and WAN IPv4/IPv6 address via standard 3GPP and Quectel AT commands (`ATI`, `AT+CGMI`, `AT+GMM`, `AT+GSN`, `AT+QCCID`, `AT+CIMI`, `AT+COPS?`, `AT+CGPADDR`).
- **Modem Python Diagnostic Toolkit** (`tests/py_modem/`): Organized 23 standalone Python SSH/AT diagnostic tools with `requirements.txt` and `README.md` using environment-driven credential loading.

---

## [v0.2.4-stable] - 2026-08-25

### 🚀 Added & Enhanced
- **AT Serial Port Auto-Discovery** (`pkg/at/client.go`): Dynamic auto-scan of candidate serial ports (`/dev/smd11` → `/dev/smd7` → `/dev/ttyUSB2` → `/dev/ttyUSB3` → `/dev/ttyUSB0` → `/dev/ttyACM0` → `/dev/cdc-wdm0`) for non-SoC modem host routers.
- **Smart 1-Click Deployment Tooling** (`deploy.sh`, `deploy.ps1`): Automatic target CPU architecture detection (`uname -m`) and init system auto-detection (`systemd` vs `procd init.d`).
- **OpenWRT Procd Init Script** (`scripts/etc/init.d/qmanager-core`): Added native OpenWRT init.d service script for Go single-binary execution on OpenWRT devices without Systemd.
- **Comprehensive Hardware Support Matrix** (`docs/HARDWARE-SUPPORT.md`): Detailed documentation covering Qualcomm chipset platforms (SDX55/62/65 vs SDX72/75), ARMv7 32-bit vs ARMv8/ARM64 64-bit parity, and On-Modem vs Host Router deployment modes.

## [v0.2.2-go] - 2026-08-24

### 🚀 Added & Enhanced
- **Real-Time Telemetry via Server-Sent Events (SSE)** (`pkg/api/sse.go` & `use-sse-telemetry.ts`): Replaced 1s HTTP polling with high-performance, low-overhead SSE push stream (`/cgi-bin/quecmanager/api/stream/status`).
- **Built-In Native Speedtest Engine** (`pkg/speedtest/speedtest.go` & `speedtest-card.tsx`): Native Go multi-threaded HTTP latency, download, and upload throughput tester.
- **Native nftables & DPI Rules Manager** (`pkg/firewall/firewall.go`): Native rule generator and file manager for `/etc/nftables.d/12-mangle-qmanager-dpi.nft` supporting NFQUEUE 200 packet inspection and TTL mangle modification.
- **Dual-SIM & eSIM Manager** (`pkg/modem/sim.go` & `sim-slot-card.tsx`): Hardware SIM slot query (`AT+QUIMSLOT?`), slot switching (`AT+QUIMSLOT=1/2`), and ICCID parser (`AT+QCCID`).
- **AES-256-GCM Encrypted Backup & Restore** (`pkg/backup/backup.go`): PBKDF2 (SHA-256) password-protected encryption for `.qmbackup` configuration archives.
- **In-Memory Token Bucket Rate Limiter & Brute-Force Guard** (`pkg/api/ratelimit.go`): Protected login and serial AT command execution against brute-force and flooding attacks.
- **Native Watchdog Goroutine** (`pkg/daemon/watchdog.go`): Non-blocking TCP/HTTP connectivity probe goroutine updating system watchdog status without external shell scripts.
- **OpenWRT UCI Native Go Parser** (`pkg/uci/uci.go`): Zero-dependency thread-safe UCI config parser and serializer.

### 🛠️ Changed & Optimized
- **GitHub Actions CI Optimization**: Added Bun store caching, Next.js `.next/cache` restoration, and `concurrency` cancellation, reducing workflow run times by ~60%.
- **Upstream Repository & Language Pack Routing**: Repointed all OTA update daemons, bootstrap installer (`qmanager-installer.sh`), and i18n language pack manifests to `latifangren/QManager-GO`.

## [v0.2.1-go] - 2026-08-24

### 🛠️ Changed & Refactored
- **Native `src/` Directory Layout**: Migrated all Next.js frontend code (`app/`, `components/`, `hooks/`, `lib/`, `types/`, `constants/`) into standard `src/` directory with clean `@/*` path mapping in `tsconfig.json` & `components.json`.
- **Root Repository Organization**: Organized multi-language READMEs to `docs/readme/`, design specifications to `docs/design/`, release logs to `docs/releases/`, dev scripts to `scripts/dev/`, and language pack manifest to `docs/language-packs/`.
- **Core Documentation Sync**: Updated `CLAUDE.md`, `docs/ARCHITECTURE.md`, `docs/BACKEND.md`, and `docs/DEPLOYMENT.md` to accurately reflect single-binary Go architecture (`qmanager-core`).

## [v0.2.0-go] - 2026-08-24

### 🚀 Added
- **Native Go Core Engine (`qmanager-core`)**: Replaced lighttpd, shell CGI handlers, and process subshell spawning with a single compiled Go binary (`qmanager-core`).
- **Single-Binary SPA Embedding (`embed.FS`)**: Embedded Next.js 16 static export frontend directly into `qmanager-core` binary with disk fallback via `WEB_ROOT` environment variable.
- **Auto TLS/HTTPS Certificate Generator (`tlsgen`)**: Created package `backend/pkg/tlsgen` to automatically generate X.509 ECDSA P-256 self-signed SSL/TLS certificates for local modem IP addresses on HTTPS port 443 out-of-the-box.
- **1-Click Workstation Deployment Tooling**: Created `deploy.sh` (POSIX Bash/macOS/Linux) and `deploy.ps1` (PowerShell/Windows) for single-command modem flashing over SSH or ADB.
- **5G NR Subcarrier Spacing (SCS) Support**: Implemented automatic SCS parsing (15, 30, 60, 120 kHz) in `AT+QSCAN` cell survey parsing and formatted `AT+QNWLOCK="common/5g"` cell locks with valid SCS to prevent modem command rejections.
- **GitHub Actions Workflows**: Added modular CI/CD workflows modeled after `BFR-WEBUI-GO`:
  - `.github/workflows/quality.yml`: `go test` unit testing & `golangci-lint` verification.
  - `.github/workflows/release.yml`: Automated Next.js UI export + Go ARMv7 cross-compilation + tarball packaging (`qmanager-core-armv7.tar.gz`) + GitHub Release publishing on tag push.
  - `.github/workflows/cleanup.yml`: Weekly workflow run & actions cache purge.
  - `.github/workflows/codeql.yml`: Weekly CodeQL static security scans for Go and TypeScript.
- **Thread-Safe Dual AT Engine**: Serial mutex (`sync.Mutex`) + file lock (`/tmp/qmanager_at.lock`) with mock fallback for local non-modem testing environments.
- **Asynchronous Status Poller Goroutine**: Non-blocking background status collector updating state atomics every 5 seconds without `fork()` overhead.
- **100% CGI Route Compatibility**: Implemented Go HTTP handlers for Auth (`login.sh`, `check.sh`, `logout.sh`), Modem Status (`fetch_data.sh`, `send_command.sh`), Bands (`current.sh`, `lock.sh`), SMS Inbox/Send/Delete, Settings, Data Usage Counter (`/proc/net/dev`), History Charts (NDJSON), Tower Lock, APN Management, Custom SIM Profiles, Monitoring Alerts & Watchdog, Tailscale VPN, Health Checks, and Language Packs.

### 🛠️ Changed
- **60-70% RAM Footprint Reduction**: Reduced modem memory usage down to **~12 – 18 MB RAM** (vs 80+ MB on legacy shell CGI stack).
- **Sub-15ms API Response Latency**: Replaced subshell execution with native Go `net/http` in-memory routing.
- **Cleaned Git Tracking**: Configured `.gitignore` to exclude temporary `backend/web/out/*` build artifacts, preserving clean working tree in GitHub Desktop.
- **Synced Upstream Universal Branching**: Aligned primary default branch to `development-home` (Universal QManager Edition).
