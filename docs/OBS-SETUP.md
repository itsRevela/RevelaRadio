# OBS setup for VinylStream

OBS's Custom Output (FFmpeg) lets it push audio to Icecast over the standard source protocol. We use the **Recording** path (not Streaming) because OBS's Streaming tab is locked to RTMP-style outputs.

This means: when you want to go live, you press **Start Recording**, not Start Streaming. It feels wrong; it isn't.

## Prerequisites

- OBS Studio (any recent version; Custom Output FFmpeg has been stable for years).
- The VinylStream stack is running and you know:
  - The Icecast hostname (`stream.example.com` in prod, `localhost` in local dev)
  - The Icecast port (`8000` unless you changed it)
  - The source password (the `ICECAST_SOURCE_PASSWORD` value from your `.env`)
  - The mount path (`/live.flac` by default)

## Configure the audio source

If you haven't already, add the audio source you want to broadcast (turntable input, Voicemeeter virtual cable, application capture, etc.) to your OBS scene. Verify the audio meter is moving.

In **Settings > Audio > Advanced**:
- Sample Rate: **48 kHz** (recommended) or 44.1 kHz. Match your source if possible.
- Channels: **Stereo**.

OBS resamples internally, so the channel/rate here is what gets encoded.

## Configure the FFmpeg output

In **Settings > Output**:

1. Set **Output Mode** to **Advanced**.
2. Switch to the **Recording** tab.
3. Configure as follows:

| Field                | Value                                                                                 |
| -------------------- | ------------------------------------------------------------------------------------- |
| Type                 | **Custom Output (FFmpeg)**                                                            |
| FFmpeg Output Type   | **Output to URL**                                                                     |
| File path or URL     | `icecast://source:<YOUR_SOURCE_PASSWORD>@<HOST>:8000/live.flac`                       |
| Container Format     | `ogg`                                                                                 |
| Container Format Description | (leave blank)                                                                 |
| Muxer Settings       | `content_type=application/ogg`                                                        |
| Video bitrate / size | **(does not matter, we disable video below)**                                         |
| Keyframe interval    | 0                                                                                     |
| Rescale Output       | unchecked                                                                             |
| Show all codecs      | checked (so FLAC is available)                                                        |
| Video Encoder        | **No Video Encoder** (uncheck the video output checkbox if present)                   |
| Audio Encoder        | `flac`                                                                                |
| Audio bitrate        | (ignored for FLAC since it's lossless; the bitrate field has no effect)               |
| Audio Track          | 1                                                                                     |

### About the URL

`icecast://source:PASSWORD@HOST:PORT/MOUNT`

- `source` is the literal Icecast source username. Leave it as `source`.
- `PASSWORD` is `ICECAST_SOURCE_PASSWORD` from your `.env`.
- `HOST` is the Icecast hostname. If OBS is running on the same machine as Docker, use `localhost`. Over the internet, use your domain.
- `PORT` is `8000` (Icecast). **Not 443/Caddy.** Source clients connect directly to Icecast's port, not through the TLS reverse proxy.
- `MOUNT` is `/live.flac`.

### Disabling video

OBS's Recording tab still expects a video stream by default. To produce an audio-only output:

- If your OBS version shows a "Video Encoder" dropdown with a "No Video" option, pick it.
- Otherwise: in the **Output > Recording** tab, leave the video encoder at the default but uncheck any scene/video output, OR add `-vn` to **Muxer Settings**. Easiest path that reliably works on current OBS: set Muxer Settings to:
  ```
  content_type=application/ogg
  ```
  and set **FFmpeg Video Encoder** to `none` if available. If your OBS UI doesn't expose `none`, set Video Bitrate to its lowest value and accept the negligible overhead. FLAC audio will still dominate the stream and Icecast will only forward what the container holds.

### Audio Encoder Settings

In **Settings > Output > Recording > Audio Encoder Settings**, you can leave it blank. If you want to be explicit:
```
compression_level=5
```
(FLAC compression level: 5 is the default. Higher = smaller output, more CPU.)

## Go live

1. Click **Start Recording** in OBS's main window. Watch OBS's log (**Help > Log Files > View Current Log**) for any FFmpeg connection errors.
2. Within ~5 seconds the VinylStream webapp should flip the status dot from grey/red to green ("Live now").
3. Test playback: open the webapp in a separate browser tab and press play on the audio control. You should hear your source.

## Stopping

Click **Stop Recording**. Icecast will drop the mount, the webapp will show "Offline" on the next poll (within 5s).

## Troubleshooting

**OBS says "Failed to open output URL".**
- Wrong password or wrong host. Double-check the URL field.
- Icecast not running. `docker compose ps icecast` should show it healthy.
- Firewall blocking outbound 8000 from OBS to the server.

**Stream connects but no audio in the browser.**
- The browser may have cached an older state. Hard-refresh the page (Ctrl+Shift+R) and press Play again.
- Open the direct URL `https://<host>/live.flac` in a new tab. If VLC/Safari/Firefox can play it directly, the audio path is fine and it's a frontend issue.
- Confirm the `Container Format` is `ogg` (not `flac` standalone, which won't stream over Icecast).

**OBS log shows "Connection refused" on Start Recording.**
- Icecast container isn't reachable on the configured host/port. If OBS is on the same machine as Docker, make sure `docker-compose.yml` exposes `8000:8000` (it does by default).

**Listener count never increments.**
- Browser autoplay policies may be blocking. The play button has to be pressed at least once per session.
- Check the Go backend log: `docker compose logs -f vinylstream`. You should see polls succeeding every 5s.

## Alternative: BUTT

If OBS's Recording-as-streaming workflow feels too awkward, [BUTT (Broadcast Using This Tool)](https://danielnoethen.de/butt/) is a free, purpose-built Icecast source client. You can run BUTT alongside OBS: OBS handles your scene/visuals, BUTT handles the audio push. The Icecast source URL is the same.
