package booking

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestMemoryRepository_FindActivity_results(t *testing.T) {
	tests := []struct {
		name    string
		id      int
		cancel  bool
		want    *Activity
		wantErr error
	}{
		{"found", 1, false, &Activity{ID: 1, Name: "Go meetup", AvailableSeats: 3}, nil},
		{"missing", 999, false, nil, ErrActivityNotFound},
		{"canceled", 1, true, nil, context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			repo := NewMemoryRepository(testLogger())
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tt.cancel {
				cancel()
			}

			// Act
			activity, err := repo.FindActivity(ctx, tt.id)

			// Assert
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v; want %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(activity, tt.want) {
				t.Errorf("activity = %+v; want %+v", activity, tt.want)
			}
		})
	}
}

func TestMemoryRepository_FindActivity_returnsIndependentSnapshot(t *testing.T) {
	// Arrange
	repo := NewMemoryRepository(testLogger())
	snapshot, err := repo.FindActivity(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}

	// Act
	snapshot.Name = "changed"
	snapshot.AvailableSeats = 100
	activity, err := repo.FindActivity(t.Context(), 1)

	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if *activity != (Activity{ID: 1, Name: "Go meetup", AvailableSeats: 3}) {
		t.Errorf("stored activity changed through snapshot: %+v", activity)
	}
}

func TestMemoryRepository_CreateBooking_reservesAndRecordsSeats(t *testing.T) {
	// Arrange
	repo := NewMemoryRepository(testLogger())

	// Act
	first, firstErr := repo.CreateBooking(t.Context(), 1, 2)
	second, secondErr := repo.CreateBooking(t.Context(), 1, 1)
	activity, lookupErr := repo.FindActivity(t.Context(), 1)

	// Assert
	if firstErr != nil || secondErr != nil || lookupErr != nil {
		t.Fatalf("errors = %v, %v, %v", firstErr, secondErr, lookupErr)
	}
	if first != (Booking{ID: 1, ActivityID: 1, Seats: 2}) || second != (Booking{ID: 2, ActivityID: 1, Seats: 1}) {
		t.Errorf("bookings = %+v, %+v; want consecutive IDs and requested seats", first, second)
	}
	if activity.AvailableSeats != 0 {
		t.Errorf("remaining seats = %d; want 0", activity.AvailableSeats)
	}
	if len(repo.bookings) != 2 || repo.bookings[first.ID] != first || repo.bookings[second.ID] != second {
		t.Errorf("recorded bookings = %+v; want both returned bookings", repo.bookings)
	}
}

func TestMemoryRepository_CreateBooking_rejectionLeavesStateUnchanged(t *testing.T) {
	tests := []struct {
		name       string
		activityID int
		seats      int
		cancel     bool
		wantErr    error
	}{
		{"missing activity", 999, 2, false, ErrActivityNotFound},
		{"zero seats", 1, 0, false, ErrInvalidSeats},
		{"negative seats", 1, -1, false, ErrInvalidSeats},
		{"insufficient seats", 1, 4, false, ErrInsufficientSeats},
		{"canceled", 1, 2, true, context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			repo := NewMemoryRepository(testLogger())
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tt.cancel {
				cancel()
			}

			// Act
			created, err := repo.CreateBooking(ctx, tt.activityID, tt.seats)
			activity, lookupErr := repo.FindActivity(t.Context(), 1)

			// Assert
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v; want %v", err, tt.wantErr)
			}
			if created != (Booking{}) {
				t.Errorf("rejected booking = %+v; want zero value", created)
			}
			if lookupErr != nil || activity == nil || activity.AvailableSeats != 3 {
				t.Fatalf("activity = %+v, %v; want unchanged capacity", activity, lookupErr)
			}
			if len(repo.bookings) != 0 || repo.nextID != 1 {
				t.Errorf("bookings/next ID = %+v/%d; want empty/1", repo.bookings, repo.nextID)
			}
		})
	}
}

type mutexCheckingWriter struct {
	repo   *MemoryRepository
	locked bool
	writes int
}

func (w *mutexCheckingWriter) Write(data []byte) (int, error) {
	w.writes++
	if w.repo.mu.TryLock() {
		w.repo.mu.Unlock()
	} else {
		w.locked = true
	}
	return len(data), nil
}

func TestMemoryRepository_loggingDoesNotHoldMutex(t *testing.T) {
	tests := []struct {
		name string
		call func(context.Context, *MemoryRepository) error
	}{
		{"find activity", func(ctx context.Context, repo *MemoryRepository) error {
			_, err := repo.FindActivity(ctx, 1)
			return err
		}},
		{"create booking", func(ctx context.Context, repo *MemoryRepository) error {
			_, err := repo.CreateBooking(ctx, 1, 2)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			writer := &mutexCheckingWriter{}
			repo := NewMemoryRepository(slog.New(slog.NewTextHandler(writer, nil)))
			writer.repo = repo

			// Act
			err := tt.call(t.Context(), repo)

			// Assert
			if err != nil {
				t.Fatal(err)
			}
			if writer.writes == 0 {
				t.Fatal("operation did not log")
			}
			if writer.locked {
				t.Error("logger called while repository mutex was held")
			}
		})
	}
}
