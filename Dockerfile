FROM golang:1.26 AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64

ENV CGO_ENABLED=0
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /app/duck2api .

FROM alpine:3.22

WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata ffmpeg

COPY --from=builder /app/duck2api /app/duck2api

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/ping >/dev/null || exit 1

CMD ["/app/duck2api"]
