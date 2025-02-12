package server

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/watchlist-kata/media/internal/service" // Импортируем интерфейс service
	"github.com/watchlist-kata/protos/media"           // Импортируем proto
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// MediaServer представляет собой структуру для реализации gRPC сервиса
type MediaServer struct {
	media.UnimplementedMediaServiceServer
	svc    service.Service // Используем интерфейс service.Service
	Logger *slog.Logger
}

// NewMediaServer создает новый экземпляр MediaServer
func NewMediaServer(svc service.Service, logger *slog.Logger) *MediaServer {
	return &MediaServer{
		svc:    svc,
		Logger: logger,
	}
}

// checkContextCancellation проверяет, был ли отменен контекст
func (s *MediaServer) checkContextCancellation(ctx context.Context, methodName string) error {
	select {
	case <-ctx.Done():
		s.Logger.WarnContext(ctx, methodName+" cancelled", "error", ctx.Err())
		return status.FromContextError(ctx.Err()).Err()
	default:
		return nil
	}
}

// GetRequestID извлекает ID запроса из контекста или генерирует новый, если его нет
func GetRequestID(ctx context.Context) string {
	if reqID, ok := ctx.Value("requestID").(string); ok && reqID != "" {
		return reqID
	}
	return uuid.New().String() // Генерируем новый, если его нет в контексте
}

// SaveMedia реализует метод SaveMedia из интерфейса MediaServiceServer
func (s *MediaServer) SaveMedia(ctx context.Context, req *media.SaveMediaRequest) (*media.Media, error) {
	if err := s.checkContextCancellation(ctx, "SaveMedia"); err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, "requestID", GetRequestID(ctx))
	requestID := GetRequestID(ctx)

	s.Logger.InfoContext(ctx, "SaveMedia called", "tmdbID", req.Media.TmdbId, "request_id", requestID)

	m, err := s.svc.SaveMedia(ctx, req)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to SaveMedia", "tmdbID", req.Media.TmdbId, "error", err, "request_id", requestID)
		return nil, status.Errorf(codes.Internal, "failed to save media with tmdbID %d: %v", req.Media.TmdbId, err)
	}
	return m, nil
}

// GetMediaByID реализует метод GetMediaByID из интерфейса MediaServiceServer
func (s *MediaServer) GetMediaByID(ctx context.Context, req *media.GetMediaByIDRequest) (*media.Media, error) {
	if err := s.checkContextCancellation(ctx, "GetMediaByID"); err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, "requestID", GetRequestID(ctx))
	requestID := GetRequestID(ctx)

	s.Logger.InfoContext(ctx, "GetMediaByID called", "mediaID", req.Id, "request_id", requestID)
	m, err := s.svc.GetMediaByID(ctx, req)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to GetMediaByID", "mediaID", req.Id, "error", err, "request_id", requestID)
		return nil, status.Errorf(codes.Internal, "failed to get media by ID %d: %v", req.Id, err)
	}
	return m, nil
}

// GetMediasByName реализует метод GetMediasByName из интерфейса MediaServiceServer
func (s *MediaServer) GetMediasByName(ctx context.Context, req *media.GetMediasByNameRequest) (*media.MediaList, error) {
	if err := s.checkContextCancellation(ctx, "GetMediasByName"); err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, "requestID", GetRequestID(ctx))
	requestID := GetRequestID(ctx)

	s.Logger.InfoContext(ctx, "GetMediasByName called", "name", req.Name, "request_id", requestID)
	mediaList, err := s.svc.GetMediasByName(ctx, req)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to GetMediasByName", "name", req.Name, "error", err, "request_id", requestID)
		return nil, status.Errorf(codes.Internal, "failed to get medias by name %s: %v", req.Name, err)
	}
	return mediaList, nil
}

// UpdateMedia реализует метод UpdateMedia из интерфейса MediaServiceServer
func (s *MediaServer) UpdateMedia(ctx context.Context, req *media.SaveMediaRequest) (*media.Media, error) {
	if err := s.checkContextCancellation(ctx, "UpdateMedia"); err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, "requestID", GetRequestID(ctx))
	requestID := GetRequestID(ctx)

	s.Logger.InfoContext(ctx, "UpdateMedia called", "tmdbID", req.Media.TmdbId, "request_id", requestID)
	m, err := s.svc.UpdateMedia(ctx, req.Media)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to UpdateMedia", "tmdbID", req.Media.TmdbId, "error", err, "request_id", requestID)
		// Проверяем, является ли ошибка нарушением уникальности
		if strings.Contains(err.Error(), "tmdb_id already exists") {
			return nil, status.Errorf(codes.AlreadyExists, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "failed to update media with tmdbID %d: %v", req.Media.TmdbId, err)
	}
	return m, nil
}

// SearchTMDB реализует метод SearchTMDB из интерфейса MediaServiceServer
func (s *MediaServer) SearchTMDB(ctx context.Context, req *media.SearchTMDBRequest) (*media.SearchTMDBResponse, error) {
	if err := s.checkContextCancellation(ctx, "SearchTMDB"); err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, "requestID", GetRequestID(ctx))
	requestID := GetRequestID(ctx)

	s.Logger.InfoContext(ctx, "SearchTMDB called", "name", req.Name, "request_id", requestID)

	medias, err := s.svc.SearchTMDB(ctx, req.Name)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to search TMDB", "name", req.Name, "error", err, "request_id", requestID)
		return nil, status.Errorf(codes.Internal, "failed to search TMDB with name %s: %v", req.Name, err)
	}
	return &media.SearchTMDBResponse{Medias: medias}, nil
}

// loggingInterceptor — gRPC интерсептор для логирования запросов
func loggingInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()

		// Генерация requestID, если его нет в контексте
		requestID := GetRequestID(ctx)
		ctx = context.WithValue(ctx, "requestID", requestID)

		// Извлекаем метаданные
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			logger.InfoContext(ctx, "Request Headers", "metadata", md, "request_id", requestID)
		}

		logger.InfoContext(ctx, "Request started", "method", info.FullMethod, "request_id", requestID, "start_time", start.Format(time.RFC3339))

		resp, err := handler(ctx, req)

		duration := time.Since(start)

		if err != nil {
			// Извлекаем stack trace
			stackTrace := debug.Stack()
			logger.ErrorContext(ctx, "Request failed", "method", info.FullMethod, "request_id", requestID, "duration", duration, "error", err, "stacktrace", string(stackTrace))
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				logger.WarnContext(ctx, "Request cancelled", "method", info.FullMethod, "request_id", requestID, "duration", duration, "error", err)
			} else {
				logger.ErrorContext(ctx, "Request failed", "method", info.FullMethod, "request_id", requestID, "duration", duration, "error", err, "stacktrace", string(stackTrace))
			}
		} else {
			logger.InfoContext(ctx, "Request finished", "method", info.FullMethod, "request_id", requestID, "duration", duration)
		}

		return resp, err
	}
}

// StartGRPCServer - запуск gRPC сервера
func StartGRPCServer(port string, svc service.Service, logger *slog.Logger, grpcServer *grpc.Server) {
	if grpcServer == nil {
		grpcServer = grpc.NewServer(
			grpc.UnaryInterceptor(loggingInterceptor(logger)),
		)
	}
	listener, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	media.RegisterMediaServiceServer(grpcServer, NewMediaServer(svc, logger))
	log.Printf("gRPC server listening on %s", port)
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
