package repository

import (
	"context"

	"github.com/watchlist-kata/media/api"
)

// Repository определяет интерфейс для репозитория
type Repository interface {
	GetMediaByID(ctx context.Context, id int64) (*api.Media, error)
	GetMediasByName(ctx context.Context, name string) ([]*api.Media, error)
	CreateMedia(ctx context.Context, media *api.Media) error
	UpdateMedia(ctx context.Context, media *api.Media) error
}
