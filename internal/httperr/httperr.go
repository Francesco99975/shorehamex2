package httperr

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Francesco99975/shorehamex2/internal/enums"
	"github.com/Francesco99975/shorehamex2/internal/helpers"
	"github.com/Francesco99975/shorehamex2/internal/monitoring"
	"github.com/Francesco99975/shorehamex2/internal/tools"
	"github.com/Francesco99975/shorehamex2/views/components"
	"github.com/labstack/echo/v4"
)

// errorDescriptor holds the user-facing Why and How-to-remedy for a given HTTP status code.
type errorDescriptor struct {
	why    string
	remedy string
}

// JSONErrorResponse is the structured error body returned by HandleJSON.
// RequestID is omitted when absent so it does not appear on handlers that
// don't carry one.
type JSONErrorResponse struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// clientMessages maps 4xx status codes to user-facing error context.
var clientMessages = map[int]errorDescriptor{
	http.StatusBadRequest: {
		why:    "the request contained invalid or malformed data",
		remedy: "check your request parameters and try again",
	},
	http.StatusUnauthorized: {
		why:    "authentication failed",
		remedy: "provide valid credentials and try again",
	},
	http.StatusForbidden: {
		why:    "you do not have permission to perform this action",
		remedy: "contact an administrator if you believe this is an error",
	},
	http.StatusNotFound: {
		why:    "the requested resource could not be found",
		remedy: "verify the resource identifier and try again",
	},
	http.StatusMethodNotAllowed: {
		why:    "this HTTP method is not permitted for the requested resource",
		remedy: "consult the API documentation for the correct method",
	},
	http.StatusConflict: {
		why:    "a conflict occurred with the current state of the resource",
		remedy: "resolve the conflict and retry the request",
	},
	http.StatusUnprocessableEntity: {
		why:    "the provided data failed validation",
		remedy: "review the field constraints and correct the data before retrying",
	},
	http.StatusTooManyRequests: {
		why:    "the allowed request rate has been exceeded",
		remedy: "wait before retrying or contact support to adjust your rate limit",
	},
}

// serverMessages maps 5xx status codes to user-facing error context.
var serverMessages = map[int]errorDescriptor{
	http.StatusInternalServerError: {
		why:    "an unexpected error occurred on the server",
		remedy: "try again later or contact support if the problem persists",
	},
	http.StatusNotImplemented: {
		why:    "the requested operation is not supported by the server",
		remedy: "consult the API documentation or contact support",
	},
	http.StatusBadGateway: {
		why:    "an upstream service returned an invalid response",
		remedy: "try again in a few moments",
	},
	http.StatusServiceUnavailable: {
		why:    "the service is temporarily unavailable",
		remedy: "try again in a few moments",
	},
	http.StatusGatewayTimeout: {
		why:    "an upstream service did not respond in time",
		remedy: "try again later or contact support if the problem persists",
	},
}

// defaultClientMessage is the fallback for unmapped 4xx codes.
var defaultClientMessage = errorDescriptor{
	why:    "an unexpected client error occurred",
	remedy: "check your request and try again",
}

// defaultServerMessage is the fallback for unmapped 5xx codes.
var defaultServerMessage = errorDescriptor{
	why:    "an unexpected server error occurred",
	remedy: "try again later or contact support if the problem persists",
}

// HttpErrorMessage handles structured, consistent error reporting for a controller action.
// The what, origin, and requestID fields are constant for the lifetime of the handler
// invocation and are attached to every log entry produced by this instance.
type HttpErrorMessage struct {
	what      string // the operation being performed, e.g. "creating post"
	origin    string // the handler function name, e.g. "CreatePost"
	requestID string // the request ID for log correlation, e.g. from X-Request-ID
}

// errorCall is an ephemeral, single-use builder produced by Why().
// It carries a why override for one specific error without mutating
// the parent HttpErrorMessage — preventing bleed between errors.
type errorCall struct {
	h           *HttpErrorMessage
	whyOverride string
}

// New creates an HttpErrorMessage for a given controller action.
//
//   - what:      the operation being performed (e.g. "creating post")
//   - origin:    the handler function name (e.g. "CreatePost")
//   - requestID: the request-scoped ID for correlating log lines (e.g. from X-Request-ID)
func New(what, origin, requestID string) *HttpErrorMessage {
	return &HttpErrorMessage{
		what:      what,
		origin:    origin,
		requestID: requestID,
	}
}

// Why returns an ephemeral errorCall carrying a caller-supplied why override.
// Use when the status code alone is too generic for the specific failure.
//
// Usage:
//
//	return errMsg.Why("the username does not exist").Handle(w, http.StatusUnauthorized, err)
func (h *HttpErrorMessage) Why(reason string) *errorCall {
	return &errorCall{h: h, whyOverride: reason}
}

// ─── HttpErrorMessage handlers (no why override) ─────────────────────────────

// Handle is for HTMX/HTML handlers. Logs the error and triggers a toast.
func (h *HttpErrorMessage) Handle(w http.ResponseWriter, code int, err error) error {
	return handle(h, "", w, code, err)
}

