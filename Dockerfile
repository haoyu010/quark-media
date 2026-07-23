# Quark Media - Go multi-stage
FROM golang:1.22-alpine AS builder
WORKDIR /src
RUN apk add --no-cache git ca-certificates tzdata
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY VERSION ./VERSION
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /out/quark-media ./cmd/quark-media

FROM alpine:3.20
ENV TZ=Asia/Shanghai \
    QM_CONFIG=/app/config/config.yaml \
    QM_HOME=/app
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata \
 && ln -snf /usr/share/zoneinfo/$TZ /etc/localtime \
 && echo $TZ > /etc/timezone \
 && mkdir -p /app/data /app/strm /app/config /app/data/mtproto /app/third_party
COPY --from=builder /out/quark-media /app/quark-media
COPY web /app/web
COPY config.example.yaml /app/config.example.yaml
COPY category.yaml /app/category.yaml
COPY VERSION /app/VERSION
COPY third_party/quark-auto-save-x /app/third_party/quark-auto-save-x
COPY docker/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh
EXPOSE 18025
VOLUME ["/app/data", "/app/strm", "/app/config"]
ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["serve"]
