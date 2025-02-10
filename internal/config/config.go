package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config содержит параметры конфигурации приложения
type Config struct {
	TMDBAPIKey    string
	DBHost        string
	DBPort        string
	DBUser        string
	DBPassword    string
	DBName        string
	DBSSLMode     string
	KafkaBrokers  []string
	KafkaTopic    string
	GRPCPort      string
	ServiceName   string
	LogBufferSize int
}

// LoadConfig загружает конфигурацию из .env файла
func LoadConfig() (*Config, error) {
	// Загружаем переменные окружения из .env файла
	err := godotenv.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load .env file: %w", err)
	}

	// Преобразуем KAFKA_BROKERS в []string
	kafkaBrokers := strings.Split(os.Getenv("KAFKA_BROKERS"), ",")

	// Преобразуем LOG_BUFFER_SIZE в int
	logBufferSizeStr := os.Getenv("LOG_BUFFER_SIZE")
	logBufferSize, err := strconv.Atoi(logBufferSizeStr)
	if err != nil {
		return nil, fmt.Errorf("failed to convert LOG_BUFFER_SIZE to int: %w", err)
	}

	return &Config{
		TMDBAPIKey:    os.Getenv("TMDB_API_KEY"),
		DBHost:        os.Getenv("DB_HOST"),
		DBPort:        os.Getenv("DB_PORT"),
		DBUser:        os.Getenv("DB_USER"),
		DBPassword:    os.Getenv("DB_PASSWORD"),
		DBName:        os.Getenv("DB_NAME"),
		DBSSLMode:     os.Getenv("DB_SSLMODE"),
		KafkaBrokers:  kafkaBrokers,
		KafkaTopic:    os.Getenv("KAFKA_TOPIC"),
		GRPCPort:      os.Getenv("GRPC_PORT"),
		ServiceName:   os.Getenv("SERVICE_NAME"),
		LogBufferSize: logBufferSize,
	}, nil
}
