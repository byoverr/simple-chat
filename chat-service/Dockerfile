FROM golang:1.25-alpine AS builder

WORKDIR /workspace

COPY go.work go.work.sum ./

COPY chat-service/go.mod chat-service/go.sum ./chat-service/
COPY protos/go.mod ./protos/

COPY chat-service/ ./chat-service/
COPY protos/ ./protos/

WORKDIR /workspace/auth-service
RUN go build -o /bin/chat-service cmd/auth/main.go

# run stage
FROM alpine:3.19

WORKDIR /app

RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /bin/chat-service /app/chat-service

EXPOSE 50051

CMD ["/app/auth-service"]
