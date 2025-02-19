package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/watchlist-kata/media/internal/config"
	"github.com/watchlist-kata/media/internal/kinopoisk"
	"github.com/watchlist-kata/media/internal/repository"
	"github.com/watchlist-kata/protos/media"
	"gorm.io/gorm"
)

// Service определяет интерфейс для сервиса
type Service interface {
	GetMediaByID(ctx context.Context, req *media.GetMediaByIDRequest) (*media.Media, error)
	GetMediasByName(ctx context.Context, req *media.GetMediasByNameRequest) (*media.MediaList, error)
	SearchKinopoisk(ctx context.Context, name string) ([]*media.Media, error)
	SaveMedia(ctx context.Context, req *media.SaveMediaRequest) (*media.Media, error)
	UpdateMedia(ctx context.Context, m *media.Media) (*media.Media, error)
	DeleteMedia(ctx context.Context, req *media.DeleteMediaRequest) (*media.DeleteMediaResponse, error)
}

// MediaService представляет собой структуру сервиса
type MediaService struct {
	repo            repository.Repository
	logger          *slog.Logger
	cfg             *config.Config
	kinopoiskClient *kinopoisk.KPClient
}

// NewMediaService создает новый экземпляр MediaService
func NewMediaService(repo repository.Repository, logger *slog.Logger, cfg *config.Config) (Service, error) {
	// Инициализация KinopoiskClient с API-ключом и логгером
	kinopoiskAPIKey := cfg.KinopoiskAPIKey // Получаем API ключ из конфигурации
	kinopoiskClient, err := kinopoisk.NewKinopoiskClient(kinopoiskAPIKey, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Kinopoisk client: %w", err)
	}

	return &MediaService{
		repo:            repo,
		logger:          logger,
		cfg:             cfg,
		kinopoiskClient: kinopoiskClient,
	}, nil
}

// Verify that MediaService implements the Service interface at compile time.
var _ Service = (*MediaService)(nil)

// GetMediaByID получает медиа по его ID
func (s *MediaService) GetMediaByID(ctx context.Context, req *media.GetMediaByIDRequest) (*media.Media, error) {
	s.logger.InfoContext(ctx, "GetMediaByID called", "id", req.Id)
	m, err := s.repo.GetMediaByID(ctx, req.Id)
	if err != nil {
		return nil, s.handleError(ctx, "Failed to GetMediaByID", fmt.Errorf("failed to get media with id %d: %w", req.Id, err), "id", req.Id, "error", err)
	}
	return m, nil
}

// GetMediasByName получает медиа по названию
func (s *MediaService) GetMediasByName(ctx context.Context, req *media.GetMediasByNameRequest) (*media.MediaList, error) {
	s.logger.InfoContext(ctx, "GetMediasByName called", "name", req.Name)

	// 1. Поиск медиа в Кинопоиске
	kinopoiskMedias, err := s.SearchKinopoisk(ctx, req.Name)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to search Kinopoisk", "name", req.Name, "error", err)
	}

	// 2. Получение медиа из локальной базы данных
	localMedias, err := s.repo.GetMediasByNameFromRepo(ctx, req.Name)
	if err != nil {
		return nil, s.handleError(ctx, "Failed to GetMediasByName from DB", fmt.Errorf("failed to get medias by name %s from DB: %w", req.Name, err), "name", req.Name, "error", err)
	}

	// 3. Объединение результатов
	var mediaPointers []*media.Media
	mediaMap := make(map[int64]*media.Media)

	addMedia := func(m *media.Media) {
		if _, exists := mediaMap[m.KinopoiskId]; !exists {
			mediaPointers = append(mediaPointers, m)
			mediaMap[m.KinopoiskId] = m
		}
	}

	for _, m := range localMedias {
		addMedia(m)
	}

	for _, kpMedia := range kinopoiskMedias {
		addMedia(kpMedia)
	}

	// 4. Асинхронное сохранение и обновление медиа
	go func() {
		for _, m := range mediaPointers {
			existingMedia, err := s.repo.GetMediaByID(context.Background(), m.KinopoiskId)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					// Медиа нет в базе - сохраняем его
					s.logger.InfoContext(context.Background(), "Saving media from Kinopoisk", "kinopoiskID", m.KinopoiskId)
					_, err := s.repo.CreateMedia(context.Background(), m)
					if err != nil {
						s.logger.ErrorContext(context.Background(), "Failed to save media from Kinopoisk", "kinopoiskID", m.KinopoiskId, "error", err)
					} else {
						s.logger.InfoContext(context.Background(), "Media from Kinopoisk saved successfully", "kinopoiskID", m.KinopoiskId)
					}
				} else {
					s.logger.ErrorContext(context.Background(), "Failed to check existing media in DB", "kinopoiskID", m.KinopoiskId, "error", err)
				}
			} else {
				// Если медиа существует - проверяем на необходимость обновления
				if needsUpdate(existingMedia, m) {
					s.logger.InfoContext(context.Background(), "Updating media from Kinopoisk", "kinopoiskID", m.KinopoiskId)
					_, err := s.repo.UpdateMedia(context.Background(), m)
					if err != nil {
						s.logger.ErrorContext(context.Background(), "Failed to update media from Kinopoisk", "kinopoiskID", m.KinopoiskId, "error", err)
					} else {
						s.logger.InfoContext(context.Background(), "Media from Kinopoisk updated successfully", "kinopoiskID", m.KinopoiskId)
					}
				}
			}
		}
	}()

	return &media.MediaList{Medias: mediaPointers}, nil
}

