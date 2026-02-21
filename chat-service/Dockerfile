FROM golang:1.25-alpine AS builder

WORKDIR /workspace

# Copy only the modules this service needs
COPY chat-service/go.mod chat-service/go.sum ./chat-service/
COPY protos/go.mod ./protos/

WORKDIR /workspace/chat-service
RUN GOWORK=off go mod download

WORKDIR /workspace
COPY chat-service/ ./chat-service/
COPY protos/ ./protos/

WORKDIR /workspace/chat-service
RUN GOWORK=off go build -o /bin/chat-service ./cmd/chat/main.go

# run stage
FROM alpine:3.19

WORKDIR /app

RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /bin/chat-service /app/chat-service

EXPOSE 8081 50052

CMD ["/app/chat-service"]
