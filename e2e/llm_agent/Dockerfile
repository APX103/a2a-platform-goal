FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
ENV GOTOOLCHAIN=auto
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /llm_agent ./e2e/llm_agent

FROM alpine:latest
COPY --from=builder /llm_agent /usr/local/bin/llm_agent
EXPOSE 10003
ENTRYPOINT ["llm_agent"]
CMD ["-port", "10003"]
