package internal

import (
	"context"
	"fmt"
	"time"

	"github.com/rlch/neogo"
)

type CommentService struct {
	db neogo.Driver
}

func NewCommentService(driver neogo.Driver) *CommentService {
	return &CommentService{db: driver}
}

func (s *CommentService) Create(ctx context.Context, text string, authorID, postID int) (*Comment, error) {
	if text == "" {
		return nil, fmt.Errorf("text is required")
	}

	count, err := CountCommentsByPost(ctx, s.db, postID)
	if err != nil {
		return nil, fmt.Errorf("counting comments: %w", err)
	}
	id := 1
	if len(count) > 0 {
		id = count[0] + 1000 // offset to avoid conflicts
	}

	createdAt := int(time.Now().Unix())
	results, err := CreateComment(ctx, s.db, authorID, postID, id, text, createdAt)
	if err != nil {
		return nil, fmt.Errorf("creating comment: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no comment created")
	}

	return &Comment{
		ID:        results[0].ID,
		Text:      results[0].Text,
		CreatedAt: createdAt,
	}, nil
}

func (s *CommentService) GetByID(ctx context.Context, id int) (*Comment, error) {
	results, err := GetCommentById(ctx, s.db, id)
	if err != nil {
		return nil, fmt.Errorf("getting comment: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("comment not found")
	}

	return &Comment{
		ID:         results[0].ID,
		Text:       results[0].Text,
		AuthorName: results[0].AuthorName,
		PostTitle:  results[0].PostTitle,
	}, nil
}

func (s *CommentService) GetByPost(ctx context.Context, postID int) ([]*Comment, error) {
	results, err := GetCommentsByPost(ctx, s.db, postID)
	if err != nil {
		return nil, fmt.Errorf("getting comments: %w", err)
	}

	comments := make([]*Comment, len(results))
	for i, r := range results {
		comments[i] = &Comment{
			ID:         r.ID,
			Text:       r.Text,
			AuthorName: r.AuthorName,
		}
	}
	return comments, nil
}

func (s *CommentService) GetByUser(ctx context.Context, userID int) ([]*Comment, error) {
	results, err := GetCommentsByUser(ctx, s.db, userID)
	if err != nil {
		return nil, fmt.Errorf("getting comments: %w", err)
	}

	comments := make([]*Comment, len(results))
	for i, r := range results {
		comments[i] = &Comment{
			ID:        r.ID,
			Text:      r.Text,
			PostTitle: r.PostTitle,
		}
	}
	return comments, nil
}

func (s *CommentService) Update(ctx context.Context, id int, text string) (*Comment, error) {
	if text == "" {
		return nil, fmt.Errorf("text is required")
	}

	results, err := UpdateComment(ctx, s.db, id, text)
	if err != nil {
		return nil, fmt.Errorf("updating comment: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("comment not found")
	}

	return &Comment{
		ID:   results[0].ID,
		Text: results[0].Text,
	}, nil
}

func (s *CommentService) Delete(ctx context.Context, id int) error {
	_, err := DeleteComment(ctx, s.db, id)
	if err != nil {
		return fmt.Errorf("deleting comment: %w", err)
	}
	return nil
}

func (s *CommentService) Reply(ctx context.Context, text string, authorID, parentID int) (*Comment, error) {
	if text == "" {
		return nil, fmt.Errorf("text is required")
	}

	count, err := CountReplies(ctx, s.db, parentID)
	if err != nil {
		return nil, fmt.Errorf("counting replies: %w", err)
	}
	id := parentID*1000 + 1
	if len(count) > 0 {
		id = parentID*1000 + count[0] + 1
	}

	createdAt := int(time.Now().Unix())
	results, err := CreateReply(ctx, s.db, authorID, parentID, id, text, createdAt)
	if err != nil {
		return nil, fmt.Errorf("creating reply: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no reply created")
	}

	return &Comment{
		ID:        results[0].ID,
		Text:      results[0].Text,
		CreatedAt: createdAt,
	}, nil
}

func (s *CommentService) GetReplies(ctx context.Context, commentID int) ([]*Comment, error) {
	results, err := GetReplies(ctx, s.db, commentID)
	if err != nil {
		return nil, fmt.Errorf("getting replies: %w", err)
	}

	comments := make([]*Comment, len(results))
	for i, r := range results {
		comments[i] = &Comment{
			ID:         r.ID,
			Text:       r.Text,
			AuthorName: r.AuthorName,
		}
	}
	return comments, nil
}
