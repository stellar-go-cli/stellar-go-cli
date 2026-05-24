# Multi-service Dockerfile for building all MozartPay components
FROM golang:1.24-alpine AS base-builder

WORKDIR /app
RUN apk add --no-cache git ca-certificates tzdata
COPY go.mod go.sum ./
RUN go mod download

# WebAuthn Server Builder
FROM base-builder AS webauthn-builder
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o webauthn-server ./cmd/webauthn-server

# MCP Server Builder  
FROM base-builder AS mcp-builder
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o mozartpay ./cmd/mozartpay

# CLI Builder
FROM base-builder AS cli-builder
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o mozartpay-cli ./cmd/mozartpay

# WebAuthn Server Final Image
FROM alpine:latest AS webauthn
RUN apk --no-cache add ca-certificates tzdata wget
WORKDIR /root/
COPY --from=webauthn-builder /app/webauthn-server .
EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8000/health || exit 1
CMD ["./webauthn-server"]

# MCP Server Final Image
FROM alpine:latest AS mcp
RUN apk --no-cache add ca-certificates tzdata wget
WORKDIR /root/
COPY --from=mcp-builder /app/mozartpay .
RUN mkdir -p /root/.mozartpay
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:3000/health || exit 1
CMD ["./mozartpay", "mcp", "--transport", "sse", "--port", "3000"]

# CLI Final Image
FROM alpine:latest AS cli
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/
COPY --from=cli-builder /app/mozartpay-cli .
RUN mkdir -p /root/.mozartpay
CMD ["./mozartpay-cli"]
