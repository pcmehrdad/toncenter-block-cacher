FROM golang:1.22.5-alpine AS builder

# Install required system packages and update certificates
RUN apk update && \
    apk upgrade && \
    apk add --no-cache ca-certificates && \
    update-ca-certificates

# Add Maintainer Info
LABEL maintainer="Your Name <your.email@example.com>"

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy the entire project
COPY . .

# Download all dependencies
RUN go mod download

# Build the Go app
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o toncenter-block-cacher ./cmd/processor

# Create data directory in builder stage
RUN mkdir -p ./data/blocks

# Start a new stage from scratch
FROM scratch

WORKDIR /app

# Copy everything needed from builder
COPY --from=builder /app/toncenter-block-cacher .
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/data /app/data
COPY .env.example .env

# Expose port 8080
EXPOSE 8080

# Command to run the executable
CMD ["./toncenter-block-cacher"]