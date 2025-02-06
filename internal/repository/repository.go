package repository

import (
	"fmt"
	"github.com/watchlist-kata/media/api" // Модели из api/media.proto
	"github.com/watchlist-kata/media/internal/config"
	"gorm.io/driver/postgres" // Импортируем драйвер PostgreSQL
	"gorm.io/gorm"
	"strings"
)

// Repository представляет собой структуру репозитория
type Repository struct {
	db *gorm.DB
}

// NewRepository создает новый экземпляр репозитория
func NewRepository(cfg *config.Config) (*Repository, error) {
	db, err := gorm.Open(postgres.Open(cfg.DBURL), &gorm.Config{}) // Используем postgres.Open
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &Repository{db: db}, nil
}

// CreateMedia добавляет новое медиа в базу данных
func (r *Repository) CreateMedia(media *api.Media) error {
	result := r.db.Create(media)
	return result.Error
}

// GetMediaByID получает медиа по его ID
func (r *Repository) GetMediaByID(id int64) (*api.Media, error) {
	var media api.Media
	result := r.db.First(&media, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &media, nil
}

// UpdateMedia обновляет существующее медиа в базе данных
func (r *Repository) UpdateMedia(media *api.Media) error {
	result := r.db.Save(media)
	return result.Error
}

// GetMediasByName получает список медиа по названию
func (r *Repository) GetMediasByName(name string) ([]*api.Media, error) {
	var medias []*api.Media
	query := r.db.Where("lower(title) LIKE ? OR lower(title_ru) LIKE ?", "%"+strings.ToLower(name)+"%", "%"+strings.ToLower(name)+"%")
	result := query.Find(&medias)
	return medias, result.Error
}
