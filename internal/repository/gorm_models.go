package repository

// GormMedia представляет структуру данных для работы с GORM и базой данных
type GormMedia struct {
	ID            int64  `gorm:"primaryKey"` // primary key
	TmdbID        int64  `gorm:"unique"`     // уникальный tmdb_id
	Type          string // Тип (Movie или Show)
	Title         string // Название на английском
	TitleRu       string // Название на русском
	Description   string // Описание на английском
	DescriptionRu string // Описание на русском
	ReleaseDate   string // Дата выпуска
	Poster        string // URL постера
}

// TableName возвращает имя таблицы для GORM
func (GormMedia) TableName() string {
	return "media" // Здесь указываем правильное имя таблицы
}
