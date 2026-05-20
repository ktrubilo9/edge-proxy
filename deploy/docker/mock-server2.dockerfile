FROM golang:1.25-alpine AS build
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o mock-backend2 ./cmd/mock-backend

FROM alpine:3.20
WORKDIR /app

COPY --from=build /app/mock-backend2 .

EXPOSE 3000
CMD ["./mock-backend2"]