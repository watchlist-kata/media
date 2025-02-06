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

// Service представляет собой структуру сервиса
type Service struct {
	repo   *repository.Repository
	Logger *slog.Logger
}

// NewService создает новый экземпляр сервиса
func NewService(repo *repository.Repository, logger *slog.Logger) *Service {
	return &Service{repo: repo, Logger: logger}
}

// GetMediaByID получает медиа по его ID и обновляет базу данных, если необходимо
func (s *Service) GetMediaByID(req *api.GetMediaRequest) (*api.Media, error) {
	s.Logger.InfoContext(context.Background(), "GetMediaByID called", "id", req.Id)
	media, err := s.repo.GetMediaByID(req.Id)
	if err != nil {
		s.Logger.ErrorContext(context.Background(), "Failed to GetMediaByID", "id", req.Id, "error", err)
		return nil, err
	}
	return media, nil
}

func (s *Service) GetMediasByName(req *api.GetMediaRequest) (*api.MediaList, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		s.Logger.ErrorContext(context.Background(), "Failed to load config", "error", err)
		return nil, err
	}

	s.Logger.InfoContext(context.Background(), "GetMediasByName called", "name", req.Name)

	// 1. Поиск медиа в TMDB
	tmdbMedias, err := s.searchTMDB(req.Name, cfg)
	if err != nil {
		s.Logger.ErrorContext(context.Background(), "Failed to search TMDB", "name", req.Name, "error", err)
		// Не прерываем выполнение, продолжаем с локальной базой данных
	}

	// 2. Получение медиа из локальной базы данных
	localMedias, err := s.repo.GetMediasByName(req.Name)
	if err != nil {
		s.Logger.ErrorContext(context.Background(), "Failed to GetMediasByName from DB", "name", req.Name, "error", err)
		return nil, err
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
	go s.updateDatabaseAsync(localMedias, tmdbMedias)

	return &api.MediaList{Medias: mediaPointers}, nil
}

func (s *Service) updateDatabaseAsync(localMedias []*api.Media, tmdbMedias []*api.Media) {
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
				_, err := s.UpdateMedia(tmdbMedia)
				if err != nil {
					s.Logger.ErrorContext(context.Background(), "Failed to update media", "tmdbID", tmdbMedia.TmdbId, "error", err)
				}
			}
		} else {
			// Медиа не существует в локальной базе данных, сохраняем
			err := s.repo.CreateMedia(tmdbMedia)
			if err != nil {
				s.Logger.ErrorContext(context.Background(), "Failed to create media", "tmdbID", tmdbMedia.TmdbId, "error", err)
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

func (s *Service) searchTMDB(name string, cfg *config.Config) ([]*api.Media, error) {
	// Инициализируем клиент TMDB
	tmdbClient, err := tmdb.InitTMDBClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize TMDB client: %w", err)
	}

	// Выполняем поиск
	searchResults, err := tmdbClient.GetSearchMovies(name, nil)
	if err != nil {
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

// SaveMedia сохраняет новое медиа или обновляет существующее
func (s *Service) SaveMedia(req *api.SaveMediaRequest) (*api.Media, error) {
	s.Logger.InfoContext(context.Background(), "SaveMedia called", "tmdbID", req.Media.TmdbId)
	if err := s.repo.CreateMedia(req.Media); err != nil {
		s.Logger.ErrorContext(context.Background(), "Failed to SaveMedia", "tmdbID", req.Media.TmdbId, "error", err)
		return nil, err
	}
	return req.Media, nil
}

// UpdateMedia обновляет существующее медиа, если информация отличается
func (s *Service) UpdateMedia(media *api.Media) (*api.Media, error) {
	s.Logger.InfoContext(context.Background(), "UpdateMedia called", "tmdbID", media.TmdbId)
	existingMedia, err := s.repo.GetMediaByID(media.Id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.Logger.WarnContext(context.Background(), "Media with id not found", "mediaID", media.Id, "error", err)
			return nil, fmt.Errorf("media with id %d not found", media.Id)
		}
		s.Logger.ErrorContext(context.Background(), "Failed to UpdateMedia", "mediaID", media.Id, "error", err)
		return nil, err
	}

	if existingMedia.TmdbId == media.TmdbId &&
		existingMedia.Type == media.Type &&
		existingMedia.Title == media.Title &&
		existingMedia.TitleRu == media.TitleRu &&
		existingMedia.Description == media.Description &&
		existingMedia.DescriptionRu == media.DescriptionRu &&
		existingMedia.ReleaseDate == media.ReleaseDate &&
		existingMedia.Poster == media.Poster {
		s.Logger.InfoContext(context.Background(), "No fields to update", "mediaID", media.Id)
		return nil, fmt.Errorf("all fields are the same, nothing to update")
	}

	if err := s.repo.UpdateMedia(media); err != nil {
		s.Logger.ErrorContext(context.Background(), "Failed to UpdateMedia in repository", "mediaID", media.Id, "error", err)
		return nil, err
	}

	updatedMedia, err := s.repo.GetMediaByID(media.Id) // Fetch updated media
	if err != nil {
		s.Logger.ErrorContext(context.Background(), "Failed to get updated media", "mediaID", media.Id, "error", err)
		return nil, err
	}

	s.Logger.InfoContext(context.Background(), "Media updated successfully", "mediaID", media.Id)
	return updatedMedia, nil // Return updated media
}
