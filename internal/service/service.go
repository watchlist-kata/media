package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"

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
	wg              sync.WaitGroup
	retryCount      int
	retryInterval   time.Duration
}

// NewMediaService создает новый экземпляр MediaService
func NewMediaService(repo repository.Repository, logger *slog.Logger, cfg *config.Config) (Service, error) {
	kinopoiskAPIKey := cfg.KinopoiskAPIKey
	kinopoiskClient, err := kinopoisk.NewKinopoiskClient(kinopoiskAPIKey, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Kinopoisk client: %w", err)
	}

	return &MediaService{
		repo:            repo,
		logger:          logger,
		cfg:             cfg,
		kinopoiskClient: kinopoiskClient,
		retryCount:      3,
		retryInterval:   2 * time.Second,
	}, nil
}

// Verify that MediaService implements the Service interface at compile time.
var _ Service = (*MediaService)(nil)

// GetMediaByID получает медиа по его ID
func (s *MediaService) GetMediaByID(ctx context.Context, req *media.GetMediaByIDRequest) (*media.Media, error) {
	if req == nil {
		return nil, fmt.Errorf("invalid request: nil pointer")
	}

	if req.Id <= 0 {
		return nil, fmt.Errorf("invalid ID: must be positive")
	}

	s.logger.InfoContext(ctx, "GetMediaByID called", "id", req.Id)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	m, err := s.repo.GetMediaByID(ctx, req.Id)
	if err != nil {
		return nil, s.handleError(ctx, "Failed to GetMediaByID", fmt.Errorf("failed to get media with id %d: %w", req.Id, err), "id", req.Id, "error", err)
	}

	s.logger.InfoContext(ctx, "GetMediaByID successful", "id", req.Id)
	return m, nil
}

// GetMediasByName получает медиа по названию
func (s *MediaService) GetMediasByName(ctx context.Context, req *media.GetMediasByNameRequest) (*media.MediaList, error) {
	if req == nil {
		return nil, fmt.Errorf("invalid request: nil pointer")
	}

	if req.Name == "" {
		return nil, fmt.Errorf("invalid name: cannot be empty")
	}

	s.logger.InfoContext(ctx, "GetMediasByName called", "name", req.Name)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// 1. Поиск медиа в Кинопоиске
	kinopoiskMedias, err := s.SearchKinopoisk(ctx, req.Name)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to search Kinopoisk", "name", req.Name, "error", err)
		return nil, s.handleError(ctx, "Failed to search Kinopoisk", fmt.Errorf("failed to search Kinopoisk: %w", err), "name", req.Name, "error", err)
	}

	// 2. Получение медиа из локальной базы данных
	localMedias, err := s.repo.GetMediasByNameFromRepo(ctx, req.Name)
	if err != nil {
		return nil, s.handleError(ctx, "Failed to GetMediasByName from DB", fmt.Errorf("failed to get medias by name %s from DB: %w", req.Name, err), "name", req.Name, "error", err)
	}

	// 3. Объединение результатов
	var mediaPointers []*media.Media
	mediaMap := make(map[int64]*media.Media)

	// Сначала добавляем локальные медиа
	for _, m := range localMedias {
		if _, exists := mediaMap[m.KinopoiskId]; !exists {
			mediaPointers = append(mediaPointers, m)
			mediaMap[m.KinopoiskId] = m
		}
	}

	// Сохраняем/обновляем и добавляем медиа из Кинопоиска
	for _, kpMedia := range kinopoiskMedias {
		// Проверяем, есть ли уже такое медиа в результатах
		if existingMedia, exists := mediaMap[kpMedia.KinopoiskId]; !exists {
			// Если нет в результатах - проверяем в БД
			dbMedia, err := s.repo.GetMediaByKinopoiskID(ctx, kpMedia.KinopoiskId)
			if err != nil {
				if errors.Is(err, repository.ErrMediaNotFound) {
					// Медиа нет в базе - сохраняем его
					s.logger.InfoContext(ctx, "Saving media from Kinopoisk", "kinopoiskID", kpMedia.KinopoiskId)
					// Заполняем поля времени перед сохранением
					kpMedia.CreatedAt = time.Now().Format(time.RFC3339)
					kpMedia.UpdatedAt = time.Now().Format(time.RFC3339)
					savedMedia, saveErr := s.repo.CreateMedia(ctx, kpMedia)
					if saveErr != nil {
						s.logger.ErrorContext(ctx, "Failed to save media from Kinopoisk", "kinopoiskID", kpMedia.KinopoiskId, "error", saveErr)
						mediaPointers = append(mediaPointers, kpMedia) // Даже если не удалось сохранить, добавляем в результаты
					} else {
						s.logger.InfoContext(ctx, "Media from Kinopoisk saved successfully", "kinopoiskID", kpMedia.KinopoiskId)
						mediaPointers = append(mediaPointers, savedMedia)
					}
				} else {
					s.logger.ErrorContext(ctx, "Failed to check existing media in DB", "kinopoiskID", kpMedia.KinopoiskId, "error", err)
					mediaPointers = append(mediaPointers, kpMedia) // Добавляем несмотря на ошибку
				}
				mediaMap[kpMedia.KinopoiskId] = kpMedia
			} else {
				// Медиа есть в БД, но не в текущих результатах
				if needsUpdate(dbMedia, kpMedia) {
					s.logger.InfoContext(ctx, "Updating media from Kinopoisk", "kinopoiskID", kpMedia.KinopoiskId)
					updatedMedia, updateErr := s.repo.UpdateMedia(ctx, kpMedia)
					if updateErr != nil {
						s.logger.ErrorContext(ctx, "Failed to update media from Kinopoisk", "kinopoiskID", kpMedia.KinopoiskId, "error", updateErr)
						mediaPointers = append(mediaPointers, dbMedia)
					} else {
						s.logger.InfoContext(ctx, "Media updated successfully", "kinopoiskID", kpMedia.KinopoiskId)
						mediaPointers = append(mediaPointers, updatedMedia)
					}
				} else {
					// Обновление не требуется, используем версию из БД
					mediaPointers = append(mediaPointers, dbMedia)
				}
				mediaMap[kpMedia.KinopoiskId] = kpMedia
			}
		} else {
			// Если медиа уже есть в результатах, проверяем, нужно ли обновить
			if needsUpdate(existingMedia, kpMedia) {
				// Обновляем существующее медиа данными из Кинопоиска
				s.logger.InfoContext(ctx, "Updating media with Kinopoisk data", "kinopoiskID", kpMedia.KinopoiskId)

				// Если у медиа есть ID в базе, обновляем через репозиторий
				if existingMedia.Id > 0 {
					kpMedia.Id = existingMedia.Id // Сохраняем ID из БД
					updatedMedia, updateErr := s.repo.UpdateMedia(ctx, kpMedia)
					if updateErr != nil {
						s.logger.ErrorContext(ctx, "Failed to update existing media", "kinopoiskID", kpMedia.KinopoiskId, "error", updateErr)
					} else {
						// Заменяем медиа в результатах
						for i, m := range mediaPointers {
							if m.KinopoiskId == updatedMedia.KinopoiskId {
								mediaPointers[i] = updatedMedia
								break
							}
						}
						mediaMap[kpMedia.KinopoiskId] = updatedMedia
					}
				}
			}
		}
	}

	// Формируем итоговый ответ
	result := &media.MediaList{
		Medias: mediaPointers,
	}

	s.logger.InfoContext(ctx, "GetMediasByName successful", "totalMedias", len(result.Medias))
	return result, nil
}

