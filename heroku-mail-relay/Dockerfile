# -------- Build stage --------
FROM golang:1.26.1-alpine AS build_base

# Install required packages
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Cache dependencies
COPY go.mod ./
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

# Set working directory
WORKDIR /root/

# Copy binary from build stage
COPY --from=build_base /app/app .

# Document the port the app listens on
EXPOSE 8080

# Run the application
CMD ["./app"]
