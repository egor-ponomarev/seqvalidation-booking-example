package booking

const (
	fieldActivityID = "activityId"
	FieldSeats      = "seats"
)

// CreateRequest preserves missing fields until booking validation succeeds.
type CreateRequest struct {
	ActivityID *int `json:"activityId"`
	Seats      *int `json:"seats"`
}

type CreateCommand struct {
	ActivityID int
	Seats      int
}

type Activity struct {
	ID             int
	Name           string
	AvailableSeats int
}

func (r CreateRequest) value(field string) *int {
	switch field {
	case fieldActivityID:
		return r.ActivityID
	case FieldSeats:
		return r.Seats
	default:
		return nil
	}
}

type Booking struct {
	ID         int `json:"id"`
	ActivityID int `json:"activityId"`
	Seats      int `json:"seats"`
}
