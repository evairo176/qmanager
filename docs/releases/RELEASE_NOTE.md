# 🚀 QManager Go Edition v0.2.4-stable

**First stable release.** QManager Go Edition v0.2.4-stable is the production build
running on the Quectel RM500Q-GL (dashboard 192.168.225.1). It brings 100% Go
Native API coverage across all WebUI modules, native 5G NR5G-SA (Standalone) &
NR5G-NSA signal parsing, universal modem hardware auto-discovery, and a dedicated
Python diagnostic toolkit.

## ✨ New Features & Enhancements

- **100% Go Native API Handlers**: Replaced all remaining legacy shell CGI handlers with high-performance in-memory Go endpoints (`/network/ethernet.sh`, `/frequency/status.sh`, `/tower/status.sh`, `/at_cmd/speedtest_start.sh`, `/cellular/network_priority.sh`, `/cellular/apn.sh`, `/cellular/imei.sh`, `/cellular/fplmn.sh`, `/system/known_sims.sh`, etc.).
- **5G Standalone (NR5G-SA) & NSA Signal Parsing**: Poller engine now accurately decodes 5G ARFCN, PCI, Band (N12/N28/N41/N77/N78), RSRP (-92 dBm), RSRQ, and SINR from Quectel `+QENG` serving cell responses.
- **Universal Modem Hardware Auto-Discovery**: Dynamic query of Model, Firmware, IMEI, ICCID, IMSI, Carrier Operator, and WAN IPv4/IPv6 address via 3GPP and Quectel AT commands (`ATI`, `AT+CGMI`, `AT+GMM`, `AT+GSN`, `AT+QCCID`, `AT+CIMI`, `AT+COPS?`, `AT+CGPADDR`).
- **Modem Python Diagnostic Toolkit**: Added `tests/py_modem/` containing 23 Python SSH/AT diagnostic tools with sanitized environment-driven authentication (`SSH_HOST`, `SSH_USERNAME`, `SSH_PASSWORD`).

---

## 📦 Package Distribution & Hardware Details

Refer to the table below to select the appropriate binary or deployment method for your setup. For in-depth technical breakdowns, see the [Hardware Support Documentation](https://github.com/latifangren/QManager-GO/blob/dev-go/docs/HARDWARE-SUPPORT.md).

| Hardware Platform | Qualcomm Chipset | Operating System | Binary Executable | Target Modem / Host Devices |
| :--- | :--- | :--- | :--- | :--- |
| **ARMv7 32-bit (SDX55 / SDX62 / SDX65)** | SDX55, SDX62, SDX65 | Linux + Systemd | `qmanager-core-armv7.tar.gz` | **Quectel RM520N-GL**, RM500Q-GL, RM502Q-AE, **RG501Q-EU**, RM521F-GL |
| **ARMv8 64-bit / ARM64 (SDX72 / SDX75)** | SDX72, SDX75 | Native OpenWRT (`init.d`) | `qmanager-core-arm64.tar.gz` | **Quectel RM551E-GL**, RM550E-GL, RG650V-EU |
| **Host Router & Gateways (ARM64)** | Any (Passthrough) | OpenWRT / Linux | `qmanager-core-arm64.tar.gz` | Raspberry Pi 4/5, NanoPi, GL.iNet, FriendlyWrt |
| **PC & Router Hardware (x86_64)** | Any (Passthrough) | Linux / OpenWRT x86 | `qmanager-core-amd64.tar.gz` | x86 Routers, Mini PCs, MikroTik CHR, Linux VMs |

---

## 📥 Installation & Deployment

### 1-Click Flashing (Recommended)

```bash
# Workstation Flashing via SSH (Linux / macOS / Git Bash)
./deploy.sh 192.168.225.1

# Workstation Flashing via ADB
./deploy.sh adb

# Workstation Flashing (Windows PowerShell)
.\deploy.ps1 -Target "192.168.225.1"
```

---

## 🔗 Quick Reference Links

- 📜 [Full Changelog](https://github.com/latifangren/QManager-GO/blob/dev-go/docs/releases/CHANGELOG.md)
- 📱 [Hardware Support Guide](https://github.com/latifangren/QManager-GO/blob/dev-go/docs/HARDWARE-SUPPORT.md)
- ⚙️ [System Architecture](https://github.com/latifangren/QManager-GO/blob/dev-go/docs/ARCHITECTURE.md)
- 🌐 [Language Packs Manifest](https://github.com/latifangren/QManager-GO/blob/dev-go/docs/language-packs/manifest.json)

---

## 💙 Thank You

Thank you to all contributors, testers, and community members! Bug reports and feature requests are welcome via GitHub Issues.
