FROM golang:1.25.1-bookworm AS builder
ENV GOMAXPROCS=1 GOFLAGS=-p=1 GOMEMLIMIT=360MiB GOGC=60 GOTOOLCHAIN=local
RUN apt-get update && apt-get install -y --no-install-recommends libpcap-dev && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN mkdir /out && go build -trimpath -o /out/cctvscan ./cmd/cctvscan && go build -trimpath -o /out/cctvscan-server ./cmd/cctvscan-server && go build -trimpath -o /out/cctvscan-worker ./cmd/cctvscan-worker
FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends libpcap0.8 ca-certificates ffmpeg masscan curl tar && rm -rf /var/lib/apt/lists/* && mkdir -p /var/lib/cctvscan/jobs && chown -R 1000:1000 /var/lib/cctvscan && chmod 700 /var/lib/cctvscan /var/lib/cctvscan/jobs
COPY --from=builder --chmod=0555 /out/ /usr/local/bin/
COPY --chmod=0555 deploy/healthcheck.sh /usr/local/bin/console-healthcheck
USER 1000:1000
WORKDIR /var/lib/cctvscan
CMD ["cctvscan-server"]
