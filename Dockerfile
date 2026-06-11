FROM golang:1.21-alpine AS builder

WORKDIR /app

RUN apk add --no-cache gcc musl-dev sqlite-dev
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o wa-bot .

FROM alpine:latest

WORKDIR /root/

RUN apk --no-cache add ca-certificates sqlite
COPY --from=builder /app/wa-bot .
RUN mkdir -p .device

EXPOSE 8080

CMD ["./wa-bot", "-device", ".device", "-db", "wa-bot.db"]
