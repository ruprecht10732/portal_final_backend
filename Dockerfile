# Build stage
FROM golang:1.24.12-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /app/bin/server ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /app/bin/scheduler ./cmd/scheduler

# Runtime stage
FROM alpine:3.20

RUN addgroup -S app && adduser -S app -G app && apk add --no-cache ca-certificates curl

WORKDIR /app
COPY --from=builder /app/bin/server /app/server
COPY --from=builder /app/bin/scheduler /app/scheduler
COPY --from=builder /app/migrations /app/migrations
COPY healthcheck.sh /app/healthcheck.sh

RUN chmod +x /app/healthcheck.sh

ENV HTTP_ADDR=:8080
ENV SERVICE_ROLE=api
EXPOSE 8080

USER app

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD ["/app/healthcheck.sh"]

ENTRYPOINT ["/app/server"]
