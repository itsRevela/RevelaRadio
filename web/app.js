// VinylStream frontend.
(function () {
    "use strict";

    const VOLUME_STORAGE_KEY = "vinylstream:volume";

    const el = {
        streamName: document.getElementById("stream-name"),
        streamDescription: document.getElementById("stream-description"),
        statusDot: document.getElementById("status-dot"),
        statusLabel: document.getElementById("status-label"),
        audio: document.getElementById("audio"),
        playBtn: document.getElementById("play-btn"),
        playLabel: document.querySelector("#play-btn .play-label"),
        volumeSlider: document.getElementById("volume-slider"),
        listeners: document.getElementById("stat-listeners"),
        peak: document.getElementById("stat-peak"),
        uptime: document.getElementById("stat-uptime"),
        format: document.getElementById("stat-format"),
        sampleRate: document.getElementById("stat-samplerate"),
        bitDepth: document.getElementById("stat-bitdepth"),
        chart: document.getElementById("history-chart"),
        rangeButtons: document.querySelectorAll(".range-toggle button"),
    };

    const state = {
        range: "24h",
        meta: null,
        lastStream: null,
    };

    function fmtNumber(n) {
        if (n === null || n === undefined || Number.isNaN(n)) return "--";
        return String(n);
    }

    function fmtUptime(seconds) {
        if (!seconds || seconds <= 0) return "--";
        const d = Math.floor(seconds / 86400);
        const h = Math.floor((seconds % 86400) / 3600);
        const m = Math.floor((seconds % 3600) / 60);
        if (d > 0) return d + "d " + h + "h";
        if (h > 0) return h + "h " + m + "m";
        if (m > 0) return m + "m";
        return seconds + "s";
    }

    function fmtSampleRate(hz) {
        if (!hz || hz <= 0) return "--";
        if (hz % 1000 === 0) return (hz / 1000) + " kHz";
        return (hz / 1000).toFixed(1) + " kHz";
    }

    function fmtFormat(contentType) {
        if (!contentType) return "--";
        const ct = contentType.toLowerCase();
        if (ct.includes("flac") || ct === "audio/ogg" || ct === "application/ogg") return "FLAC";
        if (ct.includes("mpeg") || ct === "audio/mp3") return "MP3";
        if (ct.includes("aac")) return "AAC";
        if (ct.includes("opus")) return "Opus";
        if (ct.includes("vorbis")) return "Vorbis";
        if (ct.includes("wav")) return "WAV";
        return contentType;
    }

    function applyStatus(stream) {
        if (!stream) return;
        state.lastStream = stream;
        if (stream.online) {
            el.statusDot.dataset.state = "online";
            el.statusLabel.textContent = "Live now";
        } else {
            el.statusDot.dataset.state = "offline";
            el.statusLabel.textContent = "Offline";
        }
        el.listeners.textContent = fmtNumber(stream.listeners);
        el.uptime.textContent = fmtUptime(stream.uptime_seconds);
        el.format.textContent = fmtFormat(stream.content_type);
        el.sampleRate.textContent = fmtSampleRate(stream.sample_rate);
    }

    function applyMeta(meta) {
        if (!meta) return;
        state.meta = meta;
        if (meta.name) {
            el.streamName.textContent = meta.name;
            document.title = meta.name;
        }
        if (meta.description) {
            el.streamDescription.textContent = meta.description;
        }
        if (meta.mount_path && el.audio.getAttribute("src") !== meta.mount_path) {
            el.audio.setAttribute("src", meta.mount_path);
            // Don't auto-load; the play button does it on first click so we
            // don't keep a stream connection open in the background.
        }
        el.bitDepth.textContent = meta.bit_depth || "--";
    }

    async function fetchInitial() {
        try {
            const res = await fetch("/api/status", { cache: "no-store" });
            if (!res.ok) throw new Error("status " + res.status);
            const data = await res.json();
            applyMeta(data.meta);
            applyStatus(data.stream);
        } catch (err) {
            console.warn("fetchInitial failed", err);
            el.statusLabel.textContent = "Unable to reach server";
        }
    }

    async function fetchHistory(range) {
        try {
            const res = await fetch("/api/history?range=" + encodeURIComponent(range), { cache: "no-store" });
            if (!res.ok) throw new Error("history status " + res.status);
            const data = await res.json();
            el.peak.textContent = fmtNumber(data.peak);
            renderHistory(data.snapshots || []);
        } catch (err) {
            console.warn("fetchHistory failed", err);
        }
    }

    function renderHistory(snapshots) {
        const svg = el.chart;
        while (svg.firstChild) svg.removeChild(svg.firstChild);
        if (snapshots.length === 0) return;

        const W = 600;
        const H = 160;
        const padX = 4;
        const padY = 12;

        const first = new Date(snapshots[0].observed_at).getTime();
        const last = new Date(snapshots[snapshots.length - 1].observed_at).getTime();
        const span = Math.max(1, last - first);
        const peak = snapshots.reduce((m, s) => Math.max(m, s.listeners || 0), 1);

        function x(t) {
            return padX + ((new Date(t).getTime() - first) / span) * (W - padX * 2);
        }
        function y(v) {
            return H - padY - ((v || 0) / peak) * (H - padY * 2);
        }

        const ns = "http://www.w3.org/2000/svg";

        let line = "";
        snapshots.forEach((s, i) => {
            line += (i === 0 ? "M" : "L") + x(s.observed_at).toFixed(2) + "," + y(s.listeners).toFixed(2);
        });
        const lastPx = x(snapshots[snapshots.length - 1].observed_at);
        const firstPx = x(snapshots[0].observed_at);
        const area = line +
            "L" + lastPx.toFixed(2) + "," + (H - padY).toFixed(2) +
            "L" + firstPx.toFixed(2) + "," + (H - padY).toFixed(2) + "Z";

        const baseline = document.createElementNS(ns, "line");
        baseline.setAttribute("class", "axis");
        baseline.setAttribute("x1", String(padX));
        baseline.setAttribute("x2", String(W - padX));
        baseline.setAttribute("y1", String(H - padY));
        baseline.setAttribute("y2", String(H - padY));
        svg.appendChild(baseline);

        const areaPath = document.createElementNS(ns, "path");
        areaPath.setAttribute("class", "area");
        areaPath.setAttribute("d", area);
        svg.appendChild(areaPath);

        const linePath = document.createElementNS(ns, "path");
        linePath.setAttribute("class", "line");
        linePath.setAttribute("d", line);
        svg.appendChild(linePath);
    }

    function bindRangeButtons() {
        el.rangeButtons.forEach((btn) => {
            btn.addEventListener("click", () => {
                el.rangeButtons.forEach((b) => b.classList.remove("active"));
                btn.classList.add("active");
                state.range = btn.dataset.range;
                fetchHistory(state.range);
            });
        });
    }

    // --- Custom audio player ---

    function setPlayLabel(playing) {
        el.playBtn.setAttribute("aria-pressed", playing ? "true" : "false");
        el.playLabel.textContent = playing ? "Stop Stream" : "Play Stream";
    }

    function bindPlayer() {
        // Restore last-used volume.
        let stored = parseFloat(localStorage.getItem(VOLUME_STORAGE_KEY) || "");
        if (Number.isNaN(stored) || stored < 0 || stored > 1) stored = 0.8;
        el.audio.volume = stored;
        el.volumeSlider.value = String(Math.round(stored * 100));

        el.volumeSlider.addEventListener("input", () => {
            const v = Math.max(0, Math.min(100, Number(el.volumeSlider.value))) / 100;
            el.audio.volume = v;
            localStorage.setItem(VOLUME_STORAGE_KEY, String(v));
        });

        el.playBtn.addEventListener("click", () => {
            if (el.audio.paused) {
                // Force a fresh fetch every time so we don't get a stale buffer.
                el.audio.load();
                el.audio.play().catch((err) => {
                    console.warn("audio play failed", err);
                    el.statusLabel.textContent = "Playback blocked or stream unavailable";
                });
            } else {
                el.audio.pause();
                // Stopping should fully release the upstream connection so
                // we don't count as a ghost listener on Icecast.
                el.audio.removeAttribute("src");
                if (state.meta && state.meta.mount_path) {
                    el.audio.setAttribute("src", state.meta.mount_path);
                }
            }
        });

        el.audio.addEventListener("play",  () => setPlayLabel(true));
        el.audio.addEventListener("pause", () => setPlayLabel(false));
        el.audio.addEventListener("ended", () => setPlayLabel(false));
        el.audio.addEventListener("error", () => {
            setPlayLabel(false);
            el.statusLabel.textContent = "Stream error";
        });
    }

    // --- WebSocket live updates ---

    function connectWebSocket() {
        const scheme = location.protocol === "https:" ? "wss:" : "ws:";
        const url = scheme + "//" + location.host + "/ws";
        let backoff = 1000;

        function open() {
            const sock = new WebSocket(url);
            sock.addEventListener("open", () => { backoff = 1000; });
            sock.addEventListener("message", (ev) => {
                try {
                    const msg = JSON.parse(ev.data);
                    if (msg.stream) applyStatus(msg.stream);
                } catch (err) {
                    console.warn("ws message parse", err);
                }
            });
            sock.addEventListener("close", () => {
                setTimeout(open, backoff);
                backoff = Math.min(backoff * 2, 30000);
            });
            sock.addEventListener("error", () => { sock.close(); });
        }
        open();
    }

    document.addEventListener("DOMContentLoaded", () => {
        bindRangeButtons();
        bindPlayer();
        fetchInitial();
        fetchHistory(state.range);
        connectWebSocket();
        // Refresh history (peak + chart) on a longer interval since the
        // live listener count comes via WebSocket.
        setInterval(() => fetchHistory(state.range), 60000);
    });
})();
