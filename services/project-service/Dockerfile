FROM golang:1.27.2-alpine AS builder

WORKDIR /app

COPY packages/secretcrypto ./packages/secretcrypto
COPY contracts ./contracts
COPY services/project-service ./services/project-service

WORKDIR /app/services/project-service

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/project-service ./cmd/api

# Stage 2: Runtime
FROM alpine:3.22

WORKDIR /app

COPY --from=builder /app/bin/project-service /app/project-service

EXPOSE 8080

CMD ["/app/project-service"]