package server

import (
	"context"
	"log"
	"net"

	"github.com/watchlist-kata/media/api" // Импортируйте ваши модели из api/media.proto
	"github.com/watchlist-kata/media/internal/service"
	"google.golang.org/grpc"
)

// MediaServer представляет собой структуру для реализации gRPC сервиса
type MediaServer struct {
	api.UnimplementedMediaServiceServer                  // Встраиваем автоматически сгенерированный интерфейс
	svc                                 *service.Service // Ссылка на сервис
}

// NewMediaServer создает новый экземпляр MediaServer
func NewMediaServer(svc *service.Service) *MediaServer {
	return &MediaServer{svc: svc}
}

// SaveMedia реализует метод SaveMedia из интерфейса MediaServiceServer
func (s *MediaServer) SaveMedia(ctx context.Context, req *api.SaveMediaRequest) (*api.Media, error) {
	// Проверяем, не был ли отменен контекст
	select {
	case <-ctx.Done():
		log.Println("Context cancelled")
		return nil, ctx.Err() // Возвращаем ошибку отмены
	default:
		// Продолжаем выполнение, если контекст не был отменен
	}

	media, err := s.svc.SaveMedia(req) // Передаём req
	if err != nil {
		return nil, err
	}
	return media, nil
}

// GetMediaByID реализует метод GetMediaByID из интерфейса MediaServiceServer
func (s *MediaServer) GetMediaByID(ctx context.Context, req *api.GetMediaRequest) (*api.Media, error) {
	// Проверяем, не был ли отменен контекст
	select {
	case <-ctx.Done():
		log.Println("Context cancelled")
		return nil, ctx.Err() // Возвращаем ошибку отмены
	default:
		// Продолжаем выполнение, если контекст не был отменен
	}

	media, err := s.svc.GetMediaByID(req)
	if err != nil {
		return nil, err
	}
	return media, nil
}

// GetMediasByName реализует метод GetMediasByName из интерфейса MediaServiceServer
func (s *MediaServer) GetMediasByName(ctx context.Context, req *api.GetMediaRequest) (*api.MediaList, error) {
	// Проверяем, не был ли отменен контекст
	select {
	case <-ctx.Done():
		log.Println("Context cancelled")
		return nil, ctx.Err() // Возвращаем ошибку отмены
	default:
		// Продолжаем выполнение, если контекст не был отменен
	}

	mediaList, err := s.svc.GetMediasByName(req)
	if err != nil {
		return nil, err
	}
	return mediaList, nil
}

// UpdateMedia реализует метод UpdateMedia из интерфейса MediaServiceServer
func (s *MediaServer) UpdateMedia(ctx context.Context, req *api.SaveMediaRequest) (*api.Media, error) {
	// Проверяем, не был ли отменен контекст
	select {
	case <-ctx.Done():
		log.Println("Context cancelled")
		return nil, ctx.Err() // Возвращаем ошибку отмены
	default:
		// Продолжаем выполнение, если контекст не был отменен
	}

	media, err := s.svc.UpdateMedia(req.Media)
	if err != nil {
		return nil, err
	}
	return media, nil
}

// StartGRPCServer - запуск gRPC сервера
func StartGRPCServer(port string, svc *service.Service) {
	listener, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	api.RegisterMediaServiceServer(grpcServer, NewMediaServer(svc))

	log.Printf("gRPC server listening on %s", port)
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
