package httpapi

import "github.com/egor-ponomarev/seqvalidation-booking-example/internal/validation"

const (
	errorInvalidJSON        = "invalid_json"
	errorBookingUnavailable = "booking_unavailable"
)

type errorResponse struct {
	Error string `json:"error"`
}

type violationResponse struct {
	Violations []*validation.Violation `json:"violations"`
}
