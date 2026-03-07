package httpapi

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
)

const (
	retryMinSeconds = 3
	retryMaxSeconds = 100
)

func (h *Handler) withGlobalSemaphore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case h.concurrencySem <- struct{}{}:
			defer func() { <-h.concurrencySem }()
			next.ServeHTTP(w, r)
		default:
			retrySeconds := randomRetrySeconds(retryMinSeconds, retryMaxSeconds)
			w.Header().Set("Retry-After", strconv.Itoa(retrySeconds))
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"error":               fmt.Sprintf("concurrency limit reached, please try again in %d seconds", retrySeconds),
				"retry_after_seconds": retrySeconds,
			})
		}
	})
}

func randomRetrySeconds(min int, max int) int {
	if min >= max {
		return min
	}
	span := max - min + 1
	raw, err := rand.Int(rand.Reader, big.NewInt(int64(span)))
	if err != nil {
		return min
	}
	return min + int(raw.Int64())
}
