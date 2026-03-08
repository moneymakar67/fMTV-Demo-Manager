# fMTV Demo Manager

A standalone Windows GUI application to parse CS:GO demos and generate detailed fMTV match reports.

## Features
- **Standalone GUI**: Built with Lorca (Chromium-based), no installation required.
- **Detailed Stats**: Tracks Kills, Assists, Deaths, K/D, K/R, HS%, Entry Kills, Clutch Kills, MVPs, and Multi-kills (3K/4K/5K).
- **KAST Tracking**: Accurate KAST (Kill, Assist, Survive, Traded) logic.
- **fMR Scoring**: Custom performance metric based on fragMount fMR formulas.
- **Batch Processing**: Process a single demo or an entire folder at once.

## Building
Requires Go 1.21+ and a Chromium-based browser (Edge/Chrome).
```bash
go build -ldflags="-H windowsgui" -o DemoManager.exe
```
