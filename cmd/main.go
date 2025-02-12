package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/watchlist-kata/media/api/server"
	"github.com/watchlist-kata/media/internal/config"
	"github.com/watchlist-kata/media/internal/repository"
	"github.com/watchlist-kata/media/internal/service"
	"github.com/watchlist-kata/media/pkg/logger"
	"google.golang.org/grpc"
	pg "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	customLogger, err := logger.NewLogger(
		cfg.KafkaBrokers,
		cfg.KafkaTopic,
		cfg.ServiceName,
		cfg.LogBufferSize,
	)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	// Закрываем kafka producer
	defer func() {
		multiHandler := customLogger.Handler()
		if multiHandler != nil {
			if closer, ok := multiHandler.(interface{ Close() error }); ok {
				closer.Close()
			} else {
				log.Println("Failed to close all logger handlers")
			}
		}
	}()

	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=%s password=%s",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBName, cfg.DBSSLMode, cfg.DBPassword)
	db, err := gorm.Open(pg.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Получаем SQL.DB для закрытия соединения при завершении
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("Failed to get database instance: %v", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			log.Printf("Failed to close database connection: %v", err)
		}
	}()

	// Создаем репозиторий и сервис
	repo := repository.NewPostgresRepository(db, customLogger)
	svc, err := service.NewMediaService(repo, customLogger, cfg)
	if err != nil {
		log.Fatalf("Failed to create media service: %v", err)
	}

	// Создаем контекст с отменой
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Создаем WaitGroup
	var wg sync.WaitGroup

	// Канал для ошибок сервера
	errChan := make(chan error, 1)
	defer close(errChan)

	// Создаем gRPC-сервер
	grpcServer := grpc.NewServer()

	// Запускаем gRPC-сервер в отдельной горутине
	wg.Add(1)
	go func() {
		defer wg.Done()
		customLogger.Info("Starting gRPC server", "port", cfg.GRPCPort)
		server.StartGRPCServer(cfg.GRPCPort, svc, customLogger, grpcServer)
	}()

	// Обрабатываем сигналы завершения
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Ожидаем сигнал или ошибку
	select {
	case sig := <-sigChan:
		customLogger.Info("Received shutdown signal", "signal", sig)
	case err := <-errChan:
		customLogger.Error("Server error", "error", err)
	}

	// Создаем канал для завершения graceful shutdown
	shutdownDone := make(chan bool, 1)

	// Запускаем горутину для graceful shutdown
	wg.Add(1)
	go func() {
		defer wg.Done()

		// Останавливаем gRPC-сервер gracefully
		customLogger.Info("Stopping gRPC server gracefully")
		grpcServer.GracefulStop()
		customLogger.Info("gRPC server stopped gracefully")

		// Закрываем соединение с базой данных
		customLogger.Info("Closing database connection")
		if err := sqlDB.Close(); err != nil {
			customLogger.Error("Failed to close database connection", "error", err)
		}
		customLogger.Info("Database connection closed")

		customLogger.Info("Shutdown complete")
		shutdownDone <- true
	}()

	// Создаем контекст с таймаутом для graceful shutdown
	shutdownTimeout := 10 * time.Second
	timeout := time.After(shutdownTimeout)

	// Ожидаем завершения graceful shutdown или таймаута
	select {
	case <-shutdownDone:
		customLogger.Info("Graceful shutdown completed")
	case <-timeout:
		customLogger.Warn("Graceful shutdown timed out", "timeout", shutdownTimeout)
		customLogger.Warn("Forcing shutdown")
		os.Exit(1)
	}

	// Ожидаем завершения всех горутин
	wg.Wait()

	customLogger.Info("Server exited properly")
}
