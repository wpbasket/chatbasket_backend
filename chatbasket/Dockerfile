# FROM golang:1.24.4-alpine AS build_base
# RUN apk add --no-cache git
# WORKDIR /app

# COPY . .
# RUN go mod download

# RUN GOOS=linux CGO_ENABLED=0 go build -o bootstrap ./app
# FROM alpine:3.22.0
# RUN apk add ca-certificates

# # To use the AWS Lambda Web Adapter in the docker image
# COPY --from=public.ecr.aws/awsguru/aws-lambda-adapter:0.9.1 /lambda-adapter /opt/extensions/lambda-adapter
# COPY --from=build_base /app/bootstrap /app/bootstrap

# ENV PORT=8000 AWS_LWA_ASYNC_INIT=true
# EXPOSE 8000

# CMD ["/app/bootstrap"]




# FROM golang:1.24.4-alpine AS build_base

# # Install git and ca-certificates
# RUN apk add --no-cache git ca-certificates

# # Set working directory
# WORKDIR /app

# # Copy go mod files for dependency caching
# COPY go.mod go.sum ./

# # Download dependencies
# RUN go mod download

# # Copy source code
# COPY . .

# # Build the application from app directory
# RUN go build -o main ./app

# # Final runtime stage
# FROM alpine:latest

# # Install ca-certificates for HTTPS
# RUN apk --no-cache add ca-certificates

# # Set working directory
# WORKDIR /root/

# # Copy binary from build stage
# COPY --from=build_base /app/main .

# # Heroku dynamic port
# EXPOSE 8080

# # Run the application
# CMD ["./main"]



# Build stage
FROM golang:1.24.4-alpine AS build_base

# Install dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy all source code
COPY . .

# Build the Go app (adjust if your main.go path is different)
RUN go build -o main ./app

# Runtime stage
FROM alpine:latest

# Install only certs
RUN apk --no-cache add ca-certificates

# Set working directory
WORKDIR /root/

# Copy the built binary
COPY --from=build_base /app/main .

# Set the PORT env (Heroku injects this automatically at runtime)
ENV PORT=8080

# Run the app (it should bind to PORT from env)
CMD ["./main"]
