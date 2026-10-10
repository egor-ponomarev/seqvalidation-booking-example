package booking

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/egor-ponomarev/seqvalidation-booking-example/internal/validation"
)

type recordedRepository struct {
	source     Repository
	lookupErr  error
	ids        []int
	writeCalls int
}

func (r *recordedRepository) FindActivity(ctx context.Context, id int) (*Activity, error) {
	r.ids = append(r.ids, id)
	if r.lookupErr != nil {
		return nil, r.lookupErr
	}
	return r.source.FindActivity(ctx, id)
}

func (r *recordedRepository) CreateBooking(ctx context.Context, id, seats int) (Booking, error) {
	r.writeCalls++
	return r.source.CreateBooking(ctx, id, seats)
}

func intPointer(value int) *int {
	return &value
}

func testCreateValidator(t testing.TB, repo Repository) *CreateValidator {
	t.Helper()
	validator, err := NewCreateValidator(repo)
	if err != nil {
		t.Fatal(err)
	}
	return validator
}

func TestCreateValidator_Validate_rulesAndLookupCaching(t *testing.T) {
	tests := []struct {
		name    string
		request CreateRequest
		want    []*validation.Violation
		ids     []int
	}{
		{"missing fields", CreateRequest{}, []*validation.Violation{{Field: "activityId", Code: "required"}, {Field: "seats", Code: "required"}}, nil},
		{"required before positive", CreateRequest{nil, intPointer(0)}, []*validation.Violation{{Field: "activityId", Code: "required"}}, nil},
		{"missing seats", CreateRequest{intPointer(1), nil}, []*validation.Violation{{Field: "seats", Code: "required"}}, nil},
		{"nonpositive values", CreateRequest{intPointer(0), intPointer(-1)}, []*validation.Violation{{Field: "activityId", Code: "positive"}, {Field: "seats", Code: "positive"}}, nil},
		{"negative activity", CreateRequest{intPointer(-1), intPointer(2)}, []*validation.Violation{{Field: "activityId", Code: "positive"}}, nil},
		{"zero seats", CreateRequest{intPointer(1), intPointer(0)}, []*validation.Violation{{Field: "seats", Code: "positive"}}, nil},
		{"missing activity", CreateRequest{intPointer(999), intPointer(2)}, []*validation.Violation{{Field: "activityId", Code: "not_found"}}, []int{999}},
		{"insufficient seats", CreateRequest{intPointer(1), intPointer(4)}, []*validation.Violation{{Field: "seats", Code: "insufficient_seats"}}, []int{1}},
		{"valid request", CreateRequest{intPointer(1), intPointer(2)}, nil, []int{1}},
		{"exact capacity", CreateRequest{intPointer(1), intPointer(3)}, nil, []int{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			repo := &recordedRepository{source: NewMemoryRepository(testLogger())}
			validator := testCreateValidator(t, repo)

			// Act
			violations, err := validator.Validate(t.Context(), tt.request)

			// Assert
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(violations, tt.want) {
				t.Errorf("violations = %+v; want %+v", violations, tt.want)
			}
			if !reflect.DeepEqual(repo.ids, tt.ids) {
				t.Errorf("loaded activity IDs = %v; want %v", repo.ids, tt.ids)
			}
			if repo.writeCalls != 0 {
				t.Errorf("validation performed %d writes", repo.writeCalls)
			}
		})
	}
}

func TestCreateValidator_Validate_repositoryErrors(t *testing.T) {
	lookupErr := errors.New("database unavailable")
	tests := []struct {
		name    string
		err     error
		want    []*validation.Violation
		wantErr error
	}{
		{"wrapped not found", fmt.Errorf("lookup: %w", ErrActivityNotFound), []*validation.Violation{{Field: "activityId", Code: "not_found"}}, nil},
		{"operational error", lookupErr, nil, lookupErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			repo := &recordedRepository{lookupErr: tt.err}
			validator := testCreateValidator(t, repo)
			request := CreateRequest{ActivityID: intPointer(1), Seats: intPointer(2)}

			// Act
			violations, err := validator.Validate(t.Context(), request)

			// Assert
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v; want %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(violations, tt.want) {
				t.Errorf("violations = %+v; want %+v", violations, tt.want)
			}
			if !reflect.DeepEqual(repo.ids, []int{1}) || repo.writeCalls != 0 {
				t.Errorf("lookups/writes = %v/%d; want [1]/0", repo.ids, repo.writeCalls)
			}
		})
	}
}

func TestCreateValidator_Validate_refreshesSnapshotBetweenRequests(t *testing.T) {
	// Arrange
	store := NewMemoryRepository(testLogger())
	repo := &recordedRepository{source: store}
	validator := testCreateValidator(t, repo)
	request := CreateRequest{ActivityID: intPointer(1), Seats: intPointer(2)}

	// Act
	first, firstErr := validator.Validate(t.Context(), request)
	_, writeErr := store.CreateBooking(t.Context(), 1, 2)
	second, secondErr := validator.Validate(t.Context(), request)

	// Assert
	if firstErr != nil || writeErr != nil || secondErr != nil {
		t.Fatalf("errors = %v, %v, %v", firstErr, writeErr, secondErr)
	}
	if len(first) != 0 {
		t.Errorf("first violations = %+v; want none", first)
	}
	want := []*validation.Violation{{Field: "seats", Code: "insufficient_seats"}}
	if !reflect.DeepEqual(second, want) {
		t.Errorf("second violations = %+v; want %+v", second, want)
	}
	if !reflect.DeepEqual(repo.ids, []int{1, 1}) {
		t.Errorf("lookups = %v; want [1 1]", repo.ids)
	}
}

func TestCreateValidator_Validate_canceledContextSkipsLookup(t *testing.T) {
	// Arrange
	repo := &recordedRepository{source: NewMemoryRepository(testLogger())}
	validator := testCreateValidator(t, repo)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	request := CreateRequest{ActivityID: intPointer(1), Seats: intPointer(2)}

	// Act
	violations, err := validator.Validate(ctx, request)

	// Assert
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v; want canceled", err)
	}
	if len(violations) != 0 || len(repo.ids) != 0 || repo.writeCalls != 0 {
		t.Errorf("violations/lookups/writes = %+v/%v/%d; want none", violations, repo.ids, repo.writeCalls)
	}
}
