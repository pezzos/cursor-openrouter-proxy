# Build stage
FROM golang:1.21-alpine AS builder

# Install necessary build tools
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Clone the repository
RUN git clone https://github.com/danilofalcao/cursor-deepseek.git . && \
    rm -rf .git && \
    rm proxy-openrouter.go  # Supprime le fichier qui cause le conflit

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

# Run the application
CMD ["./proxy"]
