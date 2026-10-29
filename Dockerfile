# ==== Сборка ====
FROM golang:1.27.3 AS builder
WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o server .

# ==== Запуск ====
FROM gcr.io/distroless/static-debian13:nonroot
LABEL org.opencontainers.image.source=https://github.com/nemelix/go-service-platform
COPY --from=builder /app/server /server
EXPOSE 8080
CMD ["/server"]