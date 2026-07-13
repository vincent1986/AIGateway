# FAQ

**Sprachen:** [EN](FAQ-en) · [中文](FAQ-zh-CN) · [日本語](FAQ-ja) · [Deutsch](FAQ-de) · [Tiếng Việt](FAQ-vi) · [繁體中文](FAQ-zh-TW) · [Start](Home)

Häufige Fragen. Siehe auch [Fehlerbehebung](Troubleshooting-de) und [Erste Schritte](Getting-Started-de).

---

## 1. Produkt

### Was ist AIGateway?

Lokales **KI-Modellmanagement + Traffic-Gateway**. Verbindet Multi-Provider-LLM-APIs mit Tools wie ChatGPT/Codex, Claude Code, OpenClaw, Harness:

- **Tokens sparen** — Nutzung pro Anbieter/Modell  
- **Günstigere Anbieter** — viele APIs an einem Ort  
- **Einmal einrichten** — Tools zeigen nur aufs lokale Gateway  
- **Hot Routing** — virtuelle Modellgruppen + Prioritäts-Failover  

### Unterschied zu manueller Tool-Konfiguration?

V2: **einmal übernehmen, dauerhaft routen**. `base_url` einmal setzen; Modelle/Anbieter danach in AIGateway wechseln.

### Verlassen Daten den Rechner?

AIGateway läuft lokal. Requests gehen an von Ihnen konfigurierte Upstream-APIs. Daten unter `~/.codex-manager/`. API-Keys selbst schützen.

---

## 2. Installation

### Betriebssysteme?

macOS (Apple Silicon / Intel), Windows, Linux — [Releases](https://github.com/vincent1986/AIGateway/releases).

### Docker / Node nötig?

Desktop-Release: nein. Source-Build: Go + Frontend-Toolchain.

---

## 3. Gateway

### Standard-URL?

```
http://127.0.0.1:18080/v1
```

### Tools anbinden?

**Apps → Ein-Klick-Übernahme**. Manuell: Base URL wie oben, API-Key `aigateway`, `model` = **Gruppenname**.

### Muss die App laufen?

Ja. Ohne AIGateway kein lokaler Proxy.

---

## 4. Anbieter

**Anbieter** → Preset → API-Key → Modelle laden. Meist reicht der Key.

| Modus | Nutzen |
|-------|--------|
| **Standard OpenAI** | Normalisierte Request/Response |
| **Passthrough** | Body weitgehend durchreichen |

Zuerst Standard OpenAI testen.

---

## 5. Modelle & Failover

Virtuelle Gruppe = gleiche/äquivalente Modelle über Anbieter. Failover bei 429, Quota, 401-ähnlich. Alles tot → `model_group_all_exhausted`.

Stream-Mitte: eingeschränkt — siehe [Fehlerbehebung](Troubleshooting-de).

Priorität: in **Modelle** per Drag-and-Drop.

---

## 6. App-Übernahme

Ein-Klick: **ChatGPT (Codex)**, **Claude Code**, **OpenClaw**, **Harness**. Backup, dann Base-URL aufs Gateway. Wiederherstellen über die Karte oder `~/.codex-manager/backups/`.

Codex reservierte IDs / `aigateway_api_key`: [Fehlerbehebung](Troubleshooting-de).

---

## 7. Tokens & Daten

Tab **Nutzung**, SQLite `aigateway.db` — [Daten & Pfade](Data-Paths-de). Keys gehen nicht zu GitHub.

---

## 8. UI-Sprachen

Popup: Vereinfachtes Chinesisch, Traditionelles Chinesisch, English, Japanisch, Koreanisch, Deutsch, Vietnamesisch, Thai.

Fünf Bereiche: Anbieter / Modelle / Apps / Gateway / Nutzung.

---

## 9. Upgrade

v2: JSON → SQLite-Migration. Gateway- und Übernahme-Status prüfen.

---

## 10. Feedback

Issues: https://github.com/vincent1986/AIGateway/issues/new/choose  
Lizenz: [MIT](https://github.com/vincent1986/AIGateway/blob/main/LICENSE) © Mars Waller

| Item | Wert |
|------|------|
| Base URL | `http://127.0.0.1:18080/v1` |
| Lokaler API-Key | `aigateway` |
| Daten | `~/.codex-manager/` |
| Exhausted-Code | `model_group_all_exhausted` |
