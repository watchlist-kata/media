package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/watchlist-kata/media/internal/config"
	"github.com/watchlist-kata/media/internal/repository"
	"github.com/watchlist-kata/media/internal/tmdb"
	"github.com/watchlist-kata/protos/media"
	"gorm.io/gorm"
	"log/slog"
)

// Service определяет интерфейс для сервиса
type Service interface {
	GetMediaByID(ctx context.Context, req *media.GetMediaByIDRequest) (*media.Media, error)
	GetMediasByName(ctx context.Context, req *media.GetMediasByNameRequest) (*media.MediaList, error)
	SearchTMDB(ctx context.Context, name string) ([]*media.Media, error)
	SaveMedia(ctx context.Context, req *media.SaveMediaRequest) (*media.Media, error)
	UpdateMedia(ctx context.Context, m *media.Media) (*media.Media, error)
}

// MediaService представляет собой структуру сервиса
type MediaService struct {
	repo       repository.Repository
	Logger     *slog.Logger
	cfg        *config.Config
	tmdbClient *tmdb.TMDBClient
}

// NewMediaService создает новый экземпляр MediaService
func NewMediaService(repo repository.Repository, logger *slog.Logger, cfg *config.Config) (Service, error) {
	tmdbClient, err := tmdb.NewTMDBClient(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize TMDB client: %s", err)
	}

	return &MediaService{
		repo:       repo,
		Logger:     logger,
		cfg:        cfg,
		tmdbClient: tmdbClient,
	}, nil
}

// Verify that MediaService implements the Service interface at compile time.
var _ Service = (*MediaService)(nil)

// GetMediaByID получает медиа по его ID
func (s *MediaService) GetMediaByID(ctx context.Context, req *media.GetMediaByIDRequest) (*media.Media, error) {
	s.Logger.InfoContext(ctx, "GetMediaByID called", "id", req.Id)
	m, err := s.repo.GetMediaByID(ctx, req.Id)
	if err != nil {
		return nil, s.handleError(ctx, "Failed to GetMediaByID", fmt.Sprintf("failed to get media with id %d: %s", req.Id, err), "id", req.Id, "error", err)
	}
	return m, nil
}

// GetMediasByName получает медиа по названию, ищет в TMDB и локальной БД, и обновляет локальную БД асинхронно
func (s *MediaService) GetMediasByName(ctx context.Context, req *media.GetMediasByNameRequest) (*media.MediaList, error) {
	s.Logger.InfoContext(ctx, "GetMediasByName called", "name", req.Name)

	// 1. Поиск медиа в TMDB
	tmdbMedias, err := s.SearchTMDB(ctx, req.Name)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to search TMDB", "name", req.Name, "error", err)
		// Не прерываем выполнение, продолжаем с локальной базой данных
	}

	// 2. Получение медиа из локальной базы данных
	localMedias, err := s.repo.GetMediasByNameFromRepo(ctx, req.Name)
	if err != nil {
		return nil, s.handleError(ctx, "Failed to GetMediasByName from DB", fmt.Sprintf("failed to get medias by name %s from DB: %s", req.Name, err), "name", req.Name, "error", err)
	}

	// 3. Объединение результатов
	var mediaPointers []*media.Media
	mediaMap := make(map[int64]*media.Media)

	addMedia := func(m *media.Media) {
		if _, exists := mediaMap[m.TmdbId]; !exists {
			mediaPointers = append(mediaPointers, m)
			mediaMap[m.TmdbId] = m
		} else {
			// Возможно, нужно обновить локальные данные, если TMDB данные актуальнее
			if needsUpdate(mediaMap[m.TmdbId], m) {
				mediaMap[m.TmdbId] = m
			}
		}
	}

	for _, m := range localMedias {
		addMedia(m)
	}

	for _, m := range tmdbMedias {
		addMedia(m)
	}

	// Асинхронное обновление базы данных
	go s.updateDatabaseAsync(ctx, localMedias, tmdbMedias)

	return &media.MediaList{Medias: mediaPointers}, nil
}

