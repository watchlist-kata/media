package tmdb

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/cyruzin/golang-tmdb"
	"github.com/watchlist-kata/media/internal/config"
	"github.com/watchlist-kata/protos/media"
)

// TMDBClient представляет собой клиент для работы с TMDB API.
type TMDBClient struct {
	client *tmdb.Client
	logger *slog.Logger
}

// NewTMDBClient создает новый экземпляр TMDBClient.
func NewTMDBClient(cfg *config.Config, logger *slog.Logger) (*TMDBClient, error) {
	// Инициализация клиента TMDB с использованием API ключа из конфигурации
	tmdbClient, err := tmdb.Init(cfg.TMDBAPIKey)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize TMDB client: %w", err)
	}

	return &TMDBClient{
		client: tmdbClient,
		logger: logger,
	}, nil
}

// GetSearchMultiContext ищет media в TMDB API с поддержкой контекста.
func (c *TMDBClient) GetSearchMultiContext(ctx context.Context, query string, options map[string]string) ([]*media.Media, error) {
	// Проверяем, не был ли отменен контекст
	select {
	case <-ctx.Done():
		c.logger.WarnContext(ctx, "GetSearchMultiContext cancelled", "query", query, "error", ctx.Err())
		return nil, fmt.Errorf("GetSearchMultiContext cancelled: %w", ctx.Err())
	default:
		// Продолжаем выполнение
	}

	// Вызываем оригинальный метод GetSearchMulti из библиотеки golang-tmdb с использованием контекста
	searchMulti, err := c.client.GetSearchMulti(query, options)
	if err != nil {
		c.logger.ErrorContext(ctx, "Failed to execute GetSearchMulti", "query", query, "error", err)
		return nil, fmt.Errorf("failed to execute GetSearchMulti: %w", err)
	}

	// Преобразуем результаты поиска в []*media.Media
	medias := convertSearchMultiResults(searchMulti)

	return medias, nil
}

// convertSearchMultiResults преобразует результаты поиска TMDB в []*media.Media.
func convertSearchMultiResults(searchMulti *tmdb.SearchMulti) []*media.Media {
	medias := make([]*media.Media, 0)
	for _, result := range searchMulti.Results {
		media := &media.Media{}
		// Обрабатываем типы медиа (movie, tv, person)
		switch result.MediaType {
		case "movie":
			media.Title = result.Title
			media.Description = result.Overview
			media.ReleaseDate = result.ReleaseDate
			// Сохраняем только имя файла постера, а не полный URL
			media.Poster = result.PosterPath
			media.TmdbId = result.ID
		case "tv":
			media.Title = result.Name
			media.Description = result.Overview
			media.ReleaseDate = result.FirstAirDate
			// Сохраняем только имя файла постера
			media.Poster = result.PosterPath
			media.TmdbId = result.ID
		case "person":
			// Игнорируем результаты поиска по персонам
			continue
		default:
			// Обрабатываем другие типы медиа, если необходимо
			continue
		}

		medias = append(medias, media)
	}

	return medias
}
