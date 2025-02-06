package main

import (
	"log"

	"github.com/watchlist-kata/media/api/server" // Импортируем сервер
	"github.com/watchlist-kata/media/internal/config"
	"github.com/watchlist-kata/media/internal/repository"
	"github.com/watchlist-kata/media/internal/service"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Could not load config: %v", err)
	}

	repo, err := repository.NewRepository(cfg)
	if err != nil {
		log.Fatalf("Could not create repository: %v", err)
	}

	svc := service.NewService(repo)

	// Запускаем gRPC сервер на порту 50051 (или любой другой)
	server.StartGRPCServer(":50051", svc)
}
