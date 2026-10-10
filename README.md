# Booking API with seqvalidation

A small Gin API that shows how to validate a request in ordered groups with
[seqvalidation](https://github.com/egor-ponomarev/seqvalidation).

Try five requests and watch when the API loads an activity. Missing fields and invalid
values stop before the lookup. Later checks share one loaded activity per request.

The API validates the request and creates a booking. A successful request reserves
seats in memory. Run the five requests in order on a fresh server; restart it to
reset the activities and bookings.

## Run

You need Go 1.25.0 or newer. Gin v1.12.0 requires this version; seqvalidation itself
supports Go 1.22. The first run downloads dependencies.

```sh
git clone https://github.com/egor-ponomarev/seqvalidation-booking-example.git
cd seqvalidation-booking-example
go run .
```

The server listens on `127.0.0.1:8080`. Stop it with Ctrl+C.

The commands above require both repositories to be public. At the time this
example was prepared, the public Go proxy could not fetch seqvalidation v0.1.0.
If you have both checkouts locally, use a workspace:

```sh
go work init . ../seqvalidation
go run .
```

The workspace files are local development files and are ignored by Git. No local
`replace` is committed in go.mod.

## Activities

The repository keeps two activities in memory:

| ID | Activity | Available seats |
| --- | --- | ---: |
| 1 | Go meetup | 3 |
| 2 | Go workshop | 12 |

No database or Docker setup is needed.

## Try five requests

Keep the server running and open another terminal. You can also run the same
requests from [requests.http](requests.http) in GoLand or an HTTP client extension.

### 1. Missing fields

```sh
curl -i http://localhost:8080/bookings \
  -H 'Content-Type: application/json' \
  -d '{}'
```

HTTP `422`:

```json
{"violations":[{"field":"activityId","code":"required"},{"field":"seats","code":"required"}]}
```

Both rules in the required group run. The API does not load an activity.

### 2. Invalid values

```sh
curl -i http://localhost:8080/bookings \
  -H 'Content-Type: application/json' \
  -d '{"activityId":-1,"seats":0}'
```

HTTP `422`:

```json
{"violations":[{"field":"activityId","code":"positive"},{"field":"seats","code":"positive"}]}
```

Both fields are present, but their values must be positive. No lookup runs.

### 3. Unknown activity

```sh
curl -i http://localhost:8080/bookings \
  -H 'Content-Type: application/json' \
  -d '{"activityId":999,"seats":2}'
```

HTTP `422`:

```json
{"violations":[{"field":"activityId","code":"not_found"}]}
```

The API loads activity 999 once. It does not run the availability group.

### 4. Not enough seats

```sh
curl -i http://localhost:8080/bookings \
  -H 'Content-Type: application/json' \
  -d '{"activityId":1,"seats":5}'
```

HTTP `422`:

```json
{"violations":[{"field":"seats","code":"insufficient_seats"}]}
```

The activity exists, but it has only three seats. The existence and availability
checks use the same cached activity, so the repository is called once.

### 5. Create a booking

```sh
curl -i http://localhost:8080/bookings \
  -H 'Content-Type: application/json' \
  -d '{"activityId":1,"seats":2}'
```

HTTP `201 Created`:

```json
{"id":1,"activityId":1,"seats":2}
```

All groups pass and booking 1 is created. Activity 1 now has one seat left. Repeating
this request returns `422` with `insufficient_seats`. Restart the server to reset
the example; all data is kept in this process only.

## Watch the lookup

The first two requests produce no `loading activity` log. Each of the last three
produces one line like this:

```text
level=INFO msg="loading activity" activityId=1
```

The real log also has a timestamp. A successful request adds a `booking created`
line. Every new request gets fresh state and loads the current activity once. The
atomic write rechecks capacity in the stored activity under a lock; it does not call
`FindActivity` a second time.

## Read the code

```text
main.go
internal/
  httpapi/
    handler.go          Routing
    booking_handler.go  Decode, validate, create and respond
    request.go          Bounded JSON decoding
    response.go         HTTP response types and error codes
    logging.go          Shared HTTP logging keys
  booking/
    models.go             CreateRequest, CreateCommand, activity and booking
    service.go            Create a booking from validated values
    validation.go         CreateValidator, rules, request state and lazy activity loading
    repository.go         Repository contract and write-conflict errors
    repository_memory.go  Synchronized activities and bookings
  validation/
    violation.go        Shared violation type and codes
```

Start with [booking_handler.go](internal/httpapi/booking_handler.go), then read
[validation.go](internal/booking/validation.go) and [service.go](internal/booking/service.go).
[repository_memory.go](internal/booking/repository_memory.go) implements the repository.
The article draft is in [article.md](article.md).

`main` connects the packages. `httpapi` depends on `booking`. The booking package
defines both the repository interface and its memory implementation. The service
and validator accept that interface. The booking package does not import Gin.

The HTTP handler calls the validator before calling the service. After successful
validation, it converts a CreateRequest with pointer fields to a CreateCommand
with ordinary integer values. The service accepts that command and returns a
booking or an error. The handler converts an insufficient-capacity write error
to the same violation response as an earlier availability check. The validator
holds the repository, reuses the rules and creates fresh state for each call.
Request state contains only the input and cached lookup result.
Names such as `exists` label errors; they do not select or change rules. The state caches an
activity snapshot, including absence and errors, for this call only.

The JSON decoder reads one value and checks that the body ends there. There are
no `binding` validation tags on the request; seqvalidation owns the required and
business checks. Pointer fields make missing values different from zero. Both
missing and explicit null values produce `required` violations.

Malformed JSON, wrong field types and bodies above 1 MiB return `400` with
`{"error":"invalid_json"}`. Input violations return `422`. Operational errors
return `500` with `{"error":"booking_unavailable"}`; the original error goes to the server log,
with a rule location when validation fails. Cancellation also follows the error path.

## Capacity can change after validation

Two requests may load the same available-seat count and both pass validation.
`CreateBooking` checks the current count again, reduces it and records the booking
under one mutex. Only one of two requests for the last seats can succeed. A write
conflict becomes the same `insufficient_seats` violation as an earlier failure.

`FindActivity` returns a copy, so a request cannot change stored capacity through its
cached snapshot. The lock protects both the activity and booking maps. This works
inside one process. A database implementation would need an atomic write or a
transaction that keeps the capacity check and booking record together.

Bookings have integer IDs. Data is not persisted, and there is no retry or
idempotency mechanism. Sending another valid POST creates another booking if
capacity is available.

## Check

```sh
go test ./...
make check
```

`make check` runs formatting, tests on Go 1.25.0, race tests and vet on Go 1.27.1,
pinned golangci-lint and govulncheck. First use needs network access for tools,
toolchains and the vulnerability database. Tool binaries go into ignored `.tools/`.

Tests cover the five HTTP requests, malformed JSON, operation errors, cached
lookups, rejected writes, repeated requests and concurrent capacity updates.
A deterministic HTTP test holds all reads until sixteen requests have loaded
the same snapshot, then lets them reserve seats together. Exactly one succeeds.
GitHub Actions runs `make check`.
