package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/watchlist-kata/media/api"
	"github.com/watchlist-kata/media/internal/repository"
	"gorm.io/gorm"
	"log/slog"
)

// Verify that PostgresRepository implements the Repository interface at compile time.
var _ repository.Repository = (*PostgresRepository)(nil)

// PostgresRepository представляет собой реализацию репозитория для PostgreSQL
type PostgresRepository struct {
	db     *gorm.DB
	logger *slog.Logger
}

// NewPostgresRepository создает новый экземпляр PostgresRepository
func NewPostgresRepository(db *gorm.DB, logger *slog.Logger) *PostgresRepository {
	return &PostgresRepository{db: db, logger: logger}
}

// GetMediaByID получает медиа по его ID
func (r *PostgresRepository) GetMediaByID(ctx context.Context, id int64) (*api.Media, error) {
	var media api.Media
	err := r.db.WithContext(ctx).First(&media, id).Error
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to get media by ID", "id", id, "error", err)
		return nil, fmt.Errorf("failed to get media with id %d: %w", id, err)
	}
	r.logger.InfoContext(ctx, "Media retrieved successfully", "id", id)
	return &media, nil
}

// GetMediasByName получает список медиа по названию
func (r *PostgresRepository) GetMediasByName(ctx context.Context, name string) ([]*api.Media, error) {
	var medias []*api.Media
	query := r.db.WithContext(ctx).Where("lower(title) LIKE ? OR lower(title_ru) LIKE ?", "%"+strings.ToLower(name)+"%", "%"+strings.ToLower(name)+"%")
	result := query.Find(&medias)
	if result.Error != nil {
		r.logger.ErrorContext(ctx, "Failed to get medias by name", "name", name, "error", result.Error)
		return nil, fmt.Errorf("failed to get medias with name %s: %w", name, result.Error)
	}

	r.logger.InfoContext(ctx, "Medias retrieved successfully", "name", name, "count", len(medias))
	return medias, nil
}

// CreateMedia создает новое медиа
func (r *PostgresRepository) CreateMedia(ctx context.Context, media *api.Media) error {
	err := r.db.WithContext(ctx).Create(media).Error
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to create media", "media", media, "error", err)
		return fmt.Errorf("failed to create media with tmdb_id %d: %w", media.TmdbId, err)
	}

	r.logger.InfoContext(ctx, "Media created successfully", "media", media)
	return nil
}

// UpdateMedia обновляет существующее медиа
func (r *PostgresRepository) UpdateMedia(ctx context.Context, media *api.Media) error {
	err := r.db.WithContext(ctx).Save(media).Error
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to update media", "media", media, "error", err)
		return fmt.Errorf("failed to update media with id %d: %w", media.Id, err)
	}

	r.logger.InfoContext(ctx, "Media updated successfully", "media", media)
	return nil
}
