package booking

import (
	"context"
	"log/slog"
	"sync"
)

const logKeyActivityID = "activityId"

type MemoryRepository struct {
	mu         sync.Mutex
	activities map[int]Activity
	bookings   map[int]Booking
	nextID     int
	logger     *slog.Logger
}

func NewMemoryRepository(logger *slog.Logger) *MemoryRepository {
	return &MemoryRepository{
		activities: map[int]Activity{
			1: {ID: 1, Name: "Go meetup", AvailableSeats: 3},
			2: {ID: 2, Name: "Go workshop", AvailableSeats: 12},
		},
		bookings: make(map[int]Booking),
		nextID:   1,
		logger:   logger,
	}
}

func (m *MemoryRepository) FindActivity(ctx context.Context, id int) (*Activity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.logger.InfoContext(ctx, "loading activity", logKeyActivityID, id)

	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	found, exists := m.activities[id]
	if !exists {
		return nil, ErrActivityNotFound
	}
	return &found, nil
}

func (m *MemoryRepository) CreateBooking(ctx context.Context, activityID, seats int) (Booking, error) {
	if err := ctx.Err(); err != nil {
		return Booking{}, err
	}

	created, err := m.createBooking(ctx, activityID, seats)
	if err != nil {
		return Booking{}, err
	}

	m.logger.InfoContext(ctx, "booking created", "bookingId", created.ID, logKeyActivityID, activityID, "seats", seats)

	return created, nil
}

func (m *MemoryRepository) createBooking(ctx context.Context, activityID, seats int) (Booking, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return Booking{}, err
	}
	if seats <= 0 {
		return Booking{}, ErrInvalidSeats
	}

	current, exists := m.activities[activityID]
	if !exists {
		return Booking{}, ErrActivityNotFound
	}
	if seats > current.AvailableSeats {
		return Booking{}, ErrInsufficientSeats
	}

	created := Booking{ID: m.nextID, ActivityID: activityID, Seats: seats}
	current.AvailableSeats -= seats
	m.activities[activityID] = current
	m.bookings[created.ID] = created
	m.nextID++

	return created, nil
}
