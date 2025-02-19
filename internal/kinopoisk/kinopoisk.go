package kinopoisk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/watchlist-kata/protos/media"
)

// KPClient представляет собой клиента для работы с API Кинопоиска
type KPClient struct {
	apiKey string
	client *http.Client
	logger *slog.Logger
}

// NewKinopoiskClient создает новый клиент для Кинопоиска
func NewKinopoiskClient(apiKey string, logger *slog.Logger) (*KPClient, error) {
	client := &http.Client{
		Timeout: 10 * time.Second, // Устанавливаем таймаут
	}

	return &KPClient{
		apiKey: apiKey,
		client: client,
		logger: logger,
	}, nil
}

// SearchByKeyword выполняет запрос на поиск фильмов по ключевому слову
func (c *KPClient) SearchByKeyword(ctx context.Context, keyword string) ([]*media.Media, error) {
	// Construct the URL with both keyword and page parameters
	baseURL := "https://kinopoiskapiunofficial.tech/api/v2.1/films/search-by-keyword"
	queryParams := url.Values{}
	queryParams.Add("keyword", keyword)
	queryParams.Add("page", "1") // You can modify this as needed
	fullURL := baseURL + "?" + queryParams.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add the API key to the request header
	req.Header.Set("X-API-KEY", c.apiKey)
	req.Header.Set("accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request for keyword %s: %w", keyword, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to search Kinopoisk with keyword %s: status code %d", keyword, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Log the response body for debugging purposes
	c.logger.InfoContext(ctx, "Kinopoisk API Response", "body", string(body))

	var response struct {
		Films      []*film `json:"films"`
		Keyword    string  `json:"keyword"`
		PagesCount int     `json:"pagesCount"`
		SearchFilmsResult
	}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response JSON: %w", err)
	}

	var medias []*media.Media
	for _, film := range response.Films {
		medias = append(medias, &media.Media{
			KinopoiskId: int64(film.FilmId),
			NameRu:      film.NameRu,
			NameEn:      film.NameEn,
			Year:        film.Year,
			Description: film.Description,
			Type:        film.Type,
			Poster:      film.PosterUrl,
			Countries:   countriesToString(film.Countries),
			Genres:      genresToString(film.Genres),
		})
	}

	return medias, nil
}

type SearchFilmsResult struct {
	Total int `json:"total"`
	Items []film
}

type film struct {
	FilmId      int    `json:"filmId"`
	NameRu      string `json:"nameRu"`
	NameEn      string `json:"nameEn"`
	Type        string `json:"type"`
	Year        string `json:"year"`
	Description string `json:"description"`
	PosterUrl   string `json:"posterUrl"`
	Countries   []country
	Genres      []genre
}

type country struct {
	Country string `json:"country"`
}

type genre struct {
	Genre string `json:"genre"`
}

func countriesToString(countries []country) string {
	result := ""
	for i, c := range countries {
		result += c.Country
		if i < len(countries)-1 {
			result += ", "
		}
	}
	return result
}

func genresToString(genres []genre) string {
	result := ""
	for i, g := range genres {
		result += g.Genre
		if i < len(genres)-1 {
			result += ", "
		}
	}
	return result
}
