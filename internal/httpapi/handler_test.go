package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/egor-ponomarev/seqvalidation-booking-example/internal/booking"
)

type recordedRepository struct {
	source     booking.Repository
	ids        []int
	lookupErr  error
	writeErr   error
	writeCalls int
}

func (r *recordedRepository) FindActivity(ctx context.Context, id int) (*booking.Activity, error) {
	r.ids = append(r.ids, id)
	if r.lookupErr != nil {
		return nil, r.lookupErr
	}
	return r.source.FindActivity(ctx, id)
}

func (r *recordedRepository) CreateBooking(ctx context.Context, id, seats int) (booking.Booking, error) {
	r.writeCalls++
	if r.writeErr != nil {
		return booking.Booking{}, r.writeErr
	}
	return r.source.CreateBooking(ctx, id, seats)
}

func testRouter(t testing.TB, repo booking.Repository, logger *slog.Logger) http.Handler {
	t.Helper()
	validator, err := booking.NewCreateValidator(repo)
	if err != nil {
		t.Fatal(err)
	}
	return NewRouter(booking.NewService(repo), validator, logger)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestBookingHandler_create_requests(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		status   int
		response string
		ids      []int
	}{
		{"missing fields", `{}`, 422, `{"violations":[{"field":"activityId","code":"required"},{"field":"seats","code":"required"}]}`, nil},
		{"invalid values", `{"activityId":-1,"seats":0}`, 422, `{"violations":[{"field":"activityId","code":"positive"},{"field":"seats","code":"positive"}]}`, nil},
		{"unknown activity", `{"activityId":999,"seats":2}`, 422, `{"violations":[{"field":"activityId","code":"not_found"}]}`, []int{999}},
		{"not enough seats", `{"activityId":1,"seats":5}`, 422, `{"violations":[{"field":"seats","code":"insufficient_seats"}]}`, []int{1}},
		{"valid request", `{"activityId":1,"seats":2}`, 201, `{"id":1,"activityId":1,"seats":2}`, []int{1}},
		{"exact capacity", `{"activityId":1,"seats":3}`, 201, `{"id":1,"activityId":1,"seats":3}`, []int{1}},
		{"second activity", `{"activityId":2,"seats":10}`, 201, `{"id":1,"activityId":2,"seats":10}`, []int{2}},
		{"null fields", `{"activityId":null,"seats":null}`, 422, `{"violations":[{"field":"activityId","code":"required"},{"field":"seats","code":"required"}]}`, nil},
		{"zero activity", `{"activityId":0,"seats":2}`, 422, `{"violations":[{"field":"activityId","code":"positive"}]}`, nil},
		{"negative seats", `{"activityId":1,"seats":-2}`, 422, `{"violations":[{"field":"seats","code":"positive"}]}`, nil},
		{"missing seats", `{"activityId":1}`, 422, `{"violations":[{"field":"seats","code":"required"}]}`, nil},
		{"missing activity", `{"seats":2}`, 422, `{"violations":[{"field":"activityId","code":"required"}]}`, nil},
		{"malformed JSON", `{"activityId":`, 400, `{"error":"invalid_json"}`, nil},
		{"wrong type", `{"activityId":"one","seats":2}`, 400, `{"error":"invalid_json"}`, nil},
		{"empty body", ``, 400, `{"error":"invalid_json"}`, nil},
		{"trailing garbage", `{"activityId":1,"seats":2} broken`, 400, `{"error":"invalid_json"}`, nil},
		{"second JSON value", `{"activityId":1,"seats":2} {}`, 400, `{"error":"invalid_json"}`, nil},
		{"oversized trailing padding", `{"activityId":1,"seats":2}` + strings.Repeat(" ", 1<<20), 400, `{"error":"invalid_json"}`, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			repo := &recordedRepository{source: booking.NewMemoryRepository(testLogger())}
			router := testRouter(t, repo, testLogger())
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/bookings", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()

			// Act
			router.ServeHTTP(response, request)

			// Assert
			if response.Code != tt.status {
				t.Fatalf("status = %d; want %d; body = %s", response.Code, tt.status, response.Body)
			}
			assertJSON(t, response.Body.Bytes(), []byte(tt.response))
			wantWrites := 0
			if tt.status == 201 {
				wantWrites = 1
			}
			if repo.writeCalls != wantWrites {
				t.Errorf("writes = %d; want %d", repo.writeCalls, wantWrites)
			}
			if !reflect.DeepEqual(repo.ids, tt.ids) {
				t.Errorf("loaded activity IDs = %v; want %v", repo.ids, tt.ids)
			}
		})
	}
}

func assertJSON(t *testing.T, got, want []byte) {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Errorf("response = %s; want %s", got, want)
	}
}

func TestBookingHandler_create_repositoryErrors(t *testing.T) {
	privateErr := errors.New("private database address")
	tests := []struct {
		name       string
		lookupErr  error
		writeErr   error
		status     int
		response   string
		wantWrites int
		loggedErr  error
	}{
		{"lookup failure", privateErr, nil, 500, `{"error":"booking_unavailable"}`, 0, privateErr},
		{"write failure", nil, privateErr, 500, `{"error":"booking_unavailable"}`, 1, privateErr},
		{"missing activity", fmt.Errorf("lookup: %w", booking.ErrActivityNotFound), nil, 422, `{"violations":[{"field":"activityId","code":"not_found"}]}`, 0, nil},
		{"capacity changed", nil, fmt.Errorf("reserve: %w", booking.ErrInsufficientSeats), 422, `{"violations":[{"field":"seats","code":"insufficient_seats"}]}`, 1, nil},
		{"activity disappeared", nil, booking.ErrActivityNotFound, 500, `{"error":"booking_unavailable"}`, 1, booking.ErrActivityNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, nil))
			repo := &recordedRepository{
				source:    booking.NewMemoryRepository(testLogger()),
				lookupErr: tt.lookupErr,
				writeErr:  tt.writeErr,
			}
			router := testRouter(t, repo, logger)
			response := httptest.NewRecorder()
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/bookings", strings.NewReader(`{"activityId":1,"seats":2}`))
			request.Header.Set("Content-Type", "application/json")

			// Act
			router.ServeHTTP(response, request)

			// Assert
			if response.Code != tt.status {
				t.Fatalf("status = %d; want %d; body = %s", response.Code, tt.status, response.Body)
			}
			assertJSON(t, response.Body.Bytes(), []byte(tt.response))
			if repo.writeCalls != tt.wantWrites {
				t.Errorf("writes = %d; want %d", repo.writeCalls, tt.wantWrites)
			}
			if tt.loggedErr != nil && !strings.Contains(logs.String(), tt.loggedErr.Error()) {
				t.Errorf("missing error diagnostics: %s", &logs)
			}
			if tt.loggedErr == nil && logs.Len() != 0 {
				t.Errorf("validation violation logged as an operational error: %s", &logs)
			}
		})
	}
}
