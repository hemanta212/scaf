package service

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/rlch/neogo"
	"github.com/rlch/scaf/cmd/crud-tui/db"
)

type User struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	CreatedAt int    `json:"createdAt,omitempty"`
}

type UserService struct {
	db neogo.Driver
}

func NewUserService(driver neogo.Driver) *UserService {
	return &UserService{db: driver}
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

func (s *UserService) Create(ctx context.Context, name, email string) (*User, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if !emailRegex.MatchString(email) {
		return nil, fmt.Errorf("invalid email format")
	}

	existing, err := db.GetUserByEmail(ctx, s.db, email)
	if err != nil {
		return nil, fmt.Errorf("checking email: %w", err)
	}
	if len(existing) > 0 {
		return nil, fmt.Errorf("email already exists")
	}

	count, err := db.CountUsers(ctx, s.db)
	if err != nil {
		return nil, fmt.Errorf("counting users: %w", err)
	}
	id := 1
	if len(count) > 0 {
		id = count[0] + 1
	}

	createdAt := int(time.Now().Unix())
	results, err := db.CreateUser(ctx, s.db, id, name, email, createdAt)
	if err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no user created")
	}

	return &User{
		ID:        results[0].ID,
		Name:      results[0].Name,
		Email:     results[0].Email,
		CreatedAt: createdAt,
	}, nil
}

func (s *UserService) GetByID(ctx context.Context, id int) (*User, error) {
	results, err := db.GetUserById(ctx, s.db, id)
	if err != nil {
		return nil, fmt.Errorf("getting user: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	return &User{
		ID:        results[0].ID,
		Name:      results[0].Name,
		Email:     results[0].Email,
		CreatedAt: results[0].CreatedAt,
	}, nil
}

func (s *UserService) List(ctx context.Context) ([]*User, error) {
	results, err := db.ListUsers(ctx, s.db)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}

	users := make([]*User, len(results))
	for i, r := range results {
		users[i] = &User{
			ID:    r.ID,
			Name:  r.Name,
			Email: r.Email,
		}
	}
	return users, nil
}

func (s *UserService) Update(ctx context.Context, id int, name, email string) (*User, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if !emailRegex.MatchString(email) {
		return nil, fmt.Errorf("invalid email format")
	}

	existing, err := db.GetUserById(ctx, s.db, id)
	if err != nil {
		return nil, fmt.Errorf("checking user: %w", err)
	}
	if len(existing) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	if existing[0].Email != email {
		dup, err := db.GetUserByEmail(ctx, s.db, email)
		if err != nil {
			return nil, fmt.Errorf("checking email: %w", err)
		}
		if len(dup) > 0 {
			return nil, fmt.Errorf("email already exists")
		}
	}

	results, err := db.UpdateUser(ctx, s.db, id, name, email)
	if err != nil {
		return nil, fmt.Errorf("updating user: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	return &User{
		ID:    results[0].ID,
		Name:  results[0].Name,
		Email: results[0].Email,
	}, nil
}

func (s *UserService) Delete(ctx context.Context, id int) error {
	existing, err := db.GetUserById(ctx, s.db, id)
	if err != nil {
		return fmt.Errorf("checking user: %w", err)
	}
	if len(existing) == 0 {
		return fmt.Errorf("user not found")
	}

	_, err = db.DeleteUser(ctx, s.db, id)
	if err != nil {
		return fmt.Errorf("deleting user: %w", err)
	}
	return nil
}
