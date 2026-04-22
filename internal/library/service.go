package library

import (
	"context"
	"fmt"

	"github.com/ismd/linktheca/internal/core/content"
)

type StoreAPI interface {
	UpsertContent(ctx context.Context, p UpsertContentParams) (*ArticleContent, error)
	GetContentByURL(ctx context.Context, url string) (*ArticleContent, error)
	CreateItem(ctx context.Context, userID, contentID int64) (*Item, error)
	GetItemByID(ctx context.Context, userID, itemID int64) (*Item, error)
	ListItems(ctx context.Context, p ListParams) (*ListResult, error)
	UpdateItem(ctx context.Context, userID, itemID int64, p UpdateParams) (*Item, error)
	DeleteItem(ctx context.Context, userID, itemID int64) error
}

type Service struct {
	store     StoreAPI
	extractor content.Extractor
}

func NewService(store StoreAPI, extractor content.Extractor) *Service {
	return &Service{store: store, extractor: extractor}
}

// SaveURL extracts content from the URL and saves it to the user's library.
// If extraction fails, we still create a record with the URL and the fetch error.
func (s *Service) SaveURL(ctx context.Context, userID int64, rawURL string) (*Item, error) {
	var params UpsertContentParams

	article, extractErr := s.extractor.Extract(ctx, rawURL)
	if extractErr != nil {
		errMsg := extractErr.Error()
		params = UpsertContentParams{
			URL:        rawURL,
			FetchError: &errMsg,
		}
	} else {
		params = UpsertContentParams{
			URL:             article.URL,
			CanonicalURL:    nilIfEmpty(article.CanonicalURL),
			Title:           nilIfEmpty(article.Title),
			Byline:          nilIfEmpty(article.Byline),
			Excerpt:         nilIfEmpty(article.Excerpt),
			Text:            nilIfEmpty(article.Text),
			HTML:            nilIfEmpty(article.HTML),
			Lang:            nilIfEmpty(article.Lang),
			ReadingTimeSecs: nilIfZero(article.ReadingTimeSecs),
		}
	}

	ac, err := s.store.UpsertContent(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("upsert content: %w", err)
	}

	item, err := s.store.CreateItem(ctx, userID, ac.ID)
	if err != nil {
		return nil, err
	}

	return item, nil
}

// List returns paginated library items for a user
func (s *Service) List(ctx context.Context, p ListParams) (*ListResult, error) {
	return s.store.ListItems(ctx, p)
}

// GetByID returns a single library item with full content
func (s *Service) GetByID(ctx context.Context, userID, itemID int64) (*Item, error) {
	return s.store.GetItemByID(ctx, userID, itemID)
}

// Update partially updates a library item
func (s *Service) Update(ctx context.Context, userID, itemID int64, req UpdateRequest) (*Item, error) {
	p := UpdateParams{
		State:      req.State,
		IsFavorite: req.IsFavorite,
		Note:       req.Note,
	}

	if p.State != nil {
		switch *p.State {
		case "unread", "read", "archived":
			// valid
		default:
			return nil, fmt.Errorf("invalid state: %s", *p.State)
		}
	}

	return s.store.UpdateItem(ctx, userID, itemID, p)
}

// Delete removes a library item
func (s *Service) Delete(ctx context.Context, userID, itemID int64) error {
	return s.store.DeleteItem(ctx, userID, itemID)
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

func nilIfZero(n int) *int {
	if n == 0 {
		return nil
	}

	return &n
}
