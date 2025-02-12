# Build stage
FROM golang:1.22.7-alpine AS builder

# Установка необходимых зависимостей
RUN apk add --no-cache git

# Установка рабочей директории
WORKDIR /app

# Копирование файлов go.mod и go.sum
COPY go.mod go.sum ./

# Загрузка зависимостей
RUN go mod download

# Копирование исходного кода
COPY . .

# Сборка приложения
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/media ./cmd/main.go

# Final stage
FROM alpine:3.19

# Установка необходимых зависимостей для работы приложения
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Копирование бинарного файла из build stage
COPY --from=builder /app/media .

# Копирование .env файла
COPY ./cmd/.env .

# Создание директории для логов
RUN mkdir -p /app/logs/media

# Экспорт порта
EXPOSE 50051

# Запуск приложения
CMD ["./media"]
