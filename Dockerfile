# ==== Сборка ====
FROM golang:1.25.9 AS builder
WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o server .

# ==== Запуск ====
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /app/server /server
EXPOSE 8080
CMD ["/server"]