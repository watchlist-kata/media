package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
	"github.com/watchlist-kata/media/api"
	"github.com/watchlist-kata/media/internal/service" // Импортируем интерфейс service
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"log/slog"
)

// MediaServer представляет собой структуру для реализации gRPC сервиса
type MediaServer struct {
	api.UnimplementedMediaServiceServer
	svc    service.Service // Используем интерфейс service.Service
	Logger *slog.Logger
}

// NewMediaServer создает новый экземпляр MediaServer
func NewMediaServer(svc service.Service, logger *slog.Logger) *MediaServer { // Используем интерфейс service.Service
	return &MediaServer{
		svc:    svc,
		Logger: logger,
	}
}

func (s *MediaServer) ensureRequestContext(ctx context.Context) context.Context {
	requestID := GetRequestID(ctx)

	if requestID == "N/A" {
		requestID := uuid.New().String()
		ctx = context.WithValue(ctx, "requestID", requestID)
	}

	return ctx
}

// SaveMedia реализует метод SaveMedia из интерфейса MediaServiceServer
func (s *MediaServer) SaveMedia(ctx context.Context, req *api.SaveMediaRequest) (*api.Media, error) {
	ctx = s.ensureRequestContext(ctx)
	requestID := GetRequestID(ctx)

	s.Logger.InfoContext(ctx, "SaveMedia called", "tmdbID", req.Media.TmdbId, "request_id", requestID)

	media, err := s.svc.SaveMedia(ctx, req)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to SaveMedia", "tmdbID", req.Media.TmdbId, "error", err, "request_id", requestID, "stacktrace", fmt.Sprintf("%+v", err))
		return nil, status.Errorf(codes.Internal, "failed to save media with tmdbID %d: %v", req.Media.TmdbId, err)
	}
	return media, nil
}

// GetMediaByID реализует метод GetMediaByID из интерфейса MediaServiceServer
func (s *MediaServer) GetMediaByID(ctx context.Context, req *api.GetMediaByIDRequest) (*api.Media, error) {
	ctx = s.ensureRequestContext(ctx)
	requestID := GetRequestID(ctx)

	s.Logger.InfoContext(ctx, "GetMediaByID called", "mediaID", req.Id, "request_id", requestID)
	media, err := s.svc.GetMediaByID(ctx, req)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to GetMediaByID", "mediaID", req.Id, "error", err, "request_id", requestID, "stacktrace", fmt.Sprintf("%+v", err))
		return nil, status.Errorf(codes.Internal, "failed to get media by ID %d: %v", req.Id, err)
	}
	return media, nil
}

// GetMediasByName реализует метод GetMediasByName из интерфейса MediaServiceServer
func (s *MediaServer) GetMediasByName(ctx context.Context, req *api.GetMediasByNameRequest) (*api.MediaList, error) {
	ctx = s.ensureRequestContext(ctx)
	requestID := GetRequestID(ctx)

	s.Logger.InfoContext(ctx, "GetMediasByName called", "name", req.Name, "request_id", requestID)
	mediaList, err := s.svc.GetMediasByName(ctx, req)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to GetMediasByName", "name", req.Name, "error", err, "request_id", requestID, "stacktrace", fmt.Sprintf("%+v", err))
		return nil, status.Errorf(codes.Internal, "failed to get medias by name %s: %v", req.Name, err)
	}
	return mediaList, nil
}

// UpdateMedia реализует метод UpdateMedia из интерфейса MediaServiceServer
func (s *MediaServer) UpdateMedia(ctx context.Context, req *api.SaveMediaRequest) (*api.Media, error) {
	ctx = s.ensureRequestContext(ctx)
	requestID := GetRequestID(ctx)

	s.Logger.InfoContext(ctx, "UpdateMedia called", "tmdbID", req.Media.TmdbId, "request_id", requestID)
	media, err := s.svc.UpdateMedia(ctx, req.Media)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to UpdateMedia", "tmdbID", req.Media.TmdbId, "error", err, "request_id", requestID, "stacktrace", fmt.Sprintf("%+v", err))
		return nil, status.Errorf(codes.Internal, "failed to update media with tmdbID %d: %v", req.Media.TmdbId, err)
	}
	return media, nil
}

// SearchTMDB реализует метод SearchTMDB из интерфейса MediaServiceServer
func (s *MediaServer) SearchTMDB(ctx context.Context, req *api.SearchTMDBRequest) (*api.SearchTMDBResponse, error) {
	ctx = s.ensureRequestContext(ctx)
	requestID := GetRequestID(ctx)

	s.Logger.InfoContext(ctx, "SearchTMDB called", "name", req.Name, "request_id", requestID)

	medias, err := s.svc.SearchTMDB(ctx, req.Name)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to search TMDB", "name", req.Name, "error", err, "request_id", requestID, "stacktrace", fmt.Sprintf("%+v", err))
		return nil, status.Errorf(codes.Internal, "failed to search TMDB with name %s: %v", req.Name, err)
	}
	return &api.SearchTMDBResponse{Medias: medias}, nil
}

// GetRequestID extracts the request ID from the context
func GetRequestID(ctx context.Context) string {
	if ctx != nil {
		if reqID, ok := ctx.Value("requestID").(string); ok {
			return reqID
		}
	}
	return "N/A"
}

func loggingInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()

		// Generate request ID if not already present
		requestID := GetRequestID(ctx)
		if requestID == "N/A" {
			requestID = uuid.New().String()
			ctx = context.WithValue(ctx, "requestID", requestID)
		}

		// Extract metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			logger.InfoContext(ctx, "Request Headers", "metadata", md, "request_id", requestID)
		}

		logger.InfoContext(ctx, "Request started", "method", info.FullMethod, "request_id", requestID, "start_time", start.Format(time.RFC3339))

		resp, err := handler(ctx, req)

		duration := time.Since(start)

		if err != nil {
			// Extract stack trace
			stackTrace := debug.Stack()
			logger.ErrorContext(ctx, "Request failed", "method", info.FullMethod, "request_id", requestID, "duration", duration, "error", err, "stacktrace", string(stackTrace))
		} else {
			logger.InfoContext(ctx, "Request finished", "method", info.FullMethod, "request_id", requestID, "duration", duration)
		}

		return resp, err
	}
}

// StartGRPCServer - запуск gRPC сервера
func StartGRPCServer(port string, svc service.Service, logger *slog.Logger) { // Используем интерфейс service.Service
	listener, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Initialize gRPC server with logging interceptor
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(loggingInterceptor(logger)),
	)
	api.RegisterMediaServiceServer(grpcServer, NewMediaServer(svc, logger))
	log.Printf("gRPC server listening on %s", port)
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
