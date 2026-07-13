# Daten & Pfade

**Sprachen:** [EN](Data-Paths-en) · [中文](Data-Paths-zh-CN) · [日本語](Data-Paths-ja) · [Deutsch](Data-Paths-de) · [Tiếng Việt](Data-Paths-vi) · [繁體中文](Data-Paths-zh-TW) · [Start](Home)

AIGateway speichert den Zustand unter `.codex-manager` im Benutzerverzeichnis (plattformübergreifend).

## Wichtige Pfade

| Inhalt | Pfad |
|--------|------|
| SQLite (Anbieter, Modellgruppen, Routing, Nutzung) | `~/.codex-manager/aigateway.db` |
| Anbieter-JSON-Spiegel | `~/.codex-manager/providers.json` |
| Gateway-Konfiguration | `~/.codex-manager/proxy.json` |
| Tool-Backups | `~/.codex-manager/backups/` |
| Umgebungsbezogene Dateien | `~/.codex-manager/env/` |

> Unter Windows ist `~` Ihr Benutzerprofil (z. B. `C:\Users\<Sie>`).

## Upgrade von v1

- Beim ersten Start von v2 werden alte `providers.json` / `usage.json` nach SQLite migriert.
- **SQLite** ist maßgeblich; manche Listen-Einstellungen können in `proxy.json` liegen.

## Backup

- Vor Upgrade/Neuinstallation den gesamten Ordner `~/.codex-manager/` sichern.
- Ein-Klick-Übernahme erstellt Snapshots der Tool-Konfiguration (Rollback in der App möglich).

Siehe: [Erste Schritte](Getting-Started-de) · [Fehlerbehebung](Troubleshooting-de)
