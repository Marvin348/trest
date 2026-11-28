FROM golang:1.27.2-alpine AS builder

WORKDIR /app

COPY packages/secretcrypto ./packages/secretcrypto
COPY contracts ./contracts
COPY services/deployment-service ./services/deployment-service

WORKDIR /app/services/deployment-service

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/grpc ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/publisher ./cmd/publisher
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/worker ./cmd/worker

# Stage 2: Runtime
FROM alpine:3.22

WORKDIR /app

COPY --from=builder /app/bin/ /app/bin/

EXPOSE 8090