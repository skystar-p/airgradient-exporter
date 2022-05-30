FROM golang:1.18.2-alpine3.16 AS build
WORKDIR /app
COPY . .
RUN go build -o airgradient-exporter

# ===

FROM alpine:3.16
MAINTAINER Jaehyeon Park <skystar@skystar.dev>
COPY --from=build /app/airgradient-exporter /app
ENTRYPOINT ["/app/airgradient-exporter"]
