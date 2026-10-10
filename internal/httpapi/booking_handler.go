package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/egor-ponomarev/seqvalidation-booking-example/internal/booking"
	"github.com/egor-ponomarev/seqvalidation-booking-example/internal/validation"
	"github.com/gin-gonic/gin"
)

type bookingHandler struct {
	validator *booking.CreateValidator
	service   *booking.Service
	logger    *slog.Logger
}

func (h *bookingHandler) create(c *gin.Context) {
	var request booking.CreateRequest
	if err := decodeJSON(c, &request); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: errorInvalidJSON})
		return
	}

	violations, err := h.validator.Validate(c.Request.Context(), request)
	if err != nil {
		h.logger.ErrorContext(c.Request.Context(), "booking validation failed", logKeyError, err)
		c.JSON(http.StatusInternalServerError, errorResponse{Error: errorBookingUnavailable})
		return
	}
	if len(violations) > 0 {
		c.JSON(http.StatusUnprocessableEntity, violationResponse{Violations: violations})
		return
	}

	command := booking.CreateCommand{
		ActivityID: *request.ActivityID,
		Seats:      *request.Seats,
	}
	created, err := h.service.Create(c.Request.Context(), command)
	if errors.Is(err, booking.ErrInsufficientSeats) {
		c.JSON(http.StatusUnprocessableEntity, violationResponse{
			Violations: []*validation.Violation{{Field: booking.FieldSeats, Code: booking.CodeInsufficientSeats}},
		})
		return
	}
	if err != nil {
		h.logger.ErrorContext(c.Request.Context(), "booking creation failed", logKeyError, err)
		c.JSON(http.StatusInternalServerError, errorResponse{Error: errorBookingUnavailable})
		return
	}
	c.JSON(http.StatusCreated, created)
}
