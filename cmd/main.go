package main

import (
	"fmt"
	"log"

	"github.com/watchlist-kata/media/api/server"
	"github.com/watchlist-kata/media/internal/config"
	"github.com/watchlist-kata/media/internal/repository"
	"github.com/watchlist-kata/media/internal/service"
	"github.com/watchlist-kata/media/pkg/logger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Инициализируем логгер
	customLogger, err := logger.NewLogger(
		cfg.KafkaBrokers,
		cfg.KafkaTopic,
		cfg.ServiceName,
		cfg.LogBufferSize,
	)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
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
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Инициализируем репозиторий
	repo := repository.NewRepository(db, customLogger)

	// Инициализируем сервис
	svc := service.NewService(repo, customLogger)

	// Запускаем gRPC-сервер
	server.StartGRPCServer(cfg.GRPCPort, svc)
}
