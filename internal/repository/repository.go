package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/watchlist-kata/protos/media"
	"gorm.io/gorm"
)

// Repository определяет интерфейс для репозитория
type Repository interface {
	GetMediaByID(ctx context.Context, id int64) (*media.Media, error)
	GetMediasByNameFromRepo(ctx context.Context, name string) ([]*media.Media, error)
	CreateMedia(ctx context.Context, media *media.Media) (*media.Media, error)
	UpdateMedia(ctx context.Context, media *media.Media) (*media.Media, error)
	DeleteMedia(ctx context.Context, id int64) (*media.DeleteMediaResponse, error)
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

	var gormMedia GormMedia
	err := r.db.WithContext(ctx).First(&gormMedia, id).Error
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to get media by ID", "id", id, "error", err)
		return nil, fmt.Errorf("failed to get media with id %d: %w", id, err)
	}

	// Преобразование GormMedia в *media.Media
	m := convertGormMediaToProtoMedia(&gormMedia)

	r.logger.InfoContext(ctx, "Media retrieved successfully", "id", id, "name_en", m.NameEn)
	return m, nil
}

// GetMediasByNameFromRepo получает список медиа по названию
func (r *PostgresRepository) GetMediasByNameFromRepo(ctx context.Context, name string) ([]*media.Media, error) {
	if err := r.checkContextCancelled(ctx, "GetMediasByName", map[string]interface{}{"name": name}); err != nil {
		return nil, err
	}

	var gormMedias []GormMedia
	query := r.db.WithContext(ctx).Where("lower(name_en) LIKE ? OR lower(name_ru) LIKE ?", "%"+strings.ToLower(name)+"%", "%"+strings.ToLower(name)+"%")
	result := query.Find(&gormMedias)
	if result.Error != nil {
		r.logger.ErrorContext(ctx, "Failed to get medias by name", "name", name, "error", result.Error)
		return nil, fmt.Errorf("failed to get medias with name %s: %w", name, result.Error)
	}

	var medias []*media.Media
	for _, gormMedia := range gormMedias {
		m := convertGormMediaToProtoMedia(&gormMedia)
		medias = append(medias, m)
	}

	r.logger.InfoContext(ctx, "Medias retrieved successfully", "name", name, "count", len(medias))
	return medias, nil
}

// CreateMedia создает новое медиа
func (r *PostgresRepository) CreateMedia(ctx context.Context, media *media.Media) (*media.Media, error) {
	if err := r.checkContextCancelled(ctx, "CreateMedia", map[string]interface{}{"media": media}); err != nil {
		return nil, err
	}

	gormMedia := convertProtoMediaToGormMedia(media)

	r.logger.InfoContext(ctx, "Creating media", "media_kinopoisk_id", gormMedia.KinopoiskID, "media_name_en", gormMedia.NameEn)
	result := r.db.WithContext(ctx).Create(&gormMedia)
	if err := result.Error; err != nil {
		r.logger.ErrorContext(ctx, "Failed to create media", "media_kinopoisk_id", gormMedia.KinopoiskID, "media_name_en", gormMedia.NameEn, "error", err)
		return nil, fmt.Errorf("failed to create media with kinopoisk_id %d: %w", gormMedia.KinopoiskID, err)
	}

	createdMedia := convertGormMediaToProtoMedia(&gormMedia)

	r.logger.InfoContext(ctx, "Media created successfully", "media_kinopoisk_id", gormMedia.KinopoiskID, "media_name_en", gormMedia.NameEn)
	return createdMedia, nil
}

// UpdateMedia обновляет данные медиа по его ID
func (r *PostgresRepository) UpdateMedia(ctx context.Context, media *media.Media) (*media.Media, error) {
	if err := r.checkContextCancelled(ctx, "UpdateMedia", map[string]interface{}{"id": media.Id}); err != nil {
		return nil, err
	}

	var existingMedia GormMedia
	if err := r.db.WithContext(ctx).First(&existingMedia, media.Id).Error; err != nil {
		r.logger.ErrorContext(ctx, "Failed to get media by ID", "id", media.Id, "error", err)
		return nil, fmt.Errorf("failed to get media with id %d: %w", media.Id, err)
	}

	r.logger.InfoContext(ctx,
		"Existing media found for update",
		map[string]interface{}{
			"id":                    media.Id,
			"existing_kinopoisk_id": existingMedia.KinopoiskID,
			"existing_name_en":      existingMedia.NameEn,
			"existing_name_ru":      existingMedia.NameRu,
		})

	if media.KinopoiskId != existingMedia.KinopoiskID {
		r.logger.ErrorContext(ctx,
			"kinopoisk_id mismatch",
			map[string]interface{}{
				"id":                   media.Id,
				"request_kinopoisk_id": media.KinopoiskId,
				"db_kinopoisk_id":      existingMedia.KinopoiskID,
			})
		return nil,
			fmt.Errorf("kinopoisk_id mismatch: cannot update media with a different kinopoisk_id")
	}

	gormUpdates := convertProtoMediaToGormMedia(media)

	updates := map[string]interface{}{
		"type":        gormUpdates.Type,
		"name_en":     gormUpdates.NameEn,
		"name_ru":     gormUpdates.NameRu,
		"description": gormUpdates.Description,
		"year":        gormUpdates.Year,
		"poster":      gormUpdates.Poster,
		"countries":   gormUpdates.Countries,
		"genres":      gormUpdates.Genres,
	}

	r.logger.InfoContext(ctx, "Updating media fields", map[string]interface{}{
		"id":             media.Id,
		"updated_fields": updates,
	})

	if err := r.db.WithContext(ctx).Model(&existingMedia).Updates(updates).Error; err != nil {
		r.logger.ErrorContext(ctx, "Failed to update media", map[string]interface{}{
			"id":    media.Id,
			"error": err,
		})
		return nil, fmt.Errorf("failed to update media with id %d: %w", media.Id, err)
	}

	updatedMedia, err := r.GetMediaByID(ctx, media.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve updated media with id %d: %w", media.Id, err)
	}

	r.logger.InfoContext(ctx, "Successfully updated media", map[string]interface{}{
		"id":              updatedMedia.Id,
		"kinopoisk_id":    updatedMedia.KinopoiskId,
		"updated_name_en": updatedMedia.NameEn,
		"updated_name_ru": updatedMedia.NameRu,
	})

	return updatedMedia, nil
}

// DeleteMedia удаляет медиа по его ID
func (r *PostgresRepository) DeleteMedia(ctx context.Context, id int64) (*media.DeleteMediaResponse, error) {
	if err := r.checkContextCancelled(ctx, "DeleteMedia", map[string]interface{}{"id": id}); err != nil {
		return nil, err
	}

	var existingMedia GormMedia
	if err := r.db.WithContext(ctx).First(&existingMedia, id).Error; err != nil {
		r.logger.ErrorContext(ctx, "Failed to find media by ID", "id", id, "error", err)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &media.DeleteMediaResponse{Success: false}, nil // Если медиа не найдено, возвращаем успешный ответ, так как ничего удалять не нужно
		}
		return nil, fmt.Errorf("failed to find media with id %d: %w", id, err)
	}

	// Удаляем медиа из базы данных
	if err := r.db.WithContext(ctx).Delete(&existingMedia).Error; err != nil {
		r.logger.ErrorContext(ctx, "Failed to delete media", "id", id, "error", err)
		return nil, fmt.Errorf("failed to delete media with id %d: %w", id, err)
	}

	r.logger.InfoContext(ctx, "Successfully deleted media", map[string]interface{}{"id": id})

	return &media.DeleteMediaResponse{Success: true}, nil
}
