FROM golang:1.25-alpine AS build
WORKDIR /app

COPY go.mod go.sum ./configs/config.json ./
RUN go mod download

COPY . .

RUN go build -o reverse-proxy ./cmd/reverse-proxy

FROM alpine:3.20
WORKDIR /app

COPY --from=build /app/reverse-proxy .
COPY --from=build /app/config.json .

EXPOSE 8080 50051 9091
CMD ["./reverse-proxy"]
