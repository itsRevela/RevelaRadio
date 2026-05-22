// VinylStream frontend. Hooks into the Go backend for status + history,
// and the /live.flac mount served via Caddy/Icecast for audio.
(function () {
    "use strict";

    const el = {
        streamName: document.getElementById("stream-name"),
        streamDescription: document.getElementById("stream-description"),
        statusDot: document.getElementById("status-dot"),
        statusLabel: document.getElementById("status-label"),
        audio: document.getElementById("audio"),
        audioSource: document.getElementById("audio-source"),
        nowPlaying: document.getElementById("now-playing"),
        npText: document.getElementById("np-text"),
        listeners: document.getElementById("stat-listeners"),
        peak: document.getElementById("stat-peak"),
        bitrate: document.getElementById("stat-bitrate"),
        chart: document.getElementById("history-chart"),
        rangeButtons: document.querySelectorAll(".range-toggle button"),
    };

    const state = {
        range: "24h",
    };

    function fmtNumber(n) {
        if (n === null || n === undefined || Number.isNaN(n)) return "--";
        return String(n);
    }

    function applyStatus(stream) {
        if (!stream) return;
        if (stream.online) {
            el.statusDot.dataset.state = "online";
            el.statusLabel.textContent = "Live now";
        } else {
            el.statusDot.dataset.state = "offline";
            el.statusLabel.textContent = "Offline";
        }
        el.listeners.textContent = fmtNumber(stream.listeners);
        el.bitrate.textContent = stream.bitrate ? fmtNumber(stream.bitrate) : "--";

        const np = [stream.artist, stream.stream_title].filter(Boolean).join(" - ");
        if (np) {
            el.nowPlaying.hidden = false;
            el.npText.textContent = np;
        } else {
            el.nowPlaying.hidden = true;
        }
    }

    function applyMeta(meta) {
        if (!meta) return;
        if (meta.name) {
            el.streamName.textContent = meta.name;
            document.title = meta.name;
        }
        if (meta.description) {
            el.streamDescription.textContent = meta.description;
        }
        if (meta.mount_path) {
            el.audioSource.src = meta.mount_path;
            el.audio.load();
        }
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
        // Clear previous children.
        while (svg.firstChild) svg.removeChild(svg.firstChild);

        if (snapshots.length === 0) {
            return;
        }

        const W = 600;
        const H = 160;
        const padX = 4;
        const padY = 12;

        const first = new Date(snapshots[0].observed_at).getTime();
        const last = new Date(snapshots[snapshots.length - 1].observed_at).getTime();
        const span = Math.max(1, last - first);
        const peak = snapshots.reduce((m, s) => Math.max(m, s.listeners || 0), 1);

        function x(t) {
            const tt = new Date(t).getTime();
            return padX + ((tt - first) / span) * (W - padX * 2);
        }
        function y(v) {
            const norm = (v || 0) / peak;
            return H - padY - norm * (H - padY * 2);
        }

        const ns = "http://www.w3.org/2000/svg";

        // Build path strings.
        let line = "";
        let area = "";
        snapshots.forEach((s, i) => {
            const px = x(s.observed_at);
            const py = y(s.listeners);
            line += (i === 0 ? "M" : "L") + px.toFixed(2) + "," + py.toFixed(2);
        });
        if (snapshots.length > 0) {
            const lastPx = x(snapshots[snapshots.length - 1].observed_at);
            const firstPx = x(snapshots[0].observed_at);
            area = line + "L" + lastPx.toFixed(2) + "," + (H - padY).toFixed(2) +
                   "L" + firstPx.toFixed(2) + "," + (H - padY).toFixed(2) + "Z";
        }

        const baseline = document.createElementNS(ns, "line");
        baseline.setAttribute("class", "axis");
        baseline.setAttribute("x1", String(padX));
        baseline.setAttribute("x2", String(W - padX));
        baseline.setAttribute("y1", String(H - padY));
        baseline.setAttribute("y2", String(H - padY));
        svg.appendChild(baseline);

        if (area) {
            const areaPath = document.createElementNS(ns, "path");
            areaPath.setAttribute("class", "area");
            areaPath.setAttribute("d", area);
            svg.appendChild(areaPath);
        }
        if (line) {
            const linePath = document.createElementNS(ns, "path");
            linePath.setAttribute("class", "line");
            linePath.setAttribute("d", line);
            svg.appendChild(linePath);
        }
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

    function connectWebSocket() {
        const scheme = location.protocol === "https:" ? "wss:" : "ws:";
        const url = scheme + "//" + location.host + "/ws";
        let backoff = 1000;

        function open() {
            const sock = new WebSocket(url);
            sock.addEventListener("open", () => {
                backoff = 1000;
            });
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
            sock.addEventListener("error", () => {
                sock.close();
            });
        }

        open();
    }

    document.addEventListener("DOMContentLoaded", () => {
        bindRangeButtons();
        fetchInitial();
        fetchHistory(state.range);
        connectWebSocket();
        // Refresh history periodically so the chart keeps pace with the WS-driven live count.
        setInterval(() => fetchHistory(state.range), 60000);
    });
})();
