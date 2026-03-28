FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod ./
COPY . .
RUN go build -o downloader .

FROM alpine:3.21

RUN apk add --no-cache \
    ffmpeg \
    python3 \
    py3-pip \
    && pip install --no-cache-dir yt-dlp --break-system-packages

WORKDIR /app
COPY --from=builder /app/downloader .

VOLUME ["/downloads"]

CMD ["./downloader"]
