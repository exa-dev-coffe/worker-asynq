# Builder stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev

WORKDIR /app

# Copy dependency files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build production binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o worker main.go

# Final runner stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy the pre-built binary
COPY --from=builder /app/worker .
COPY --from=builder /app/.env.example .env.example

# Expose port for monitoring dashboard
EXPOSE 8085

# Command to run
CMD ["./worker"]
