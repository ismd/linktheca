package library

import "time"

// ArticleContent is the shared content cache entry
type ArticleContent struct {
	ID              int64      `json:"id"`
	URL             string     `json:"url"`
	CanonicalURL    *string    `json:"canonical_url,omitempty"`
	Title           *string    `json:"title,omitempty"`
	Byline          *string    `json:"byline,omitempty"`
	Excerpt         *string    `json:"excerpt,omitempty"`
	Text            *string    `json:"text,omitempty"`
	HTML            *string    `json:"html,omitempty"`
	Lang            *string    `json:"lang,omitempty"`
	ImageURL        *string    `json:"image_url,omitempty"`
	Favicon         *string    `json:"favicon,omitempty"`
	SiteName        *string    `json:"site_name,omitempty"`
	PublishedTime   *time.Time `json:"published_time,omitempty"`
	ModifiedTime    *time.Time `json:"modified_time,omitempty"`
	ReadingTimeSecs *int       `json:"reading_time_seconds,omitempty"`
	Image           *string    `json:"image,omitempty"`
	FetchedAt       time.Time  `json:"fetched_at"`
	FetchError      *string    `json:"fetch_error,omitempty"`
}

// Item is a user's saved article in the library
type Item struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"user_id"`
	ContentID  int64      `json:"content_id"`
	State      string     `json:"state"`
	IsFavorite bool       `json:"is_favorite"`
	Note       *string    `json:"note,omitempty"`
	SavedAt    time.Time  `json:"saved_at"`
	ReadAt     *time.Time `json:"read_at,omitempty"`

	// Joined from article_contents (populated in list/get queries)
	URL          string  `json:"url"`
	Title        *string `json:"title,omitempty"`
	Excerpt      *string `json:"excerpt,omitempty"`
	ReadTimeSecs *int    `json:"reading_time_seconds,omitempty"`
	Image        *string `json:"image,omitempty"`
}

// SaveRequest is the payload for POST /library
type SaveRequest struct {
	URL string `json:"url"`
}

// UpdateRequest is the payload for PATCH /library/:id
type UpdateRequest struct {
	State      *string `json:"state,omitempty"`
	IsFavorite *bool   `json:"is_favorite,omitempty"`
	Note       *string `json:"note,omitempty"`
}

// ListParams holds query parameters for GET /library
type ListParams struct {
	UserID   int64
	State    string // empty = all states
	Favorite *bool  // nil = don't filter
	Limit    int
	Offset   int
}

// ListResult holds the paginated response for GET /library
type ListResult struct {
	Items []Item `json:"items"`
	Total int    `json:"total"`
}

// UpdateParams holds the fields that can be changed via store.UpdateItem
type UpdateParams struct {
	State      *string
	IsFavorite *bool
	Note       *string
}

// ItemDetail is a library item with the full article content for reader view.
type ItemDetail struct {
	Item
	Content ArticleContent `json:"content"`
}
