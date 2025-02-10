package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/watchlist-kata/media/api"
	"github.com/watchlist-kata/media/internal/config"
	"github.com/watchlist-kata/media/internal/repository"
	"github.com/watchlist-kata/media/internal/tmdb"
	"gorm.io/gorm"
	"log/slog"
)

// Service определяет интерфейс для сервиса
type Service interface {
	GetMediaByID(ctx context.Context, req *api.GetMediaByIDRequest) (*api.Media, error)
	GetMediasByName(ctx context.Context, req *api.GetMediasByNameRequest) (*api.MediaList, error)
	SearchTMDB(ctx context.Context, name string) ([]*api.Media, error)
	SaveMedia(ctx context.Context, req *api.SaveMediaRequest) (*api.Media, error)
	UpdateMedia(ctx context.Context, media *api.Media) (*api.Media, error)
}

// MediaService представляет собой структуру сервиса
type MediaService struct {
	repo   repository.Repository
	Logger *slog.Logger
}

// NewMediaService создает новый экземпляр MediaService
func NewMediaService(repo repository.Repository, logger *slog.Logger) Service {
	return &MediaService{repo: repo, Logger: logger}
}

// Verify that MediaService implements the Service interface at compile time.
var _ Service = (*MediaService)(nil)

// GetMediaByID получает медиа по его ID
func (s *MediaService) GetMediaByID(ctx context.Context, req *api.GetMediaByIDRequest) (*api.Media, error) {
	s.Logger.InfoContext(ctx, "GetMediaByID called", "id", req.Id)
	media, err := s.repo.GetMediaByID(ctx, req.Id)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to GetMediaByID", "id", req.Id, "error", err)
		return nil, fmt.Errorf("failed to get media with id %d: %w", req.Id, err)
	}
	return media, nil
}

// GetMediasByName получает медиа по названию, ищет в TMDB и локальной БД, и обновляет локальную БД асинхронно
func (s *MediaService) GetMediasByName(ctx context.Context, req *api.GetMediasByNameRequest) (*api.MediaList, error) {
	s.Logger.InfoContext(ctx, "GetMediasByName called", "name", req.Name)

	// 1. Поиск медиа в TMDB
	tmdbMedias, err := s.SearchTMDB(ctx, req.Name)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to search TMDB", "name", req.Name, "error", err)
		// Не прерываем выполнение, продолжаем с локальной базой данных
	}

	// 2. Получение медиа из локальной базы данных
	localMedias, err := s.repo.GetMediasByName(ctx, req.Name)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to GetMediasByName from DB", "name", req.Name, "error", err)
		return nil, fmt.Errorf("failed to get medias by name %s from DB: %w", req.Name, err)
	}

	// 3. Объединение результатов
	var mediaPointers []*api.Media
	mediaMap := make(map[int64]*api.Media) // Используем map для быстрого поиска и избежания дубликатов

	// Добавляем медиа из локальной базы данных в map и список для возврата
	for i := range localMedias {
		media := localMedias[i]
		mediaPointers = append(mediaPointers, media)
		mediaMap[media.TmdbId] = media
	}

	// Добавляем медиа из TMDB, которых еще нет в локальной базе данных
	for _, tmdbMedia := range tmdbMedias {
		if _, exists := mediaMap[tmdbMedia.TmdbId]; !exists {
			mediaPointers = append(mediaPointers, tmdbMedia)
			mediaMap[tmdbMedia.TmdbId] = tmdbMedia
		}
	}

	// Асинхронное обновление базы данных
	go s.updateDatabaseAsync(context.Background(), localMedias, tmdbMedias)

	return &api.MediaList{Medias: mediaPointers}, nil
}

func (s *MediaService) updateDatabaseAsync(ctx context.Context, localMedias []*api.Media, tmdbMedias []*api.Media) {
	mediaMap := make(map[int64]*api.Media)

	// Заполняем map локальными медиа
	for i := range localMedias {
		media := localMedias[i]
		mediaMap[media.TmdbId] = media
	}

	// Обрабатываем медиа из TMDB
	for _, tmdbMedia := range tmdbMedias {
		if localMedia, exists := mediaMap[tmdbMedia.TmdbId]; exists {
			// Медиа существует в локальной базе данных, обновляем информацию
			if needsUpdate(localMedia, tmdbMedia) {
				tmdbMedia.Id = localMedia.Id // Сохраняем ID из локальной базы данных
				_, err := s.UpdateMedia(ctx, tmdbMedia)
				if err != nil {
					s.Logger.ErrorContext(ctx, "Failed to update media", "tmdbID", tmdbMedia.TmdbId, "error", err)
					// Здесь можно не оборачивать ошибку, так как это асинхронная операция
					// и ошибка уже залогирована.
				}
			}
		} else {
			// Медиа не существует в локальной базе данных, сохраняем
			err := s.repo.CreateMedia(ctx, tmdbMedia)
			if err != nil {
				s.Logger.ErrorContext(ctx, "Failed to create media", "tmdbID", tmdbMedia.TmdbId, "error", err)
				// Здесь можно не оборачивать ошибку, так как это асинхронная операция
				// и ошибка уже залогирована.
			}
		}
	}
}

