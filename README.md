# VinylStream

A lossless audio livestream webapp. OBS pushes FLAC to an Icecast 2 server; a Go backend wraps the stream with a browser-friendly UI (player, live listener count, history chart) and Caddy fronts everything with TLS.

## Architecture

```
OBS (FFmpeg custom output, FLAC) ──> Icecast 2 (mount: /live.flac)
                                          │
                                          ▼
                                    Go backend ──> SQLite (listener history)
                                          │
                                          ▼
                                   Caddy (TLS reverse proxy)
                                          │
                                          ▼
                                     Listeners' browsers
                                  (<audio src="/live.flac">)
```

- **Icecast 2** handles source ingest and listener fanout. The webapp does not sit in the audio path.
- **Go backend** polls Icecast's `/admin/stats` every 5s, persists snapshots to SQLite, and fans out live updates to browsers over WebSocket.
- **Caddy** terminates TLS on :443, reverse-proxies `/live.flac` straight to Icecast (chunked, unbuffered) and everything else to the Go backend.
- **Frontend** is plain HTML + JS, no build step.

## Project layout

```
.
├── cmd/server/             Go entrypoint
├── internal/
│   ├── api/                REST handlers + shared status cache
│   ├── config/             env loading
│   ├── history/            SQLite store for listener snapshots
│   ├── icecast/            Icecast admin XML client
│   └── ws/                 WebSocket hub
├── web/                    static frontend (index.html, app.js, style.css)
├── icecast/                Dockerfile + icecast.xml template
├── Caddyfile               TLS + reverse proxy config
├── docker-compose.yml      three-service stack
├── Dockerfile              Go backend image
└── docs/OBS-SETUP.md       OBS source-client walkthrough
```

## Quick start

1. Copy the env template and pick passwords:
   ```sh
   cp .env.example .env
   $EDITOR .env
   ```
   For local dev, leave `VINYLSTREAM_HOSTNAME=localhost`. For prod, point your domain at the host and set `VINYLSTREAM_HOSTNAME=stream.example.com` so Caddy can fetch a Let's Encrypt cert.

2. Build and start the stack:
   ```sh
   docker compose up --build -d
   ```

3. Open the webapp at:
   - Local dev: <http://localhost> (Caddy serves plain HTTP for `localhost`)
   - Prod: `https://<your hostname>`

4. Configure OBS to push FLAC to the Icecast mount. See [docs/OBS-SETUP.md](docs/OBS-SETUP.md).

5. Hit Start Recording in OBS. The UI will flip to "Live now" within ~5 seconds.

## Dev workflow

Run the backend outside Docker for fast iteration:

```sh
# 1. Start Icecast in Docker (or run it natively).
docker compose up -d icecast

# 2. Run the Go backend locally pointed at the dockerised Icecast.
export VINYLSTREAM_LISTEN=:8080
export VINYLSTREAM_DB=./vinylstream.db
export ICECAST_URL=http://localhost:8000
export ICECAST_MOUNT=/live.flac
export ICECAST_ADMIN_USER=admin
export ICECAST_ADMIN_PASSWORD=changeme-admin
go run ./cmd/server
```

Then open <http://localhost:8080>. The frontend will hit `/live.flac` on the same host - for fully local audio playback you'll want Caddy running too (so `/live.flac` is proxied to Icecast), or you can edit `web/app.js` to point at `http://localhost:8000/live.flac` during dev.

## Operations

- **Logs:** `docker compose logs -f vinylstream icecast caddy`
- **Listener count history:** persisted in the `vinylstream_data` named volume (`/data/vinylstream.db`).
- **Bandwidth budget:** FLAC at 44.1kHz/16-bit averages ~900 kbps. 50 listeners ≈ 45 Mbps egress. Check your VPS plan.
- **Source password rotation:** edit `.env`, run `docker compose up -d --force-recreate icecast`. Update OBS afterwards.

## Security checklist

- [ ] Change every `changeme-*` value in `.env` before exposing publicly.
- [ ] If OBS pushes over the public internet, the source password is your only ingest auth. Use a strong one (32+ random chars).
- [ ] Consider binding Icecast's port 8000 to `127.0.0.1` in `docker-compose.yml` if your source client and the server are on the same machine or LAN.
- [ ] Caddy auto-renews TLS. Verify `caddy_data` is on a persistent volume so renewals don't get rate-limited.

## License

TBD.
