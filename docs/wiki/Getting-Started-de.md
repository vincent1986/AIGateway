# Erste Schritte

**Sprachen:** [EN](Getting-Started-en) · [中文](Getting-Started-zh-CN) · [日本語](Getting-Started-ja) · [Deutsch](Getting-Started-de) · [Tiếng Việt](Getting-Started-vi) · [繁體中文](Getting-Started-zh-TW) · [Wiki-Start](Home)

## 5 Schritte

1. **AIGateway installieren und starten**  
   Paket von den [Releases](https://github.com/vincent1986/AIGateway/releases) laden und die App starten.

2. **Gateway-Status prüfen**  
   Standardadresse: `http://127.0.0.1:18080/v1`  
   Im Bereich **Gateway** prüfen, ob der Dienst läuft.

3. **Anbieter hinzufügen**  
   **Anbieter** → Preset wählen (DeepSeek, SiliconFlow, Ollama, Qwen, …) → API-Key einfügen → Modelle laden.

4. **Modellgruppen konfigurieren**  
   **Modelle** → virtuelle Gruppen prüfen → per Drag-and-Drop Failover-Priorität setzen (oben primär, unten Backup).

5. **Tools per Ein-Klick übernehmen**  
   **Apps** → ChatGPT / Claude Code / OpenClaw / Harness → **Übernehmen**.  
   Modelle und Anbieter wechseln Sie danach nur noch in AIGateway.

## Client-Einrichtung

| Szenario | Empfehlung |
|----------|------------|
| Nach Übernahme | Tools zeigen bereits aufs lokale Gateway; `model` = **Gruppenname** |
| Manuell | `base_url` = `http://127.0.0.1:18080/v1`, API-Key z. B. `aigateway` |
| Codex / ChatGPT | `model_provider = "aigateway"`, model = Gruppenname |

## Architektur

```
Tools (ChatGPT / Claude Code / OpenClaw / …)
        │  base_url → http://127.0.0.1:18080/v1
        ▼
   AIGateway (Routing / Failover / Token-Statistik)
        │
        ▼
Upstream (DeepSeek / SiliconFlow / Ollama / …)
```

## Weiter

- [FAQ](FAQ-de)
- [Daten & Pfade](Data-Paths-de)
- [Fehlerbehebung](Troubleshooting-de)