// needsUpdate проверяет, нужно ли обновлять информацию о медиа
func needsUpdate(localMedia, tmdbMedia *api.Media) bool {
	return localMedia.Title != tmdbMedia.Title ||
		localMedia.TitleRu != tmdbMedia.TitleRu ||
		localMedia.Description != tmdbMedia.Description ||
		localMedia.DescriptionRu != tmdbMedia.DescriptionRu ||
		localMedia.ReleaseDate != tmdbMedia.ReleaseDate ||
		localMedia.Poster != tmdbMedia.Poster
}

// SearchTMDB выполняет поиск медиа в TMDB по названию
func (s *MediaService) SearchTMDB(ctx context.Context, name string) ([]*api.Media, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to load config", "error", err)
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Инициализируем клиент TMDB
	tmdbClient, err := tmdb.InitTMDBClient(cfg)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to initialize TMDB client", "error", err)
		return nil, fmt.Errorf("failed to initialize TMDB client: %w", err)
	}

	// Выполняем поиск
	searchResults, err := tmdbClient.GetSearchMovies(name, nil)
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to search movies in TMDB", "error", err)
		return nil, fmt.Errorf("failed to search movies in TMDB: %w", err)
	}

	var medias []*api.Media
	for _, result := range searchResults.Results {
		posterURL := result.PosterPath
		// Обрезаем URL постера, оставляя только имя файла
		if posterURL != "" {
			parts := strings.Split(posterURL, "/")
			if len(parts) > 0 {
				posterURL = parts[len(parts)-1]
			}
		}
		media := &api.Media{
			TmdbId:      result.ID,
			Title:       result.Title,
			TitleRu:     result.OriginalTitle,
			Description: result.Overview,
			ReleaseDate: result.ReleaseDate,
			Poster:      posterURL, // Сохраняем только имя файла
			Type:        "Movie",   // Assuming the default type is Movie
		}
		medias = append(medias, media)
	}

	return medias, nil
}

// SaveMedia сохраняет новое медиа
func (s *MediaService) SaveMedia(ctx context.Context, req *api.SaveMediaRequest) (*api.Media, error) {
	s.Logger.InfoContext(ctx, "SaveMedia called", "tmdbID", req.Media.TmdbId)
	if err := s.repo.CreateMedia(ctx, req.Media); err != nil {
		s.Logger.ErrorContext(ctx, "Failed to SaveMedia", "tmdbID", req.Media.TmdbId, "error", err)
		return nil, fmt.Errorf("failed to save media with tmdb_id %d: %w", req.Media.TmdbId, err)
	}
	return req.Media, nil
}

// UpdateMedia обновляет существующее медиа
func (s *MediaService) UpdateMedia(ctx context.Context, media *api.Media) (*api.Media, error) {
	s.Logger.InfoContext(ctx, "UpdateMedia called", "tmdbID", media.TmdbId)
	existingMedia, err := s.repo.GetMediaByID(ctx, media.Id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.Logger.WarnContext(ctx, "Media with id not found", "mediaID", media.Id, "error", err)
			return nil, fmt.Errorf("media with id %d not found: %w", media.Id, err)
		}
		s.Logger.ErrorContext(ctx, "Failed to UpdateMedia", "mediaID", media.Id, "error", err)
		return nil, fmt.Errorf("failed to get existing media with id %d: %w", media.Id, err)
	}

	if existingMedia.TmdbId == media.TmdbId &&
		existingMedia.Type == media.Type &&
		existingMedia.Title == media.Title &&
		existingMedia.TitleRu == media.TitleRu &&
		existingMedia.Description == media.Description &&
		existingMedia.DescriptionRu == media.DescriptionRu &&
		existingMedia.ReleaseDate == media.ReleaseDate &&
		existingMedia.Poster == media.Poster {
		s.Logger.InfoContext(ctx, "No fields to update", "mediaID", media.Id)
		return nil, fmt.Errorf("all fields are the same, nothing to update")
	}

	if err := s.repo.UpdateMedia(ctx, media); err != nil {
		s.Logger.ErrorContext(ctx, "Failed to UpdateMedia in repository", "mediaID", media.Id, "error", err)
		return nil, fmt.Errorf("failed to update media with id %d in repository: %w", media.Id, err)
	}

	updatedMedia, err := s.repo.GetMediaByID(ctx, media.Id) // Fetch updated media
	if err != nil {
		s.Logger.ErrorContext(ctx, "Failed to get updated media", "mediaID", media.Id, "error", err)
		return nil, fmt.Errorf("failed to get updated media with id %d: %w", media.Id, err)
	}

	s.Logger.InfoContext(ctx, "Media updated successfully", "mediaID", media.Id)
	return updatedMedia, nil // Return updated media
}
