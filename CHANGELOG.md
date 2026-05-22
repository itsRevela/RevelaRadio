# Changelog

All notable changes to VinylStream will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial project scaffold: Icecast 2 + Go backend + Caddy reverse proxy, orchestrated with docker-compose.
- Custom Icecast container (`icecast/Dockerfile`) with an env-templated `icecast.xml` so source/admin passwords come from the `.env` file rather than baked into the image.
- Go backend (`cmd/server`) with:
  - Icecast `/admin/stats` poller (configurable interval, default 5s).
  - SQLite-backed listener history (`internal/history`, pure-Go driver, no CGO).
  - WebSocket fan-out hub (`internal/ws`) that pushes live status to connected browsers.
  - REST API: `GET /api/status`, `GET /api/history?range={1h,6h,24h,7d}`, `GET /api/meta`, `GET /healthz`.
- Vanilla HTML/CSS/JS frontend (`web/`) with an `<audio>` player pointed at `/live.flac`, live listener counter via WebSocket, and an inline SVG history chart.
- Caddy config (`Caddyfile`) that terminates TLS, proxies `/live.flac` directly to Icecast unbuffered, and routes everything else to the Go backend.
- Setup documentation:
  - `README.md`: architecture, dev workflow, ops notes.
  - `docs/OBS-SETUP.md`: step-by-step OBS Custom Output (FFmpeg) configuration for pushing FLAC to Icecast via the Recording feature.
