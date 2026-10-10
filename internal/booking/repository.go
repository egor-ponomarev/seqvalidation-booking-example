package booking

import (
	"context"
	"errors"
)

var (
	ErrActivityNotFound  = errors.New("activity not found")
	ErrInsufficientSeats = errors.New("insufficient seats")
	ErrInvalidSeats      = errors.New("seats must be positive")
)

type Repository interface {
	// FindActivity returns an independent snapshot, or ErrActivityNotFound if the activity is missing.
	FindActivity(context.Context, int) (*Activity, error)

	// CreateBooking checks capacity, reserves seats and records a booking atomically.
	// Rejected requests must leave capacity and bookings unchanged.
	CreateBooking(context.Context, int, int) (Booking, error)
}
