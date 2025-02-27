package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/watchlist-kata/media/internal/service"
	"github.com/watchlist-kata/protos/media"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Константы для ключей контекста
const (
	contextRequestIDKey = "requestID"
	contextMethodKey    = "method"
)

// MediaServer represents the gRPC service
type MediaServer struct {
	media.UnimplementedMediaServiceServer
	svc    service.Service
	Logger *slog.Logger
}

// NewMediaServer создает новый MediaServer
func NewMediaServer(svc service.Service, logger *slog.Logger) *MediaServer {
	return &MediaServer{
		svc:    svc,
		Logger: logger,
	}
}

// checkContextCancellation проверяет отмену контекста
func (s *MediaServer) checkContextCancellation(ctx context.Context, methodName string) error {
	select {
	case <-ctx.Done():
		s.logError(ctx, methodName, ctx.Err(), "error", ctx.Err())
		return status.FromContextError(ctx.Err()).Err()
	default:
		return nil
	}
}

// GetRequestID получает или генерирует ID запроса
func GetRequestID(ctx context.Context) string {
	if reqID, ok := ctx.Value(contextRequestIDKey).(string); ok && reqID != "" {
		return reqID
	}
	return uuid.New().String()
}

// logError централизованно обрабатывает логирование ошибок
func (s *MediaServer) logError(ctx context.Context, methodName string, err error, fields ...any) {
	s.Logger.ErrorContext(ctx, methodName+" failed", append(fields, "error", err, "stack", string(debug.Stack()))...)
}

// validateSaveMediaRequest проверяет входные данные
func (s *MediaServer) validateSaveMediaRequest(req *media.SaveMediaRequest) error {
	if req == nil {
		return status.Errorf(codes.InvalidArgument, "request cannot be nil")
	}
	if req.Media == nil {
		return status.Errorf(codes.InvalidArgument, "media cannot be nil")
	}
	if req.Media.KinopoiskId <= 0 {
		return status.Errorf(codes.InvalidArgument, "kinopoiskID must be greater than 0")
	}
	return nil
}

// SaveMedia implements the SaveMedia gRPC method
func (s *MediaServer) SaveMedia(ctx context.Context, req *media.SaveMediaRequest) (*media.Media, error) {
	// Проверяем отмену контекста
	if err := s.checkContextCancellation(ctx, "SaveMedia"); err != nil {
		return nil, err
	}

	// Добавляем ID запроса в контекст
	ctx = context.WithValue(ctx, contextRequestIDKey, GetRequestID(ctx))
	requestID := GetRequestID(ctx)

	// Добавляем имя метода в контекст для логирования
	ctx = context.WithValue(ctx, contextMethodKey, "SaveMedia")

	// Валидируем входные данные
	if err := s.validateSaveMediaRequest(req); err != nil {
		s.logError(ctx, "SaveMedia", err, "kinopoiskID", req.Media.KinopoiskId, "request_id", requestID)
		return nil, err
	}

	s.Logger.InfoContext(ctx, "SaveMedia called", "kinopoiskID", req.Media.KinopoiskId, "request_id", requestID)

	m, err := s.svc.SaveMedia(ctx, req)
	if err != nil {
		s.logError(ctx, "SaveMedia", err, "kinopoiskID", req.Media.KinopoiskId, "request_id", requestID)
		return nil, status.Errorf(codes.Internal, "failed to save media with kinopoiskID %d: %v", req.Media.KinopoiskId, err)
	}
	return m, nil
}

// GetMediaByID implements the GetMediaByID gRPC method
func (s *MediaServer) GetMediaByID(ctx context.Context, req *media.GetMediaByIDRequest) (*media.Media, error) {
	if err := s.checkContextCancellation(ctx, "GetMediaByID"); err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, contextRequestIDKey, GetRequestID(ctx))
	requestID := GetRequestID(ctx)
	ctx = context.WithValue(ctx, contextMethodKey, "GetMediaByID")

	s.Logger.InfoContext(ctx, "GetMediaByID called", "mediaID", req.Id, "request_id", requestID)

	m, err := s.svc.GetMediaByID(ctx, req)
	if err != nil {
		s.logError(ctx, "GetMediaByID", err, "mediaID", req.Id, "request_id", requestID)
		return nil, status.Errorf(codes.Internal, "failed to get media by ID %d: %v", req.Id, err)
	}
	return m, nil
}

// GetMediasByName implements the GetMediasByName gRPC method
func (s *MediaServer) GetMediasByName(ctx context.Context, req *media.GetMediasByNameRequest) (*media.MediaList, error) {
	if err := s.checkContextCancellation(ctx, "GetMediasByName"); err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, contextRequestIDKey, GetRequestID(ctx))
	requestID := GetRequestID(ctx)
	ctx = context.WithValue(ctx, contextMethodKey, "GetMediasByName")

	s.Logger.InfoContext(ctx, "GetMediasByName called", "name", req.Name, "request_id", requestID)

	mediaList, err := s.svc.GetMediasByName(ctx, req)
	if err != nil {
		s.logError(ctx, "GetMediasByName", err, "name", req.Name, "request_id", requestID)
		return nil, status.Errorf(codes.Internal, "failed to get medias by name %s: %v", req.Name, err)
	}
	return mediaList, nil
}

