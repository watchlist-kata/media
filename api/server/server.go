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

// MediaServer represents the gRPC service
type MediaServer struct {
	media.UnimplementedMediaServiceServer
	svc    service.Service
	Logger *slog.Logger
}

// NewMediaServer creates a new MediaServer
func NewMediaServer(svc service.Service, logger *slog.Logger) *MediaServer {
	return &MediaServer{
		svc:    svc,
		Logger: logger,
	}
}

// checkContextCancellation checks if the context was cancelled
func (s *MediaServer) checkContextCancellation(ctx context.Context, methodName string) error {
	select {
	case <-ctx.Done():
		s.Logger.WarnContext(ctx, methodName+" cancelled", "error", ctx.Err())
		return status.FromContextError(ctx.Err()).Err()
	default:
		return nil
	}
}

// GetRequestID gets or generates a request ID
func GetRequestID(ctx context.Context) string {
	if reqID, ok := ctx.Value("requestID").(string); ok && reqID != "" {
		return reqID
	}
	return uuid.New().String()
}

// SaveMedia implements the SaveMedia gRPC method
func (s *MediaServer) SaveMedia(ctx context.Context, req *media.SaveMediaRequest) (*media.Media, error) {
	if err := s.checkContextCancellation(ctx, "SaveMedia"); err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, "requestID", GetRequestID(ctx))
	requestID := GetRequestID(ctx)

	s.Logger.InfoContext(ctx, "SaveMedia called", "kinopoiskID", req.Media.KinopoiskId, "request_id", requestID)

	m, err := s.svc.SaveMedia(ctx, req)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to SaveMedia", "kinopoiskID", req.Media.KinopoiskId, "error", err, "request_id", requestID)
		return nil, status.Errorf(codes.Internal, "failed to save media with kinopoiskID %d: %v", req.Media.KinopoiskId, err)
	}
	return m, nil
}

// GetMediaByID implements the GetMediaByID gRPC method
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

// GetMediasByName implements the GetMediasByName gRPC method
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

// UpdateMedia implements the UpdateMedia gRPC method
func (s *MediaServer) UpdateMedia(ctx context.Context, req *media.SaveMediaRequest) (*media.Media, error) {
	if err := s.checkContextCancellation(ctx, "UpdateMedia"); err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, "requestID", GetRequestID(ctx))
	requestID := GetRequestID(ctx)

	s.Logger.InfoContext(ctx, "UpdateMedia called", "kinopoiskID", req.Media.KinopoiskId, "request_id", requestID)
	m, err := s.svc.UpdateMedia(ctx, req.Media)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to UpdateMedia", "kinopoiskID", req.Media.KinopoiskId, "error", err, "request_id", requestID)
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

	ctx = context.WithValue(ctx, "requestID", GetRequestID(ctx))
	requestID := GetRequestID(ctx)

	s.Logger.InfoContext(ctx, "SearchKinopoisk called", "name", req.Name, "request_id", requestID)

	medias, err := s.svc.SearchKinopoisk(ctx, req.Name)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to search Kinopoisk", "name", req.Name, "error", err, "request_id", requestID)
		return nil, status.Errorf(codes.Internal, "failed to search Kinopoisk with name %s: %v", req.Name, err)
	}

	return &media.MediaList{Medias: medias}, nil
}

// DeleteMedia implements the DeleteMedia gRPC method
func (s *MediaServer) DeleteMedia(ctx context.Context, req *media.DeleteMediaRequest) (*media.DeleteMediaResponse, error) {
	if err := s.checkContextCancellation(ctx, "DeleteMedia"); err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, "requestID", GetRequestID(ctx))
	requestID := GetRequestID(ctx)

	s.Logger.InfoContext(ctx, "DeleteMedia called", "id", req.Id, "request_id", requestID)

	resp, err := s.svc.DeleteMedia(ctx, req)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to DeleteMedia", "id", req.Id, "error", err, "request_id", requestID)
		return nil, status.Errorf(codes.Internal, "failed to delete media with id %d: %v", req.Id, err)
	}

	return resp, nil
}

// loggingInterceptor is a gRPC interceptor for logging
func loggingInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		requestID := GetRequestID(ctx)
		ctx = context.WithValue(ctx, "requestID", requestID)

		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			logger.InfoContext(ctx, "Request Headers", "metadata", md, "request_id", requestID)
		}

		logger.InfoContext(ctx, "Request started", "method", info.FullMethod, "request_id", requestID, "start_time", start.Format(time.RFC3339))

		resp, err := handler(ctx, req)

		duration := time.Since(start)

		if err != nil {
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

// StartGRPCServer starts the gRPC server
func StartGRPCServer(port string, svc service.Service, logger *slog.Logger, grpcServer *grpc.Server) error {
	if grpcServer == nil {
		grpcServer = grpc.NewServer(
			grpc.UnaryInterceptor(loggingInterceptor(logger)),
		)
	}
	listener, err := net.Listen("tcp", port)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	media.RegisterMediaServiceServer(grpcServer, NewMediaServer(svc, logger))
	logger.Info("gRPC server listening", "port", port)
	if err := grpcServer.Serve(listener); err != nil {
		return fmt.Errorf("failed to serve: %w", err)
	}
	return nil
}
