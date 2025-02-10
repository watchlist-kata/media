package main

import (
	"fmt"
	"log"

	"github.com/watchlist-kata/media/api/server"
	"github.com/watchlist-kata/media/internal/config"
	"github.com/watchlist-kata/media/internal/repository/postgres"
	"github.com/watchlist-kata/media/internal/service"
	"github.com/watchlist-kata/media/pkg/logger"
	pg "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", fmt.Errorf("failed to load configuration: %w", err))
	}

	// Инициализируем логгер
	customLogger, err := logger.NewLogger(
		cfg.KafkaBrokers,
		cfg.KafkaTopic,
		cfg.ServiceName,
		cfg.LogBufferSize,
	)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", fmt.Errorf("failed to initialize logger: %w", err))
	}
	defer func() {
		if multiHandler, ok := customLogger.Handler().(*logger.MultiHandler); ok {
			multiHandler.CloseAll()
		} else {
			log.Println("Failed to close all logger handlers")
		}
	}()

	// Подключаемся к базе данных
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=%s password=%s",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBName, cfg.DBSSLMode, cfg.DBPassword)
	db, err := gorm.Open(pg.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", fmt.Errorf("failed to connect to database: %w", err))
	}

	// Инициализируем репозиторий
	repo := postgres.NewPostgresRepository(db, customLogger)

	// Инициализируем сервис
	svc := service.NewMediaService(repo, customLogger)

	// Запускаем gRPC-сервер
	server.StartGRPCServer(cfg.GRPCPort, svc, customLogger)
}
