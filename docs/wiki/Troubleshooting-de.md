# Fehlerbehebung

**Sprachen:** [EN](Troubleshooting-en) · [中文](Troubleshooting-zh-CN) · [日本語](Troubleshooting-ja) · [Deutsch](Troubleshooting-de) · [Tiếng Việt](Troubleshooting-vi) · [繁體中文](Troubleshooting-zh-TW) · [Start](Home)

## Gateway nicht erreichbar

1. AIGateway läuft und **Gateway** zeigt aktiv.  
2. `http://127.0.0.1:18080/v1` im Browser/curl prüfen (Port-Konflikt?).  
3. Tools zeigen noch auf alte Base-URL → **Übernahme** erneut.  
4. Firewall/Proxy blockiert `127.0.0.1`?

## Codex: Missing environment variable: aigateway_api_key

**Inline**-`api_key` in der Provider-Konfiguration nutzen:

```toml
api_key = "aigateway"
```

Nicht auf undefinierte Variable `aigateway_api_key` verlassen.

## Codex: reservierte Provider-IDs

Keine eingebauten IDs `openai` / `ollama` als Custom-Namen. Stattdessen z. B.:

- `openai` → `openai-custom`
- `ollama` → `ollama-local`

## model_group_all_exhausted

Alle Kanäle der **virtuellen Modellgruppe** sind fehlgeschlagen oder ohne Kontingent (oft 429).

Maßnahmen:

1. API-Keys, Guthaben, Limits prüfen.  
2. In **Modelle** Backups hinzufügen und Priorität anpassen.  
3. Anbieter und Modelle **aktivieren**.

## Kein Failover mitten im Stream

HTTP-/**First-Byte**-Fehler können failovern; **nach Stream-Start** zum Client oft nicht nahtlos.  
Client-Retry löst erneutes Routing aus.

## Tool nutzt weiterhin die offizielle API

1. Status „übernommen“ prüfen.  
2. CLI/IDE-Sitzung neu starten.  
3. OpenClaw: `models.providers.aigateway` verwenden.  
4. **Entfernen/Wiederherstellen**, dann erneut übernehmen.

## Nutzungsstatistik falsch

- SQLite-Statistik ist maßgeblich.  
- Manche Upstreams senden kein usage → 0 oder unvollständig.

## Immer noch Probleme?

1. [FAQ](FAQ-de)  
2. [Neueste Release](https://github.com/vincent1986/AIGateway/releases)  
3. [Issue öffnen](https://github.com/vincent1986/AIGateway/issues/new/choose) (OS, Version, Schritte; **API-Keys schwärzen**)