// SearchKinopoisk ищет медиа в Кинопоиске.
func (s *MediaService) SearchKinopoisk(ctx context.Context, name string) ([]*media.Media, error) {
	s.logger.InfoContext(ctx, "SearchKinopoisk called", "name", name)

	medias, err := s.kinopoiskClient.SearchByKeyword(ctx, name)
	if err != nil {
		return nil, s.handleError(ctx, "Failed to search Kinopoisk", fmt.Errorf("failed to search Kinopoisk: %w", err), "error", err)
	}

	return medias, nil
}

// SaveMedia сохраняет новое медиа
func (s *MediaService) SaveMedia(ctx context.Context, req *media.SaveMediaRequest) (*media.Media, error) {
	s.logger.InfoContext(ctx, "SaveMedia called", "kinopoiskID", req.Media.KinopoiskId)
	newMedia, err := s.repo.CreateMedia(ctx, req.Media)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to SaveMedia", "kinopoiskID", req.Media.KinopoiskId, "error", err)
		return nil, s.handleError(ctx, "Failed to SaveMedia", fmt.Errorf("failed to save media with kinopoisk_id %d: %w", req.Media.KinopoiskId, err), "kinopoiskID", req.Media.KinopoiskId)
	}
	return newMedia, nil
}

// UpdateMedia обновляет существующее медиа
func (s *MediaService) UpdateMedia(ctx context.Context, m *media.Media) (*media.Media, error) {
	s.logger.InfoContext(ctx, "UpdateMedia called", "kinopoiskID", m.KinopoiskId)

	existingMedia, err := s.repo.GetMediaByID(ctx, m.Id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.logger.WarnContext(ctx, "Media with id not found", "mediaID", m.Id, "error", err)
			return nil, fmt.Errorf("media with id %d not found: %w", m.Id, err)
		}
		return nil, s.handleError(ctx, "Failed to UpdateMedia", fmt.Errorf("failed to get existing media with id %d: %w", m.Id, err), "mediaID", m.Id, "error", err)
	}

	// Сравниваем поля и определяем, нужно ли обновлять запись
	if !needsUpdate(existingMedia, m) {
		s.logger.InfoContext(ctx, "No fields to update", "mediaID", m.Id)
		return existingMedia, nil // Возвращаем существующую запись, так как обновлять нечего
	}

	updatedMedia, err := s.repo.UpdateMedia(ctx, m)
	if err != nil {
		return nil, s.handleError(ctx, "Failed to UpdateMedia in repository", fmt.Errorf("failed to update media with id %d in repository: %w", m.Id, err), "mediaID", m.Id, "error", err)
	}

	s.logger.InfoContext(ctx, "Media updated successfully", "mediaID", m.Id)
	return updatedMedia, nil
}

// DeleteMedia удаляет медиа по его ID
func (s *MediaService) DeleteMedia(ctx context.Context, req *media.DeleteMediaRequest) (*media.DeleteMediaResponse, error) {
	s.logger.InfoContext(ctx, "DeleteMedia called", "id", req.Id)

	resp, err := s.repo.DeleteMedia(ctx, req.Id)
	if err != nil {
		return nil, s.handleError(ctx, "Failed to DeleteMedia", fmt.Errorf("failed to delete media with id %d: %w", req.Id, err), "id", req.Id, "error", err)
	}

	return resp, nil
}

// handleError централизованно обрабатывает ошибки с логированием
func (s *MediaService) handleError(ctx context.Context, message string, err error, args ...interface{}) error {
	s.logger.ErrorContext(ctx, message, args...)
	return fmt.Errorf("%s: %w", message, err)
}

// needsUpdate проверяет, нужно ли обновлять информацию о медиа
func needsUpdate(existingMedia, newMedia *media.Media) bool {
	if existingMedia == nil || newMedia == nil {
		return true // Если хотя бы одна из записей отсутствует, считаем, что нужно обновить
	}
	return existingMedia.KinopoiskId != newMedia.KinopoiskId ||
		existingMedia.Type != newMedia.Type ||
		existingMedia.NameEn != newMedia.NameEn ||
		existingMedia.NameRu != newMedia.NameRu ||
		existingMedia.Description != newMedia.Description ||
		existingMedia.Year != newMedia.Year ||
		existingMedia.Poster != newMedia.Poster ||
		existingMedia.Countries != newMedia.Countries ||
		existingMedia.Genres != newMedia.Genres
}
