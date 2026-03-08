package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const polarWebhookTolerance = 5 * time.Minute

var errInvalidPolarWebhook = errors.New("invalid polar webhook")

type polarWebhookEvent struct {
	Type string `json:"type"`
	Data struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Checkout *struct {
			ID string `json:"id"`
		} `json:"checkout"`
		Object *struct {
			ID string `json:"id"`
		} `json:"object"`
	} `json:"data"`
}

func verifyPolarWebhook(secret string, headers map[string]string, body []byte, now time.Time) error {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return errors.New("polar webhook secret not configured")
	}

	msgID := strings.TrimSpace(headers["webhook-id"])
	msgTimestamp := strings.TrimSpace(headers["webhook-timestamp"])
	signatureHeader := strings.TrimSpace(headers["webhook-signature"])
	if msgID == "" || msgTimestamp == "" || signatureHeader == "" {
		return errInvalidPolarWebhook
	}

	ts, err := strconv.ParseInt(msgTimestamp, 10, 64)
	if err != nil {
		return errInvalidPolarWebhook
	}

	now = now.UTC()
	msgTime := time.Unix(ts, 0).UTC()
	if msgTime.Before(now.Add(-polarWebhookTolerance)) || msgTime.After(now.Add(polarWebhookTolerance)) {
		return errInvalidPolarWebhook
	}

	signedPayload := msgID + "." + msgTimestamp + "." + string(body)
	signatures := strings.Fields(signatureHeader)
	if len(signatures) == 0 {
		return errInvalidPolarWebhook
	}

	keys := [][]byte{
		[]byte(base64.StdEncoding.EncodeToString([]byte(secret))),
		[]byte(secret),
	}

	for _, headerSig := range signatures {
		version, encodedSig, ok := strings.Cut(headerSig, ",")
		if !ok || version != "v1" || encodedSig == "" {
			continue
		}
		expectedSig, err := base64.StdEncoding.DecodeString(encodedSig)
		if err != nil {
			continue
		}
		for _, key := range keys {
			mac := hmac.New(sha256.New, key)
			_, _ = mac.Write([]byte(signedPayload))
			if subtle.ConstantTimeCompare(mac.Sum(nil), expectedSig) == 1 {
				return nil
			}
		}
	}

	return errInvalidPolarWebhook
}

func parsePolarWebhook(body []byte) (*polarWebhookEvent, error) {
	var event polarWebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, err
	}
	if strings.TrimSpace(event.Type) == "" {
		return nil, fmt.Errorf("missing event type")
	}
	return &event, nil
}

func polarCheckoutID(event *polarWebhookEvent) string {
	if event == nil {
		return ""
	}
	if id := strings.TrimSpace(event.Data.ID); id != "" {
		return id
	}
	if event.Data.Checkout != nil {
		if id := strings.TrimSpace(event.Data.Checkout.ID); id != "" {
			return id
		}
	}
	if event.Data.Object != nil {
		if id := strings.TrimSpace(event.Data.Object.ID); id != "" {
			return id
		}
	}
	return ""
}
