package cerr

import (
	"net/http"

	"github.com/aserto-dev/go-utils/cerr"
	"google.golang.org/grpc/codes"
)

var (
	// The asked-for runtime is not yet available, but will likely be in the future.
	ErrRuntimeLoading = cerr.NewAsertoError("E10006", codes.Unavailable, http.StatusTooEarly, "runtime has not yet loaded")
	// Returned when a runtime query has an error
	ErrBadQuery = cerr.NewAsertoError("E10047", codes.InvalidArgument, http.StatusBadRequest, "invalid query")
	// Returned when a runtime failed to load
	ErrBadRuntime = cerr.NewAsertoError("E10053", codes.Unavailable, http.StatusServiceUnavailable, "runtime loading failed")
	// Returned when a runtime query has an error
	ErrQueryExecutionFailed = cerr.NewAsertoError("E10048", codes.FailedPrecondition, http.StatusBadRequest, "query failed")
)
