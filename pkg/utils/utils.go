package utils

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/watchlist-kata/media/internal/config"
	"github.com/watchlist-kata/media/pkg/logger"
	"google.golang.org/grpc"
	pg "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// NewCustomLogger initializes a new custom logger.
func NewCustomLogger(cfg *config.Config) (*slog.Logger, error) {
	customLogger, err := logger.NewLogger(
		cfg.KafkaBrokers,
		cfg.KafkaTopic,
		cfg.ServiceName,
		cfg.LogBufferSize,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}
	return customLogger, nil
}

// NewDatabaseConnection connects to the database.
func NewDatabaseConnection(cfg *config.Config) (*gorm.DB, Closer, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=%s password=%s",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBName, cfg.DBSSLMode, cfg.DBPassword)
	db, err := gorm.Open(pg.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	return db, sqlDB, nil
}

// CloseLogger safely closes the logger handlers.
func CloseLogger(customLogger *slog.Logger) {
	multiHandler := customLogger.Handler()
	if multiHandler != nil {
		if closer, ok := multiHandler.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				log.Println("Failed to close logger handler:", err)
			}
		} else {
			log.Println("Failed to close all logger handlers")
		}
	}
}

// CloseDatabaseConnection safely closes the database connection.
func CloseDatabaseConnection(sqlDB Closer, customLogger *slog.Logger) {
	customLogger.Info("Closing database connection")
	if err := sqlDB.Close(); err != nil {
		customLogger.Error("Failed to close database connection", "error", err)
	}
	customLogger.Info("Database connection closed")
}

// Closer is an interface for closing resources.
type Closer interface {
	Close() error
}

// GracefulShutdown performs a graceful shutdown of the gRPC server and database connection.
func GracefulShutdown(ctx context.Context, grpcServer *grpc.Server, sqlDB Closer, customLogger *slog.Logger, wg *sync.WaitGroup) {
	// Create a channel to signal shutdown completion
	shutdownDone := make(chan bool, 1)

	// Launch a goroutine to perform the shutdown tasks
	wg.Add(1)
	go func() {
		defer wg.Done()

		// Stop the gRPC server gracefully
		customLogger.Info("Stopping gRPC server gracefully")
		grpcServer.GracefulStop()
		customLogger.Info("gRPC server stopped gracefully")

		// Close the database connection
		CloseDatabaseConnection(sqlDB, customLogger)

		customLogger.Info("Shutdown complete")
		shutdownDone <- true
	}()

	// Launch a timer for graceful shutdown
	timeout := time.After(10 * time.Second)

	// Wait for shutdown completion or timeout
	select {
	case <-shutdownDone:
		customLogger.Info("Graceful shutdown completed")
	case <-timeout:
		customLogger.Warn("Graceful shutdown timed out", "timeout", 10*time.Second)
		customLogger.Warn("Forcing shutdown")
		os.Exit(1)
	case <-ctx.Done(): // Add a case to handle context cancellation
		customLogger.Info("Context canceled, graceful shutdown aborted")
	}
}
