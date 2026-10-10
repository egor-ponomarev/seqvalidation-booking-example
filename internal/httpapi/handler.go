package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/egor-ponomarev/seqvalidation-booking-example/internal/booking"
	"github.com/gin-gonic/gin"
)

const bookingsPath = "/bookings"

func NewRouter(service *booking.Service, validator *booking.CreateValidator, logger *slog.Logger) http.Handler {
	handler := &bookingHandler{validator: validator, service: service, logger: logger}
	router := gin.New()
	router.Use(gin.Recovery())
	router.POST(bookingsPath, handler.create)
	return router
}
