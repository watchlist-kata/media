package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/watchlist-kata/media/api/server"
	"github.com/watchlist-kata/media/internal/config"
	"github.com/watchlist-kata/media/internal/repository"
	"github.com/watchlist-kata/media/internal/service"
	"github.com/watchlist-kata/media/pkg/utils"
	"google.golang.org/grpc"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize logger
	customLogger, err := utils.NewCustomLogger(cfg) // Use the new utility function
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer utils.CloseLogger(customLogger)

	// Connect to database
	db, sqlDB, err := utils.NewDatabaseConnection(cfg) // Use the new utility function
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer utils.CloseDatabaseConnection(sqlDB, customLogger)

	// Create repository and service
	repo := repository.NewPostgresRepository(db, customLogger)
	svc, err := service.NewMediaService(repo, customLogger, cfg)
	if err != nil {
		log.Fatalf("Failed to create media service: %v", err)
	}

	// Create context with cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create WaitGroup
	var wg sync.WaitGroup

	// Channel for server errors
	errChan := make(chan error, 1)
	defer close(errChan)

	// Create gRPC server
	grpcServer := grpc.NewServer()

	// Start gRPC server in a separate goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		customLogger.Info("Starting gRPC server", "port", cfg.GRPCPort)
		err := server.StartGRPCServer(cfg.GRPCPort, svc, customLogger, grpcServer)
		if err != nil {
			errChan <- fmt.Errorf("gRPC server failed: %w", err)
		}
	}()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for a signal or an error
	select {
	case sig := <-sigChan:
		customLogger.Info("Received shutdown signal", "signal", sig)
	case err := <-errChan:
		customLogger.Error("Server error", "error", err)
		cancel() // Cancel the context to signal shutdown
	case <-ctx.Done(): // Add a case to handle context cancellation
		customLogger.Info("Context canceled, initiating shutdown")
	}

	// Perform graceful shutdown
	utils.GracefulShutdown(ctx, grpcServer, sqlDB, customLogger, &wg)

	// Wait for all goroutines to complete
	wg.Wait()

	customLogger.Info("Server exited properly")
}
