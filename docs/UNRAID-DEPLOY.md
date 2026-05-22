# Unraid deployment

This walks through running the VinylStream stack on an Unraid box, behind an existing nginx that already terminates TLS for other subdomains (e.g. `git.revela.dev`).

The plan in two phases:

1. **Local testing**: bring the stack up on the Unraid host, expose ports on the LAN, verify OBS push and browser playback work.
2. **Public exposure**: add an nginx server block for `radio.revela.dev`, point DNS at the Unraid box, issue a TLS cert, drop the LAN port binding.

## Prerequisites

- Unraid 6.12+ (anything with the Docker engine works).
- One of:
  - The **Docker Compose Manager** plugin from Community Apps (recommended; gives you a UI tab for the stack), or
  - SSH access to the Unraid host so you can `git clone` and run `docker compose` directly.
- Existing nginx instance on the Unraid host (the one already handling `git.revela.dev`).
- Domain control over `revela.dev` so you can add a `radio` A record.

## Phase 1: Local testing

### 1. Place the project under appdata

By Unraid convention, app data lives under `/mnt/user/appdata/`. SSH into the host and:

```sh
cd /mnt/user/appdata
git clone https://github.com/itsRevela/RevelaRadio.git vinylstream
cd vinylstream
cp .env.example .env
```

Edit `.env`:

- Set strong values for `ICECAST_SOURCE_PASSWORD`, `ICECAST_RELAY_PASSWORD`, `ICECAST_ADMIN_PASSWORD` (32+ random chars each is fine, no quoting needed since these are not shell-evaluated).
- For phase 1, set `VINYLSTREAM_BIND=0.0.0.0` so you can hit the webapp from a LAN browser without nginx in the way.
- Leave `VINYLSTREAM_HOSTNAME=localhost` for now. We'll set it to `radio.revela.dev` in phase 2.
- Pick a host port for the backend if 8080 is taken on your Unraid (a lot of Unraid setups use 8080 for the UI). Try 8765 or similar:
  ```
  VINYLSTREAM_PORT=8765
  ```
- Icecast defaults to 8000. If another container has 8000, change `ICECAST_PORT` and remember to use the new port when configuring OBS.

### 2. Bring the stack up

If you have the Docker Compose Manager plugin, add `/mnt/user/appdata/vinylstream` as a project and click Compose Up.

From SSH:

```sh
cd /mnt/user/appdata/vinylstream
docker compose up --build -d
docker compose ps
```

You should see two containers (`vinylstream-icecast-1` and `vinylstream-vinylstream-1`) in the `Up` state. Caddy stays down because it's behind the `caddy` profile.

### 3. Verify from a LAN browser

Open `http://<unraid-ip>:8765/` (replace 8765 with whatever `VINYLSTREAM_PORT` is). You should see the VinylStream UI with status "Offline".

### 4. Push from OBS

Point OBS at `icecast://source:<ICECAST_SOURCE_PASSWORD>@<unraid-ip>:8000/live.flac` per `docs/OBS-SETUP.md`. Hit **Start Recording**. The UI should flip to "Live now" within 5 seconds and pressing play on the audio control should produce audio.

If anything's off here, fix it before doing phase 2. Logs:

```sh
docker compose logs -f vinylstream icecast
```

## Phase 2: Public exposure via radio.revela.dev

### 1. Lock the backend back down

Once the LAN test passes, edit `.env` and switch the backend back to localhost-only so the only way in from outside is via nginx:

```
VINYLSTREAM_BIND=127.0.0.1
VINYLSTREAM_HOSTNAME=radio.revela.dev
```

Bounce the stack:

```sh
docker compose up -d
```

(The `VINYLSTREAM_HOSTNAME` change goes into Icecast's advertised hostname; not required for it to work, but cleaner.)

### 2. DNS

Add an A record for `radio.revela.dev` pointing at the same public IP as `git.revela.dev`. Wait for it to propagate (`dig radio.revela.dev`).

### 3. nginx vhost

A ready-to-go server block is in `deploy/nginx/radio.revela.dev.conf`. From SSH:

```sh
sudo cp /mnt/user/appdata/vinylstream/deploy/nginx/radio.revela.dev.conf /etc/nginx/sites-available/
sudo ln -s /etc/nginx/sites-available/radio.revela.dev.conf /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

(Paths vary. If you keep configs in `/etc/nginx/conf.d/`, drop it there instead. If nginx itself runs in a container on Unraid, copy the file into that container's config volume instead.)

### 4. TLS cert

If you're using certbot:

```sh
sudo certbot --nginx -d radio.revela.dev
```

certbot will edit the server block to point at the right cert paths and add a redirect. If you're using acme.sh or another tool, use its standard flow. The cert paths in `radio.revela.dev.conf` assume certbot's default layout (`/etc/letsencrypt/live/...`); change them if you use a different tool.

### 5. Source-client routing

OBS still pushes directly to Icecast on port 8000, NOT through nginx. The TLS cert only protects the listener-facing webapp and FLAC fanout. To verify nothing routes the source push through nginx by mistake, leave `ICECAST_BIND=0.0.0.0` so 8000 stays open at the LAN/WAN level for OBS, but make sure your firewall only allows 8000 from your OBS source IP if it's coming over the public internet. If OBS is on the LAN and the Unraid is the streaming box, leaving 8000 LAN-only via `ICECAST_BIND=192.168.x.y` (the LAN-side address) is tighter.

### 6. Verify the public flow

- Browse to `https://radio.revela.dev/` and confirm the UI loads over TLS.
- Press play. Audio should stream.
- Open dev tools, Network tab: confirm `/live.flac` and `/ws` are served from `radio.revela.dev` (not the Unraid LAN IP) and that the WebSocket upgrades to `wss://`.

## Operations cheatsheet

```sh
# Where you are
cd /mnt/user/appdata/vinylstream

# Live logs
docker compose logs -f vinylstream icecast

# Update to latest commit
git pull
docker compose up --build -d

# Wipe listener history (will recreate on next snapshot)
docker compose down
docker volume rm vinylstream_vinylstream_data
docker compose up -d

# Rotate the Icecast source password
$EDITOR .env
docker compose up -d --force-recreate icecast
# ...then update OBS's Custom Output URL with the new password.
```

## Known gotchas

- **Port 8080 is often busy on Unraid.** The default Unraid webUI binds 8080. Always set `VINYLSTREAM_PORT` to something else, or check `netstat -lnt | grep 8080` first.
- **/mnt/user vs /mnt/cache.** If you put the project on cache for speed (`/mnt/cache/appdata/...`) note that the Mover will eventually relocate it to the array unless you configure the share's Use Cache setting accordingly.
- **Docker bridge vs br0.** This compose uses the default Docker bridge network internally and only publishes a couple of host ports. Don't switch the services to br0/macvlan unless you have a reason; you'll lose internal DNS-by-service-name.
- **SQLite persistence.** The `vinylstream_data` volume holds listener history. If you blow it away you'll lose the graphs but nothing else.
