package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Francesco99975/shorehamex2/internal/enums"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestRateLimiter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		env            enums.Environment
		acceptHeader   string
		requestCount   int
		delayBetween   time.Duration
		wantStatus     int
		wantRetryAfter string
		wantBodyJSON   bool
	}{
		{
			name:         "PROD: under limit → 200",
			env:          enums.Environments.PRODUCTION,
			acceptHeader: "application/json",
			requestCount: 70,
			delayBetween: 3 * time.Millisecond,
			wantStatus:   http.StatusOK,
		},
		{
			name:           "PROD: over limit → 429 + retryIn",
			env:            enums.Environments.PRODUCTION,
			acceptHeader:   "application/json",
			requestCount:   90,
			delayBetween:   1 * time.Millisecond,
			wantStatus:     http.StatusTooManyRequests,
			wantRetryAfter: "180",
			wantBodyJSON:   true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			e.HideBanner = true
			e.Use(RateLimiter())

			e.GET("/", func(c echo.Context) error {
				return c.String(http.StatusOK, "ok")
			})

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept", tt.acceptHeader)
			req.RemoteAddr = "1.2.3.4:5678"

			var lastRec *httptest.ResponseRecorder

			for i := 0; i < tt.requestCount; i++ {
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, req)
				lastRec = rec
				if tt.delayBetween > 0 {
					time.Sleep(tt.delayBetween)
				}
			}

			assert.Equal(t, tt.wantStatus, lastRec.Code, "request count: %d", tt.requestCount)

			if tt.wantStatus == http.StatusTooManyRequests {
				assert.Equal(t, tt.wantRetryAfter, lastRec.Header().Get("Retry-After"))

				body := lastRec.Body.String()
				if tt.wantBodyJSON {
					assert.Contains(t, body, "Rate limit exceeded")
					if tt.env == enums.Environments.PRODUCTION {
						assert.Contains(t, body, `"retryIn":"180"`)
					}
				} else {
					assert.Contains(t, body, "Too many requests")
				}
			}
		})
	}
}
