package booking

import (
	"context"
	"fmt"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create reserves seats for input already checked by CreateValidator.Validate.
// Capacity can change after validation; the repository must recheck it atomically.
func (s *Service) Create(ctx context.Context, command CreateCommand) (Booking, error) {
	created, err := s.repo.CreateBooking(ctx, command.ActivityID, command.Seats)
	if err != nil {
		return Booking{}, fmt.Errorf("create booking: %w", err)
	}
	return created, nil
}
