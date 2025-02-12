package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/watchlist-kata/protos/media"
	"gorm.io/gorm"
	"log/slog"
)

// Repository определяет интерфейс для репозитория
type Repository interface {
	GetMediaByID(ctx context.Context, id int64) (*media.Media, error)
	GetMediasByNameFromRepo(ctx context.Context, name string) ([]*media.Media, error)
	CreateMedia(ctx context.Context, media *media.Media) (*media.Media, error)
	UpdateMedia(ctx context.Context, media *media.Media) (*media.Media, error)
}

// PostgresRepository представляет собой реализацию репозитория для PostgreSQL
type PostgresRepository struct {
	db     *gorm.DB
	logger *slog.Logger
}

// NewPostgresRepository создает новый экземпляр PostgresRepository
func NewPostgresRepository(db *gorm.DB, logger *slog.Logger) Repository {
	return &PostgresRepository{db: db, logger: logger}
}

// checkContextCancelled проверяет отмену контекста
func (r *PostgresRepository) checkContextCancelled(ctx context.Context, action string, params map[string]interface{}) error {
	select {
	case <-ctx.Done():
		r.logger.WarnContext(ctx, fmt.Sprintf("%s cancelled", action), params, "error", ctx.Err())
		return fmt.Errorf("%s cancelled: %w", action, ctx.Err())
	default:
		return nil
	}
}

// GetMediaByID получает медиа по его ID
func (r *PostgresRepository) GetMediaByID(ctx context.Context, id int64) (*media.Media, error) {
	if err := r.checkContextCancelled(ctx, "GetMediaByID", map[string]interface{}{"id": id}); err != nil {
		return nil, err
	}

	var m media.Media
	err := r.db.WithContext(ctx).First(&m, id).Error
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to get media by ID", "id", id, "error", err)
		return nil, fmt.Errorf("failed to get media with id %d: %w", id, err)
	}
	r.logger.InfoContext(ctx, "Media retrieved successfully", "id", id, "title", m.Title)
	return &m, nil
}

// GetMediasByNameFromRepo получает список медиа по названию
func (r *PostgresRepository) GetMediasByNameFromRepo(ctx context.Context, name string) ([]*media.Media, error) {
	if err := r.checkContextCancelled(ctx, "GetMediasByName", map[string]interface{}{"name": name}); err != nil {
		return nil, err
	}

	var medias []*media.Media
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
func (r *PostgresRepository) CreateMedia(ctx context.Context, media *media.Media) (*media.Media, error) {
	if err := r.checkContextCancelled(ctx, "CreateMedia", map[string]interface{}{"media": media}); err != nil {
		return nil, err
	}

	r.logger.InfoContext(ctx, "Creating media", "media_tmdb_id", media.TmdbId, "media_title", media.Title)
	result := r.db.WithContext(ctx).Create(media)
	if err := result.Error; err != nil {
		r.logger.ErrorContext(ctx, "Failed to create media", "media_tmdb_id", media.TmdbId, "media_title", media.Title, "error", err)
		return nil, fmt.Errorf("failed to create media with tmdb_id %d: %w", media.TmdbId, err)
	}

	r.logger.InfoContext(ctx, "Media created successfully", "media_tmdb_id", media.TmdbId, "media_title", media.Title)
	return media, nil
}

// UpdateMedia обновляет данные медиа по его ID
func (r *PostgresRepository) UpdateMedia(ctx context.Context, media *media.Media) (*media.Media, error) {
	// Проверка на отмену контекста
	if err := r.checkContextCancelled(ctx, "UpdateMedia", map[string]interface{}{"id": media.Id}); err != nil {
		return nil, err
	}

	// Поиск существующего медиа
	var existingMedia GormMedia
	if err := r.db.WithContext(ctx).First(&existingMedia, media.Id).Error; err != nil {
		r.logger.ErrorContext(ctx, "Failed to get media by ID", "id", media.Id, "error", err)
		return nil, fmt.Errorf("failed to get media with id %d: %w", media.Id, err)
	}

	// Логируем существующее медиа перед обновлением
	r.logger.InfoContext(ctx, "Existing media found for update", "id", media.Id, "existing_tmdb_id", existingMedia.TmdbID, "existing_title", existingMedia.Title)

	// Если tmdb_id в запросе отличается от tmdb_id в базе данных, возвращаем ошибку
	if media.TmdbId != existingMedia.TmdbID {
		r.logger.ErrorContext(ctx, "tmdb_id mismatch", "id", media.Id, "request_tmdb_id", media.TmdbId, "db_tmdb_id", existingMedia.TmdbID)
		return nil, fmt.Errorf("tmdb_id mismatch: cannot update media with a different tmdb_id")
	}

	// Обновляем поля медиа (кроме id и tmdb_id)
	updates := map[string]interface{}{
		"type":           media.Type,
		"title":          media.Title,
		"title_ru":       media.TitleRu,
		"description":    media.Description,
		"description_ru": media.DescriptionRu,
		"release_date":   media.ReleaseDate,
		"poster":         media.Poster,
	}

	// Логируем обновляемые поля
	r.logger.InfoContext(ctx, "Updating media fields", "id", media.Id, "updated_fields", updates)

	// Обновляем медиа в базе данных
	if err := r.db.WithContext(ctx).Model(&existingMedia).Updates(updates).Error; err != nil {
		r.logger.ErrorContext(ctx, "Failed to update media", "id", media.Id, "error", err)
		return nil, fmt.Errorf("failed to update media with id %d: %w", media.Id, err)
	}

	// Возвращаем обновлённое медиа напрямую из базы
	updatedMedia, err := r.GetMediaByID(ctx, media.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve updated media with id %d: %w", media.Id, err)
	}

	// Логируем успешное обновление
	r.logger.InfoContext(ctx, "Successfully updated media", "id", media.Id, "tmdb_id", updatedMedia.TmdbId, "updated_title", updatedMedia.Title)

	return updatedMedia, nil
}
