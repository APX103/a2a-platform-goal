FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Build test binary
RUN CGO_ENABLED=0 go test -c -o /e2e_test ./e2e/

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /e2e_test /usr/local/bin/e2e_test
WORKDIR /test
ENTRYPOINT ["e2e_test"]
