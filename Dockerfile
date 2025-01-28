# Build stage
FROM golang:1.21-alpine AS builder

# Install necessary build tools
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Use ARG to get environment variables during build
ARG GIT

# Add build argument to force rebuild to avoid cache for the git clone
ARG BUILD_DATE

# Clone the repository using GIT variable
RUN git clone ${GIT:-https://github.com/danilofalcao/cursor-deepseek.git} . && \
    rm -rf .git

# Download dependencies
RUN go mod download

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o proxy proxy.go

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Set working directory
WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/proxy .

# Copy .env file if needed
COPY .env .

# Expose port 9000
EXPOSE 9000

# Use ARG and ENV to pass MODEL to runtime
ARG MODEL
ENV MODEL=${MODEL:-coder}

# Run the application with the MODEL environment variable
CMD ["sh", "-c", "./proxy -model ${MODEL}"]
