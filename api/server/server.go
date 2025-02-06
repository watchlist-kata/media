package server

import (
	"context"
	"log"
	"net"

	"github.com/watchlist-kata/media/api"
	"github.com/watchlist-kata/media/internal/service"
	"google.golang.org/grpc"
	"log/slog"
)

// MediaServer представляет собой структуру для реализации gRPC сервиса
type MediaServer struct {
	api.UnimplementedMediaServiceServer
	svc    *service.Service
	Logger *slog.Logger
}

// NewMediaServer создает новый экземпляр MediaServer
func NewMediaServer(svc *service.Service, logger *slog.Logger) *MediaServer {
	return &MediaServer{
		svc:    svc,
		Logger: logger,
	}
}

// SaveMedia реализует метод SaveMedia из интерфейса MediaServiceServer
func (s *MediaServer) SaveMedia(ctx context.Context, req *api.SaveMediaRequest) (*api.Media, error) {
	// Проверяем, не был ли отменен контекст
	select {
	case <-ctx.Done():
		s.Logger.WarnContext(ctx, "Context cancelled")
		return nil, ctx.Err() // Возвращаем ошибку отмены
	default:
		// Продолжаем выполнение, если контекст не был отменен
	}

	s.Logger.InfoContext(ctx, "SaveMedia called", "tmdbID", req.Media.TmdbId)
	media, err := s.svc.SaveMedia(req) // Передаём req
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to SaveMedia", "tmdbID", req.Media.TmdbId, "error", err)
		return nil, err
	}
	return media, nil
}

// GetMediaByID реализует метод GetMediaByID из интерфейса MediaServiceServer
func (s *MediaServer) GetMediaByID(ctx context.Context, req *api.GetMediaRequest) (*api.Media, error) {
	// Проверяем, не был ли отменен контекст
	select {
	case <-ctx.Done():
		s.Logger.WarnContext(ctx, "Context cancelled")
		return nil, ctx.Err() // Возвращаем ошибку отмены
	default:
		// Продолжаем выполнение, если контекст не был отменен
	}

	s.Logger.InfoContext(ctx, "GetMediaByID called", "mediaID", req.Id)
	media, err := s.svc.GetMediaByID(req)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to GetMediaByID", "mediaID", req.Id, "error", err)
		return nil, err
	}
	return media, nil
}

// GetMediasByName реализует метод GetMediasByName из интерфейса MediaServiceServer
func (s *MediaServer) GetMediasByName(ctx context.Context, req *api.GetMediaRequest) (*api.MediaList, error) {
	// Проверяем, не был ли отменен контекст
	select {
	case <-ctx.Done():
		s.Logger.WarnContext(ctx, "Context cancelled")
		return nil, ctx.Err() // Возвращаем ошибку отмены
	default:
		// Продолжаем выполнение, если контекст не был отменен
	}

	s.Logger.InfoContext(ctx, "GetMediasByName called", "name", req.Name)
	mediaList, err := s.svc.GetMediasByName(req)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to GetMediasByName", "name", req.Name, "error", err)
		return nil, err
	}
	return mediaList, nil
}

// UpdateMedia реализует метод UpdateMedia из интерфейса MediaServiceServer
func (s *MediaServer) UpdateMedia(ctx context.Context, req *api.SaveMediaRequest) (*api.Media, error) {
	// Проверяем, не был ли отменен контекст
	select {
	case <-ctx.Done():
		s.Logger.WarnContext(ctx, "Context cancelled")
		return nil, ctx.Err() // Возвращаем ошибку отмены
	default:
		// Продолжаем выполнение, если контекст не был отменен
	}
	s.Logger.InfoContext(ctx, "UpdateMedia called", "tmdbID", req.Media.TmdbId)
	media, err := s.svc.UpdateMedia(req.Media)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to UpdateMedia", "tmdbID", req.Media.TmdbId, "error", err)
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
	api.RegisterMediaServiceServer(grpcServer, NewMediaServer(svc, svc.Logger))

	log.Printf("gRPC server listening on %s", port)
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
