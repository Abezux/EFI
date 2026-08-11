# Multi-stage build for Go processor service
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install build essentials
RUN apk add --no-cache ca-certificates git

# Cache dependencies
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build statically linked binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/processor .

# Final minimal runtime image
FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S appgroup && adduser -S appuser -G appgroup

COPY --from=builder /app/processor /app/processor

USER appuser

ENTRYPOINT ["/app/processor"]
