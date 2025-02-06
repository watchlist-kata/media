package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config содержит параметры конфигурации приложения
type Config struct {
	TMDBAPIKey string
	DBURL      string
	DBUser     string
	DBPassword string
}

// LoadConfig загружает конфигурацию из .env файла
func LoadConfig() (*Config, error) {
	// Загружаем переменные окружения из .env файла
	err := godotenv.Load()
	if err != nil {
		return nil, err
	}

	// Собираем строку подключения к базе данных
	dbURL := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
	)

	return &Config{
		TMDBAPIKey: os.Getenv("TMDB_API_KEY"),
		DBURL:      dbURL,
	}, nil
}
