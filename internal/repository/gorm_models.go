package repository

import (
	"fmt"
	"github.com/watchlist-kata/protos/media"
	"time"
)

// GormMedia представляет структуру данных для работы с GORM и базой данных
type GormMedia struct {
	ID          int64     `gorm:"primaryKey"`                // primary key
	KinopoiskID int64     `gorm:"unique"`                    // уникальный kinopoisk_id
	Type        string    `gorm:"type:varchar(20)"`          // Тип (movie или tv)
	NameEn      string    `gorm:"type:varchar(255)"`         // Название на английском
	NameRu      string    `gorm:"type:varchar(255)"`         // Название на русском
	Description string    `gorm:"type:text"`                 // Описание
	Year        string    `gorm:"type:varchar(4)"`           // Год выпуска
	Poster      string    `gorm:"type:varchar(255)"`         // URL постера
	Countries   string    `gorm:"type:varchar(255)"`         // Страны
	Genres      string    `gorm:"type:varchar(255)"`         // Жанры
	CreatedAt   time.Time `gorm:"default:CURRENT_TIMESTAMP"` // Дата создания
	UpdatedAt   time.Time `gorm:"default:CURRENT_TIMESTAMP"` // Дата обновления
}

// TableName возвращает имя таблицы для GORM
func (GormMedia) TableName() string {
	return "media" // Здесь указываем имя таблицы в базе данных
}

// Улучшенный convertGormMediaToProtoMedia
func convertGormMediaToProtoMedia(gormMedia *GormMedia) *media.Media {
	return &media.Media{
		Id:          gormMedia.ID,
		KinopoiskId: gormMedia.KinopoiskID,
		Type:        gormMedia.Type,
		NameEn:      gormMedia.NameEn,
		NameRu:      gormMedia.NameRu,
		Description: gormMedia.Description,
		Year:        gormMedia.Year,
		Poster:      gormMedia.Poster,
		Countries:   gormMedia.Countries,
		Genres:      gormMedia.Genres,
		CreatedAt:   gormMedia.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   gormMedia.UpdatedAt.Format(time.RFC3339),
	}
}

// Улучшенный convertProtoMediaToGormMedia
func convertProtoMediaToGormMedia(protoMedia *media.Media) GormMedia {
	createdAt, err := time.Parse(time.RFC3339, protoMedia.CreatedAt)
	if err != nil {
		fmt.Printf("Ошибка парсинга CreatedAt: %v\n", err)
		createdAt = time.Now()
	}

	updatedAt, err := time.Parse(time.RFC3339, protoMedia.UpdatedAt)
	if err != nil {
		fmt.Printf("Ошибка парсинга UpdatedAt: %v\n", err)
		updatedAt = time.Now()
	}

	return GormMedia{
		ID:          protoMedia.Id,
		KinopoiskID: protoMedia.KinopoiskId,
		Type:        protoMedia.Type,
		NameEn:      protoMedia.NameEn,
		NameRu:      protoMedia.NameRu,
		Description: protoMedia.Description,
		Year:        protoMedia.Year,
		Poster:      protoMedia.Poster,
		Countries:   protoMedia.Countries,
		Genres:      protoMedia.Genres,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
}