// HandleOnForm is for inline form error rendering. Logs the error and writes
// an HTML error component into the response.
func (h *HttpErrorMessage) HandleOnForm(w http.ResponseWriter, code int, err error, box enums.Box, persistence *time.Duration) error {
	return handleOnForm(h, "", w, code, err, box, persistence)
}

// HandleJSON is for API handlers. Logs the error and writes a structured
// JSON error body.
func (h *HttpErrorMessage) HandleJSON(w http.ResponseWriter, code int, err error) error {
	return handleJSON(h, "", w, code, err)
}

// HandleEchoPage is for navigational handlers. Logs the error and returns an
// echo.HTTPError which Echo's serverErrorHandler catches to render the
// appropriate error page or JSON response based on the Accept header.
func (h *HttpErrorMessage) HandleEchoPage(code int, err error) error {
	return handleEchoPage(h, code, err)
}

// ─── errorCall handlers (with why override) ───────────────────────────────────

// Handle is for HTMX/HTML handlers. Logs the error and triggers a toast,
// using the caller-supplied why in place of the map lookup.
func (c *errorCall) Handle(w http.ResponseWriter, code int, err error) error {
	return handle(c.h, c.whyOverride, w, code, err)
}

// HandleOnForm is for inline form error rendering with a why override.
func (c *errorCall) HandleOnForm(w http.ResponseWriter, code int, err error, box enums.Box, persistence *time.Duration) error {
	return handleOnForm(c.h, c.whyOverride, w, code, err, box, persistence)
}

// HandleJSON is for API handlers with a why override.
func (c *errorCall) HandleJSON(w http.ResponseWriter, code int, err error) error {
	return handleJSON(c.h, c.whyOverride, w, code, err)
}

// HandleEchoPage is for navigational handlers with a why override.
func (c *errorCall) HandleEchoPage(code int, err error) error {
	return handleEchoPage(c.h, code, err)
}

// ─── shared implementations ───────────────────────────────────────────────────

func handle(h *HttpErrorMessage, whyOverride string, w http.ResponseWriter, code int, err error) error {
	monitoring.RecordError(fmt.Sprintf("%d", code))
	h.log(code, err)

	message := h.buildMessage(code, whyOverride)

	tools.SetToastTrigger(w, enums.ErrorToast, message)
	w.WriteHeader(code)
	return err
}

func handleOnForm(h *HttpErrorMessage, whyOverride string, w http.ResponseWriter, code int, err error, box enums.Box, persistence *time.Duration) error {
	var prs string
	if persistence != nil {
		prs = strconv.FormatInt(persistence.Milliseconds(), 10)
	}

	monitoring.RecordError(fmt.Sprintf("%d", code))
	h.log(code, err)

	message := h.buildMessage(code, whyOverride)

	htmlBytes := helpers.MustRenderHTML(components.ErrorMsg(message, box, prs))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	if _, internalErr := w.Write(htmlBytes); internalErr != nil {
		h.log(http.StatusInternalServerError, internalErr)
		return internalErr
	}
	return err
}

func handleJSON(h *HttpErrorMessage, whyOverride string, w http.ResponseWriter, code int, err error) error {
	monitoring.RecordError(fmt.Sprintf("%d", code))
	h.log(code, err)

	message := h.buildMessage(code, whyOverride)

	resp := JSONErrorResponse{
		Code:      code,
		Message:   message,
		RequestID: h.requestID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if internalErr := json.NewEncoder(w).Encode(resp); internalErr != nil {
		h.log(http.StatusInternalServerError, internalErr)
		return internalErr
	}
	return err
}

func handleEchoPage(h *HttpErrorMessage, code int, err error) error {
	monitoring.RecordError(fmt.Sprintf("%d", code))
	h.log(code, err)
	return echo.NewHTTPError(code)
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// buildMessage composes the final user-facing message.
// If whyOverride is provided it replaces the map-resolved why.
func (h *HttpErrorMessage) buildMessage(code int, whyOverride string) string {
	descriptor := h.resolve(code)
	why := descriptor.why
	if whyOverride != "" {
		why = whyOverride
	}
	return fmt.Sprintf(
		"%s failed, %s. %s.",
		h.what, why, descriptor.remedy,
	)
}

// resolve looks up the errorDescriptor for the given status code, falling back to
// the appropriate default based on whether the code is a 4xx or 5xx.
func (h *HttpErrorMessage) resolve(code int) errorDescriptor {
	if code >= 500 {
		if descriptor, ok := serverMessages[code]; ok {
			return descriptor
		}
		return defaultServerMessage
	}

	if descriptor, ok := clientMessages[code]; ok {
		return descriptor
	}
	return defaultClientMessage
}

// log emits a structured log entry at the appropriate severity level.
// Every entry carries action, origin, request_id, status, and error as
// discrete fields for easy filtering and correlation.
func (h *HttpErrorMessage) log(code int, err error) {
	attrs := []any{
		"action", h.what,
		"origin", h.origin,
		"request_id", h.requestID,
		"status", code,
		"error", err,
	}

	if code >= 500 {
		slog.Error("server error", attrs...)
	} else {
		slog.Warn("client error", attrs...)
	}
}
