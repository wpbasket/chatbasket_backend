# -------- Build stage --------
FROM golang:1.26.4-alpine AS build_base

# Install required packages
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the Go binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o app ./app


# -------- Runtime stage --------
FROM alpine:latest

# Set Go memory limit for 512MB RAM dyno
ENV GOMEMLIMIT=450MiB

# Install CA certificates for HTTPS
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

# Set working directory
WORKDIR /home/appuser

# Copy binary from build stage
COPY --from=build_base --chown=appuser:appuser /app/app .

# Switch to non-root user
USER appuser

# Document the port the app listens on
EXPOSE 8080

# Run the application
CMD ["./app"]
