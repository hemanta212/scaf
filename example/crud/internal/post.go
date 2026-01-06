package internal

import (
	"context"
	"fmt"
	"time"

	"github.com/rlch/neogo"
)

type PostService struct {
	db neogo.Driver
}

func NewPostService(driver neogo.Driver) *PostService {
	return &PostService{db: driver}
}

func (s *PostService) Create(ctx context.Context, title, content string, authorID int) (*Post, error) {
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}

	count, err := CountPosts(ctx, s.db)
	if err != nil {
		return nil, fmt.Errorf("counting posts: %w", err)
	}
	id := 1
	if len(count) > 0 {
		id = count[0] + 1
	}

	createdAt := int(time.Now().Unix())
	results, err := CreatePost(ctx, s.db, authorID, id, title, content, createdAt)
	if err != nil {
		return nil, fmt.Errorf("creating post: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no post created")
	}

	return &Post{
		ID:        results[0].ID,
		Title:     results[0].Title,
		Content:   results[0].Content,
		CreatedAt: createdAt,
	}, nil
}

func (s *PostService) GetByID(ctx context.Context, id int) (*Post, error) {
	results, err := GetPostById(ctx, s.db, id)
	if err != nil {
		return nil, fmt.Errorf("getting post: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("post not found")
	}

	return &Post{
		ID:         results[0].ID,
		Title:      results[0].Title,
		Content:    results[0].Content,
		AuthorName: results[0].AuthorName,
	}, nil
}

func (s *PostService) List(ctx context.Context) ([]*Post, error) {
	results, err := ListPosts(ctx, s.db)
	if err != nil {
		return nil, fmt.Errorf("listing posts: %w", err)
	}

	posts := make([]*Post, len(results))
	for i, r := range results {
		posts[i] = &Post{
			ID:         r.ID,
			Title:      r.Title,
			AuthorName: r.AuthorName,
		}
	}
	return posts, nil
}

func (s *PostService) GetByAuthor(ctx context.Context, authorID int) ([]*Post, error) {
	results, err := GetPostsByAuthor(ctx, s.db, authorID)
	if err != nil {
		return nil, fmt.Errorf("getting posts: %w", err)
	}

	posts := make([]*Post, len(results))
	for i, r := range results {
		posts[i] = &Post{
			ID:      r.ID,
			Title:   r.Title,
			Content: r.Content,
		}
	}
	return posts, nil
}

func (s *PostService) Update(ctx context.Context, id int, title, content string) (*Post, error) {
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}

	results, err := UpdatePost(ctx, s.db, id, title, content)
	if err != nil {
		return nil, fmt.Errorf("updating post: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("post not found")
	}

	return &Post{
		ID:      results[0].ID,
		Title:   results[0].Title,
		Content: results[0].Content,
	}, nil
}

func (s *PostService) Delete(ctx context.Context, id int) error {
	_, err := DeletePost(ctx, s.db, id)
	if err != nil {
		return fmt.Errorf("deleting post: %w", err)
	}
	return nil
}
