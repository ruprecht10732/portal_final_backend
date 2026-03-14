# Build stage
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache cmake make g++ git linux-headers

# Build whisper.cpp static libraries (pinned to Go module commit)
RUN git init /tmp/whisper.cpp \
    && cd /tmp/whisper.cpp \
    && git remote add origin https://github.com/ggerganov/whisper.cpp.git \
    && git fetch --depth 1 origin 30c5194c9691 \
    && git checkout FETCH_HEAD \
    && mkdir build && cd build \
    && cmake .. -DBUILD_SHARED_LIBS=OFF -DWHISPER_BUILD_EXAMPLES=OFF -DWHISPER_BUILD_TESTS=OFF \
    && make -j$(nproc) \
    && mkdir -p /usr/local/lib/whisper /usr/local/include/whisper \
    && cp src/libwhisper.a ggml/src/libggml*.a /usr/local/lib/whisper/ \
    && cp /tmp/whisper.cpp/include/whisper.h /tmp/whisper.cpp/ggml/include/ggml*.h /usr/local/include/whisper/ \
    && rm -rf /tmp/whisper.cpp

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV C_INCLUDE_PATH=/usr/local/include/whisper
ENV LIBRARY_PATH=/usr/local/lib/whisper

RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /app/bin/server ./cmd/api
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /app/bin/scheduler ./cmd/scheduler

# Runtime stage
FROM alpine:3.20

RUN addgroup -S app && adduser -S app -G app && apk add --no-cache ca-certificates curl ffmpeg libstdc++ libgomp

WORKDIR /app

# Download whisper model and verify integrity
RUN mkdir -p /app/models \
    && curl -fSL -o /app/models/ggml-base.bin \
       https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin \
    && MODEL_SIZE=$(stat -c%s /app/models/ggml-base.bin) \
    && if [ "$MODEL_SIZE" -lt 100000000 ]; then echo "Model download too small: ${MODEL_SIZE} bytes" && exit 1; fi \
    && echo "Whisper model downloaded: ${MODEL_SIZE} bytes"

COPY --from=builder /app/bin/server /app/server
COPY --from=builder /app/bin/scheduler /app/scheduler
COPY --from=builder /app/migrations /app/migrations
COPY --from=builder /app/AGENTS.md /app/AGENTS.md
COPY --from=builder /app/agents /app/agents
COPY start.sh /app/start.sh

RUN chmod +x /app/start.sh
COPY healthcheck.sh /app/healthcheck.sh

RUN chmod +x /app/healthcheck.sh

ENV HTTP_ADDR=:8080
ENV SERVICE_ROLE=api
ENV AGENT_WORKSPACE_ROOT=/app
EXPOSE 8080

USER app

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD ["/app/healthcheck.sh"]

ENTRYPOINT ["/app/start.sh"]
