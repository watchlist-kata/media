package repository

import (
	"context"
	"strings"

	"github.com/watchlist-kata/media/api"
	"gorm.io/gorm"
	"log/slog"
)

// Repository представляет собой структуру репозитория
type Repository struct {
	db     *gorm.DB
	logger *slog.Logger
}

// NewRepository создает новый экземпляр репозитория
func NewRepository(db *gorm.DB, logger *slog.Logger) *Repository {
	return &Repository{db: db, logger: logger}
}

// GetMediaByID получает медиа по его ID
func (r *Repository) GetMediaByID(id int64) (*api.Media, error) {
	var media api.Media
	if err := r.db.First(&media, id).Error; err != nil {
		r.logger.ErrorContext(context.Background(), "Failed to get media by ID", "id", id, "error", err)
		return nil, err
	}
	r.logger.InfoContext(context.Background(), "Media retrieved successfully", "id", id)
	return &media, nil
}

// GetMediasByName получает список медиа по названию
func (r *Repository) GetMediasByName(name string) ([]*api.Media, error) {
	var medias []*api.Media
	query := r.db.Where("lower(title) LIKE ? OR lower(title_ru) LIKE ?", "%"+strings.ToLower(name)+"%", "%"+strings.ToLower(name)+"%")
	result := query.Find(&medias)
	if result.Error != nil {
		r.logger.ErrorContext(context.Background(), "Failed to get medias by name", "name", name, "error", result.Error)
		return nil, result.Error
	}

	r.logger.InfoContext(context.Background(), "Medias retrieved successfully", "name", name, "count", len(medias))
	return medias, nil
}

// CreateMedia создает новое медиа
func (r *Repository) CreateMedia(media *api.Media) error {
	if err := r.db.Create(media).Error; err != nil {
		r.logger.ErrorContext(context.Background(), "Failed to create media", "media", media, "error", err)
		return err
	}

	r.logger.InfoContext(context.Background(), "Media created successfully", "media", media)
	return nil
}

// UpdateMedia обновляет существующее медиа
func (r *Repository) UpdateMedia(media *api.Media) error {
	if err := r.db.Save(media).Error; err != nil {
		r.logger.ErrorContext(context.Background(), "Failed to update media", "media", media, "error", err)
		return err
	}

	r.logger.InfoContext(context.Background(), "Media updated successfully", "media", media)
	return nil
}
