package booking

import (
	"context"
	"errors"
	"testing"
)

func TestService_Create_reservesRequestedActivityAndSeats(t *testing.T) {
	// Arrange
	repo := NewMemoryRepository(testLogger())
	service := NewService(repo)
	command := CreateCommand{ActivityID: 2, Seats: 5}

	// Act
	created, err := service.Create(t.Context(), command)
	activity, lookupErr := repo.FindActivity(t.Context(), 2)

	// Assert
	if err != nil || lookupErr != nil {
		t.Fatalf("errors = %v, %v", err, lookupErr)
	}
	if created != (Booking{ID: 1, ActivityID: 2, Seats: 5}) {
		t.Errorf("booking = %+v; want activity 2 and five seats", created)
	}
	if activity.AvailableSeats != 7 {
		t.Errorf("remaining seats = %d; want 7", activity.AvailableSeats)
	}
}

func TestService_Create_preservesRepositoryErrors(t *testing.T) {
	tests := []struct {
		name    string
		command CreateCommand
		cancel  bool
		wantErr error
	}{
		{"missing activity", CreateCommand{ActivityID: 999, Seats: 2}, false, ErrActivityNotFound},
		{"insufficient seats", CreateCommand{ActivityID: 1, Seats: 4}, false, ErrInsufficientSeats},
		{"canceled", CreateCommand{ActivityID: 1, Seats: 2}, true, context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			service := NewService(NewMemoryRepository(testLogger()))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tt.cancel {
				cancel()
			}

			// Act
			created, err := service.Create(ctx, tt.command)

			// Assert
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v; want %v", err, tt.wantErr)
			}
			if created != (Booking{}) {
				t.Errorf("booking = %+v; want zero value", created)
			}
		})
	}
}
