FROM golang:1.22-bookworm AS build
WORKDIR /src

# Cache module downloads.
COPY go.mod ./
RUN go mod download || true

COPY . .
# Generate go.sum on first build (we don't ship one yet) then compile.
RUN go mod tidy && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/vinylstream ./cmd/server

# Distroless static :nonroot pins UID 65532 with no shell and no other
# binaries. The /data volume must be owned by 65532 on the host;
# docker-compose.yml relies on a one-time chown of the named volume.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

COPY --from=build --chown=65532:65532 /out/vinylstream /app/vinylstream
COPY --chown=65532:65532 web /app/web

USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/app/vinylstream"]
