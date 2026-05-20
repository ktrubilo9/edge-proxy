FROM golang:1.25-alpine AS build
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o admin-api ./cmd/admin-api

FROM alpine:3.20
WORKDIR /app
COPY --from=build /app/admin-api .
EXPOSE 8081
CMD ["./admin-api"]