// UpdateMedia implements the UpdateMedia gRPC method
func (s *MediaServer) UpdateMedia(ctx context.Context, req *media.SaveMediaRequest) (*media.Media, error) {
	if err := s.checkContextCancellation(ctx, "UpdateMedia"); err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, contextRequestIDKey, GetRequestID(ctx))
	requestID := GetRequestID(ctx)
	ctx = context.WithValue(ctx, contextMethodKey, "UpdateMedia")

	s.Logger.InfoContext(ctx, "UpdateMedia called", "kinopoiskID", req.Media.KinopoiskId, "request_id", requestID)

	m, err := s.svc.UpdateMedia(ctx, req.Media)
	if err != nil {
		s.logError(ctx, "UpdateMedia", err, "kinopoiskID", req.Media.KinopoiskId, "request_id", requestID)
		// Проверяем, является ли ошибка нарушением уникальности
		if strings.Contains(err.Error(), "kinopoisk_id already exists") {
			return nil, status.Errorf(codes.AlreadyExists, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "failed to update media with kinopoiskID %d: %v", req.Media.KinopoiskId, err)
	}
	return m, nil
}

// SearchKinopoisk implements the SearchKinopoisk gRPC method
func (s *MediaServer) SearchKinopoisk(ctx context.Context, req *media.SearchKinopoiskRequest) (*media.MediaList, error) {
	if err := s.checkContextCancellation(ctx, "SearchKinopoisk"); err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, contextRequestIDKey, GetRequestID(ctx))
	requestID := GetRequestID(ctx)
	ctx = context.WithValue(ctx, contextMethodKey, "SearchKinopoisk")

	s.Logger.InfoContext(ctx, "SearchKinopoisk called", "name", req.Name, "request_id", requestID)

	medias, err := s.svc.SearchKinopoisk(ctx, req.Name)
	if err != nil {
		s.logError(ctx, "SearchKinopoisk", err, "name", req.Name, "request_id", requestID)
		return nil, status.Errorf(codes.Internal, "failed to search Kinopoisk with name %s: %v", req.Name, err)
	}

	return &media.MediaList{Medias: medias}, nil
}

// DeleteMedia implements the DeleteMedia gRPC method
func (s *MediaServer) DeleteMedia(ctx context.Context, req *media.DeleteMediaRequest) (*media.DeleteMediaResponse, error) {
	if err := s.checkContextCancellation(ctx, "DeleteMedia"); err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, contextRequestIDKey, GetRequestID(ctx))
	requestID := GetRequestID(ctx)
	ctx = context.WithValue(ctx, contextMethodKey, "DeleteMedia")

	s.Logger.InfoContext(ctx, "DeleteMedia called", "id", req.Id, "request_id", requestID)

	resp, err := s.svc.DeleteMedia(ctx, req)
	if err != nil {
		s.logError(ctx, "DeleteMedia", err, "id", req.Id, "request_id", requestID)
		return nil, status.Errorf(codes.Internal, "failed to delete media with id %d: %v", req.Id, err)
	}
	return resp, nil
}

// loggingInterceptor is a gRPC interceptor for logging
func loggingInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		requestID := GetRequestID(ctx)
		ctx = context.WithValue(ctx, contextRequestIDKey, requestID)

		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			logger.InfoContext(ctx, "Request Headers", "metadata", md, "request_id", requestID)
		}

		logger.InfoContext(ctx, "Request started", "method", info.FullMethod, "request_id", requestID, "start_time", start.Format(time.RFC3339))

		resp, err := handler(ctx, req)

		duration := time.Since(start)

		if err != nil {
			stackTrace := debug.Stack()
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

// StartGRPCServer starts the gRPC server
func StartGRPCServer(port string, svc service.Service, logger *slog.Logger, grpcServer *grpc.Server) error {
	if grpcServer == nil {
		grpcServer = grpc.NewServer(
			grpc.UnaryInterceptor(loggingInterceptor(logger)),
		)
	}

	// Формируем сообщение с портом
	startMsg := fmt.Sprintf("Starting gRPC server on port %s", port)
	logger.Info(startMsg)

	listener, err := net.Listen("tcp", port)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	media.RegisterMediaServiceServer(grpcServer, NewMediaServer(svc, logger))

	// И снова формируем сообщение с портом
	listenMsg := fmt.Sprintf("gRPC server listening on port %s", port)
	logger.Info(listenMsg)

	if err := grpcServer.Serve(listener); err != nil {
		return fmt.Errorf("failed to serve: %w", err)
	}
	return nil
}
