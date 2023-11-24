FROM golang:1.21.1-bookworm AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o airgradient-exporter

# ===

FROM debian:bookworm AS runner

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && update-ca-certificates

COPY --from=builder /app/airgradient-exporter /app/

CMD ["/app/airgradient-exporter"]
