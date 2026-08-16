FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder
ARG TARGETOS=linux
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 app
WORKDIR /app
COPY --from=builder /out/server /app/server
COPY internal/migrations /app/migrations
USER app
EXPOSE 8117
ENV HTTP_ADDR=:8117 \
    STORAGE_DRIVER=postgres
ENTRYPOINT ["/app/server"]
