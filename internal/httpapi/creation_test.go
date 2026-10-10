package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egor-ponomarev/seqvalidation-booking-example/internal/booking"
)

func TestBookingHandler_create_repeatedRequestsConsumeSeats(t *testing.T) {
	// Arrange
	store := booking.NewMemoryRepository(testLogger())
	repo := &recordedRepository{source: store}
	router := testRouter(t, repo, testLogger())
	responses := make([]*httptest.ResponseRecorder, 2)

	// Act
	for i := range responses {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/bookings", strings.NewReader(`{"activityId":1,"seats":2}`))
		request.Header.Set("Content-Type", "application/json")
		responses[i] = httptest.NewRecorder()
		router.ServeHTTP(responses[i], request)
	}
	remaining, err := store.FindActivity(t.Context(), 1)

	// Assert
	if responses[0].Code != 201 || responses[1].Code != 422 {
		t.Fatalf("statuses = %d, %d; want 201, 422", responses[0].Code, responses[1].Code)
	}
	assertJSON(t, responses[0].Body.Bytes(), []byte(`{"id":1,"activityId":1,"seats":2}`))
	assertJSON(t, responses[1].Body.Bytes(), []byte(`{"violations":[{"field":"seats","code":"insufficient_seats"}]}`))
	if !reflect.DeepEqual(repo.ids, []int{1, 1}) {
		t.Errorf("loaded activity IDs = %v; want [1 1]", repo.ids)
	}
	if repo.writeCalls != 1 {
		t.Errorf("writes = %d; want 1", repo.writeCalls)
	}
	if err != nil || remaining == nil || remaining.AvailableSeats != 1 {
		t.Fatalf("remaining = %v, %v; want one seat", remaining, err)
	}
}

type snapshotBarrierRepository struct {
	source  booking.Repository
	loaded  chan struct{}
	release chan struct{}
}

func (r *snapshotBarrierRepository) FindActivity(ctx context.Context, id int) (*booking.Activity, error) {
	activity, err := r.source.FindActivity(ctx, id)
	select {
	case r.loaded <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-r.release:
		return activity, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (r *snapshotBarrierRepository) CreateBooking(ctx context.Context, id, seats int) (booking.Booking, error) {
	return r.source.CreateBooking(ctx, id, seats)
}

func TestBookingHandler_create_concurrentRequestsRecheckCapacity(t *testing.T) {
	// Arrange
	const requestCount = 16
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	store := booking.NewMemoryRepository(testLogger())
	repo := &snapshotBarrierRepository{
		source:  store,
		loaded:  make(chan struct{}, requestCount),
		release: make(chan struct{}),
	}
	router := testRouter(t, repo, testLogger())
	responses := make([]*httptest.ResponseRecorder, requestCount)
	var workers sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		workers.Wait()
	})

	// Act
	for i := range responses {
		workers.Go(func() {
			request := httptest.NewRequestWithContext(ctx, http.MethodPost, "/bookings", strings.NewReader(`{"activityId":1,"seats":2}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			responses[i] = response
		})
	}
	for range requestCount {
		select {
		case <-repo.loaded:
		case <-ctx.Done():
			t.Fatalf("requests did not reach the snapshot barrier: %v", ctx.Err())
		}
	}
	close(repo.release)
	workers.Wait()
	remaining, err := store.FindActivity(t.Context(), 1)

	// Assert
	created, rejected := 0, 0
	for _, response := range responses {
		switch response.Code {
		case 201:
			created++
			assertJSON(t, response.Body.Bytes(), []byte(`{"id":1,"activityId":1,"seats":2}`))
		case 422:
			rejected++
			assertJSON(t, response.Body.Bytes(), []byte(`{"violations":[{"field":"seats","code":"insufficient_seats"}]}`))
		default:
			t.Errorf("unexpected status: %d", response.Code)
		}
	}
	if created != 1 || rejected != 15 {
		t.Fatalf("created/rejected = %d/%d; want 1/15", created, rejected)
	}
	if err != nil || remaining == nil || remaining.AvailableSeats != 1 {
		t.Fatalf("remaining = %v, %v; want one seat", remaining, err)
	}
}
