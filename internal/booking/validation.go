package booking

import (
	"context"
	"errors"
	"fmt"

	"github.com/egor-ponomarev/seqvalidation-booking-example/internal/validation"

	"github.com/egor-ponomarev/seqvalidation"
)

const CodeInsufficientSeats = "insufficient_seats"

func (v *CreateValidator) buildRules() seqvalidation.Rules[string, createRequestValidationContext, *validation.Violation] {
	return seqvalidation.Sequence(
		seqvalidation.Group(
			seqvalidation.Field(fieldActivityID, required),
			seqvalidation.Field(FieldSeats, required),
		).Named(validation.CodeRequired),
		seqvalidation.Group(
			seqvalidation.Field(fieldActivityID, positive),
			seqvalidation.Field(FieldSeats, positive),
		).Named("values"),
		seqvalidation.Group(
			seqvalidation.Field(fieldActivityID, v.activityExists),
		).Named("exists"),
		seqvalidation.Group(
			seqvalidation.Field(FieldSeats, v.seatsAvailable),
		).Named("availability"),
	)
}

type createRequestValidationContext struct {
	request CreateRequest

	activityLoaded bool
	activity       *Activity
	activityErr    error
}

func (v *CreateValidator) getActivity(ctx context.Context, s *createRequestValidationContext) (*Activity, error) {
	// The flag also caches missing activities and errors for this request.
	if !s.activityLoaded {
		s.activity, s.activityErr = v.repo.FindActivity(ctx, *s.request.ActivityID)
		s.activityLoaded = true
	}

	return s.activity, s.activityErr
}

func required(_ context.Context, field string, s *createRequestValidationContext) ([]*validation.Violation, error) {
	if s.request.value(field) == nil {
		return []*validation.Violation{{Field: field, Code: validation.CodeRequired}}, nil
	}

	return nil, nil
}

func positive(_ context.Context, field string, s *createRequestValidationContext) ([]*validation.Violation, error) {
	if *s.request.value(field) <= 0 {
		return []*validation.Violation{{Field: field, Code: validation.CodePositive}}, nil
	}

	return nil, nil
}

func (v *CreateValidator) activityExists(ctx context.Context, field string, s *createRequestValidationContext) ([]*validation.Violation, error) {
	_, err := v.getActivity(ctx, s)
	if errors.Is(err, ErrActivityNotFound) {
		return []*validation.Violation{{Field: field, Code: validation.CodeNotFound}}, nil
	}
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (v *CreateValidator) seatsAvailable(ctx context.Context, field string, s *createRequestValidationContext) ([]*validation.Violation, error) {
	found, err := v.getActivity(ctx, s)
	if err != nil {
		return nil, err
	}
	if *s.request.Seats > found.AvailableSeats {
		return []*validation.Violation{{Field: field, Code: CodeInsufficientSeats}}, nil
	}

	return nil, nil
}

// CreateValidator checks booking requests using fresh state for each call.
type CreateValidator struct {
	rules seqvalidation.Rules[string, createRequestValidationContext, *validation.Violation]
	repo  Repository
}

func NewCreateValidator(repo Repository) (*CreateValidator, error) {
	v := &CreateValidator{repo: repo}
	v.rules = v.buildRules()
	if err := v.rules.Check(); err != nil {
		return nil, fmt.Errorf("check booking rules: %w", err)
	}
	return v, nil
}

func (v *CreateValidator) Validate(ctx context.Context, request CreateRequest) ([]*validation.Violation, error) {
	state := &createRequestValidationContext{request: request}
	return v.rules.Validate(ctx, state)
}
