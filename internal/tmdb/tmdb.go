package tmdb

import (
	"fmt"
	"github.com/cyruzin/golang-tmdb"
	"github.com/watchlist-kata/media/internal/config" // Правильный путь импорта
)

// InitTMDBClient инициализирует клиент TMDB с вашим API ключом.
func InitTMDBClient(cfg *config.Config) (*tmdb.Client, error) {
	tmdbClient, err := tmdb.Init(cfg.TMDBAPIKey)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize TMDB client: %w", err)
	}
	return tmdbClient, nil
}
