FROM golang:1.25-alpine

# Set working directory
WORKDIR /app

# Install necessary tools
RUN apk add --no-cache git make build-base sqlite-dev

# Install air for hot reloading
RUN go install github.com/air-verse/air@latest

# Copy go.mod and go.sum files (if they exist)
# Since we will initialize the module later, we'll mount the working directory
COPY . .

# Run air for development
CMD ["air"]