// SearchKinopoisk ищет медиа в Кинопоиске.
func (s *MediaService) SearchKinopoisk(ctx context.Context, name string) ([]*media.Media, error) {
	s.logger.InfoContext(ctx, "SearchKinopoisk called", "name", name)

	medias, err := s.kinopoiskClient.SearchByKeyword(ctx, name)
	if err != nil {
		return nil, s.handleError(ctx, "Failed to search Kinopoisk", fmt.Errorf("failed to search Kinopoisk: %w", err), "error", err)
	}

	// Возвращаем пустой срез вместо nil
	if medias == nil {
		return []*media.Media{}, nil
	}

	return medias, nil
}

// SaveMedia сохраняет новое медиа
func (s *MediaService) SaveMedia(ctx context.Context, req *media.SaveMediaRequest) (*media.Media, error) {
	s.logger.InfoContext(ctx, "SaveMedia called", "kinopoiskID", req.Media.KinopoiskId)

	// Базовая валидация входных данных
	if req.Media == nil {
		return nil, fmt.Errorf("invalid request: nil media")
	}

	if req.Media.KinopoiskId <= 0 {
		return nil, fmt.Errorf("invalid kinopoiskID: must be greater than 0")
	}

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

	// Базовая валидация входных данных
	if m.Id <= 0 {
		return nil, fmt.Errorf("invalid media ID: must be greater than 0")
	}

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

	s.logger.InfoContext(ctx, "Updating media", "mediaID", m.Id)
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

	// Базовая валидация входных данных
	if req.Id <= 0 {
		return nil, fmt.Errorf("invalid media ID: must be greater than 0")
	}

	resp, err := s.repo.DeleteMedia(ctx, req.Id)
	if err != nil {
		// Проверяем, является ли это ошибкой "запись не найдена"
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("media with id %d not found", req.Id)
		}
		return nil, s.handleError(ctx, "Failed to DeleteMedia", fmt.Errorf("failed to delete media with id %d: %w", req.Id, err), "id", req.Id, "error", err)
	}

	return resp, nil
}

// handleError централизованно обрабатывает ошибки с логированием
func (s *MediaService) handleError(ctx context.Context, message string, err error, args ...interface{}) error {
	s.logger.ErrorContext(ctx, message, args...)
	return fmt.Errorf("%s: %w", message, err)
}

// Улучшенная версия needsUpdate с правильным сравнением срезов
func needsUpdate(existingMedia, newMedia *media.Media) bool {
	if existingMedia == nil || newMedia == nil {
		return true
	}

	return existingMedia.KinopoiskId != newMedia.KinopoiskId ||
		existingMedia.Type != newMedia.Type ||
		existingMedia.NameEn != newMedia.NameEn ||
		existingMedia.NameRu != newMedia.NameRu ||
		existingMedia.Description != newMedia.Description ||
		existingMedia.Year != newMedia.Year ||
		existingMedia.Poster != newMedia.Poster ||
		!reflect.DeepEqual(existingMedia.Countries, newMedia.Countries) ||
		!reflect.DeepEqual(existingMedia.Genres, newMedia.Genres)
}
