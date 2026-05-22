FROM golang:1.22-bookworm AS build
WORKDIR /src

# Cache module downloads.
COPY go.mod ./
RUN go mod download || true

COPY . .
# Generate go.sum on first build (we don't ship one yet) then compile.
RUN go mod tidy && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/vinylstream ./cmd/server

FROM gcr.io/distroless/static-debian12
WORKDIR /app

COPY --from=build /out/vinylstream /app/vinylstream
COPY web /app/web

# Distroless has no shell, so we can't chown a mounted /data volume at start.
# Running as root inside a no-shell image leaves a minimal attack surface and
# keeps the SQLite path writable regardless of volume ownership.
EXPOSE 8080
ENTRYPOINT ["/app/vinylstream"]
