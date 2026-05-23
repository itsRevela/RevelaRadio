# Changelog

All notable changes to VinylStream will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Background image (`web/spellground-bg.png`) tiled across the page with a radial vignette overlay to soften repeat seams on ultrawide/widescreen/vertical viewports.
- New stat cards on the player page: `Uptime`, `Format`, `Sample Rate`, `Bit Depth`. Sample rate and channels are parsed from Icecast's per-source `<channels>`/`<samplerate>` elements with a fallback to the semicolon-packed `audio_info` string. Bit depth is shown from the `STREAM_BIT_DEPTH` env var since Icecast doesn't surface it.
- Footer "Contact" link pointing at the Fluxer channel.

### Changed
- Page heading and subheading switched to `radio.revela.dev` and `Lossless vinyl audio livestream`. Driven by the `STREAM_NAME` / `STREAM_DESCRIPTION` env vars in `.env`, no code change needed for further edits.
- Removed the bitrate (kbps) stat. FLAC's bitrate varies block-by-block, so it was misleading; the bit depth + sample rate combo is the relevant lossless-stream signal.
- Removed the "Now Playing" panel (was an unwired placeholder).
- Audio control replaced with a custom `Play Stream` / `Stop Stream` toggle button plus a volume slider. The browser's default `<audio controls>` chrome is hidden. Volume is persisted to `localStorage` and restored across sessions.
- Stopping playback now releases the `<audio>` element's source so the listener doesn't linger as a ghost on Icecast's listener count.

### Security
- vinylstream backend container now runs as UID 65532:65532 (the distroless `nonroot` user) instead of root. Requires a one-time chown of the `vinylstream_vinylstream_data` named volume; the README documents the command. Trims privileges in the unlikely event of an RCE.
- WebSocket upgrade no longer accepts arbitrary origins. `internal/ws/hub.go` previously had `InsecureSkipVerify: true`, which let any cross-origin site open a WebSocket to `/ws` from a victim's browser. Now defaults to same-origin only. Embed the player from other hosts by setting `VINYLSTREAM_ALLOWED_ORIGINS` to a comma-separated glob list (e.g. `*.revela.dev`).
- Container image versions pinned: `gcr.io/distroless/static-debian12:nonroot` instead of floating `latest`. Build-stage `golang:1.22-bookworm` retained (build-only, no runtime exposure).

### Fixed
- Audio player in the webapp was non-functional (play button blocked by `/live.flac` 404) when the backend was reached on a different host port than Icecast. The backend now reverse-proxies the audio mount (and the `.m3u` / `.xspf` playlist variants) straight to Icecast with `FlushInterval = -1`, so listeners pull audio chunks unbuffered. This makes the same-origin assumption baked into the static frontend valid regardless of whether Caddy/NPM/nginx are in front.

### Changed
- Caddy is now opt-in via the `caddy` compose profile. The default `docker compose up -d` brings up only `icecast` + `vinylstream` so an upstream reverse proxy (e.g. nginx already on the Unraid host) can front the stack without a second TLS terminator in the way.
- Backend HTTP port is now bound to `127.0.0.1:8080` by default (override with `VINYLSTREAM_BIND` and `VINYLSTREAM_PORT`). Keeps the Go server unreachable from the LAN unless explicitly opened, since the reverse proxy is the intended entry point.
- Icecast host port is configurable via `ICECAST_BIND` and `ICECAST_PORT` (defaults `0.0.0.0:8000`) so source clients can still push to it without the listener-facing port being broadcast on every interface.

### Added
- `deploy/nginx/radio.revela.dev.conf`: drop-in nginx server block for terminating TLS in front of the stack. Includes the unbuffered `/live.flac` proxy and WebSocket upgrade rules.
- `docs/UNRAID-DEPLOY.md`: Unraid-specific deploy walkthrough covering appdata layout, the Docker Compose Manager plugin path, LAN-side smoke testing, then cutting over to a public `radio.revela.dev` vhost via the existing nginx.

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
