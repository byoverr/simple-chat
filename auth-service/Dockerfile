FROM golang:1.25-alpine AS builder

WORKDIR /workspace

# Copy only the modules this service needs
COPY auth-service/go.mod auth-service/go.sum ./auth-service/
COPY protos/go.mod ./protos/

WORKDIR /workspace/auth-service
RUN GOWORK=off go mod download

WORKDIR /workspace
COPY auth-service/ ./auth-service/
COPY protos/ ./protos/

WORKDIR /workspace/auth-service
RUN GOWORK=off go build -o /bin/auth-service ./cmd/auth-http/main.go

# run stage
FROM alpine:3.19

WORKDIR /app

RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /bin/auth-service /app/auth-service

EXPOSE 8080 50051

CMD ["/app/auth-service"]
