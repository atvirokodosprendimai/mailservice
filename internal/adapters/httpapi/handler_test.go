package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONRejectsMultiplePayloads(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"owner_email":"owner@example.com"}{"extra":true}`))

	var payload map[string]any
	err := decodeJSON(req, &payload)
	if err == nil {
		t.Fatalf("expected decodeJSON to reject multiple JSON payloads")
	}
}
