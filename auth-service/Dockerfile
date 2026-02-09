FROM golang:1.25-alpine AS builder

WORKDIR /workspace

COPY go.work go.work.sum ./

COPY auth-service/go.mod auth-service/go.sum ./auth-service/
COPY protos/go.mod ./protos/

COPY auth-service/ ./auth-service/
COPY protos/ ./protos/

WORKDIR /workspace/auth-service
RUN go build -o /bin/auth-service cmd/auth/main.go

# run stage
FROM alpine:3.19

WORKDIR /app

RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /bin/auth-service /app/auth-service

EXPOSE 50051

CMD ["/app/auth-service"]
