package validation

const (
	CodeRequired = "required"
	CodePositive = "positive"
	CodeNotFound = "not_found"
)

type Violation struct {
	Field string `json:"field"`
	Code  string `json:"code"`
}