// updateDatabaseAsync обновляет базу данных асинхронно
func (s *MediaService) updateDatabaseAsync(ctx context.Context, localMedias []*media.Media, tmdbMedias []*media.Media) {
	s.Logger.InfoContext(ctx, "updateDatabaseAsync started")
	defer s.Logger.InfoContext(ctx, "updateDatabaseAsync finished")

	mediaMap := make(map[int64]*media.Media)

	// Заполняем map локальными медиа
	for i := range localMedias {
		m := localMedias[i]
		mediaMap[m.TmdbId] = m
	}

	// Обрабатываем медиа из TMDB асинхронно
	for _, tmdbMedia := range tmdbMedias {
		go func(tmdbMedia *media.Media) {
			select {
			case <-ctx.Done():
				s.Logger.WarnContext(ctx, "updateDatabaseAsync cancelled", "tmdbID", tmdbMedia.TmdbId, "error", ctx.Err())
				return // Завершаем работу, если контекст отменен
			default:
				// Продолжаем обработку
			}

			if localMedia, exists := mediaMap[tmdbMedia.TmdbId]; exists {
				// Медиа существует в локальной базе данных, обновляем информацию
				if needsUpdate(localMedia, tmdbMedia) {
					tmdbMedia.Id = localMedia.Id // Сохраняем ID из локальной базы данных
					_, err := s.UpdateMedia(ctx, tmdbMedia)
					if err != nil {
						s.Logger.ErrorContext(ctx, "Failed to update media", "tmdbID", tmdbMedia.TmdbId, "error", err)
					}
				}
			} else {
				// Медиа не существует в локальной базе данных, сохраняем
				_, err := s.repo.CreateMedia(ctx, tmdbMedia)
				if err != nil {
					s.Logger.ErrorContext(ctx, "Failed to create media", "tmdbID", tmdbMedia.TmdbId, "error", err)
				}
			}
		}(tmdbMedia) // Передаем tmdbMedia в горутину
	}
}

// needsUpdate проверяет, нужно ли обновлять информацию о медиа
func needsUpdate(localMedia, tmdbMedia *media.Media) bool {
	return localMedia.Title != tmdbMedia.Title ||
		localMedia.TitleRu != tmdbMedia.TitleRu ||
		localMedia.Description != tmdbMedia.Description ||
		localMedia.DescriptionRu != tmdbMedia.DescriptionRu ||
		localMedia.ReleaseDate != tmdbMedia.ReleaseDate ||
		localMedia.Poster != tmdbMedia.Poster
}

// SearchTMDB ищет media в TMDB.
func (s *MediaService) SearchTMDB(ctx context.Context, name string) ([]*media.Media, error) {
	s.Logger.InfoContext(ctx, "SearchTMDB called", "name", name)

	medias, err := s.tmdbClient.GetSearchMultiContext(ctx, name, nil)
	if err != nil {
		return nil, s.handleError(ctx, "Failed to search TMDB", fmt.Sprintf("failed to search TMDB: %s", err), "error", err)
	}

	return medias, nil
}

// SaveMedia сохраняет новое медиа
func (s *MediaService) SaveMedia(ctx context.Context, req *media.SaveMediaRequest) (*media.Media, error) {
	s.Logger.InfoContext(ctx, "SaveMedia called", "tmdbID", req.Media.TmdbId)
	newMedia, err := s.repo.CreateMedia(ctx, req.Media)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to SaveMedia", "tmdbID", req.Media.TmdbId, "error", err)
		return nil, fmt.Errorf("failed to save media with tmdb_id %d: %s", req.Media.TmdbId, err)
	}
	return newMedia, nil
}

// UpdateMedia обновляет существующее медиа
func (s *MediaService) UpdateMedia(ctx context.Context, m *media.Media) (*media.Media, error) {
	s.Logger.InfoContext(ctx, "UpdateMedia called", "tmdbID", m.TmdbId)
	existingMedia, err := s.repo.GetMediaByID(ctx, m.Id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.Logger.WarnContext(ctx, "Media with id not found", "mediaID", m.Id, "error", err)
			return nil, fmt.Errorf("media with id %d not found: %s", m.Id, err)
		}
		return nil, s.handleError(ctx, "Failed to UpdateMedia", fmt.Sprintf("failed to get existing media with id %d: %s", m.Id, err), "mediaID", m.Id, "error", err)
	}

	if !needsUpdate(existingMedia, m) {
		s.Logger.InfoContext(ctx, "No fields to update", "mediaID", m.Id)
		return nil, fmt.Errorf("all fields are the same, nothing to update")
	}

	updatedMedia, err := s.repo.UpdateMedia(ctx, m)
	if err != nil {
		return nil, s.handleError(ctx, "Failed to UpdateMedia in repository", fmt.Sprintf("failed to update media with id %d in repository: %s", m.Id, err), "mediaID", m.Id, "error", err)
	}

	s.Logger.InfoContext(ctx, "Media updated successfully", "mediaID", m.Id)
	return updatedMedia, nil
}

// handleError централизованно обрабатывает ошибки с логированием
func (s *MediaService) handleError(ctx context.Context, message, errStr string, args ...interface{}) error {
	s.Logger.ErrorContext(ctx, message, args...)
	return fmt.Errorf(errStr)
}
