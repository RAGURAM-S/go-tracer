FROM golang:1.23.2-alpine AS builder

# Install git and other necessary build tools
RUN apk add --no-cache git build-base

WORKDIR /app
COPY go.mod go.sum main.go ./

# Download dependencies and generate go.sum
RUN go mod download
RUN go mod tidy

# Build the application
RUN go build -o main .

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/main .
EXPOSE 9999
CMD ["./main"]