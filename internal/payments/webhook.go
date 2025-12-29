package payments

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Event types
const (
	EventCheckoutCompleted     = "checkout.session.completed"
	EventSubscriptionCreated   = "customer.subscription.created"
	EventSubscriptionUpdated   = "customer.subscription.updated"
	EventSubscriptionDeleted   = "customer.subscription.deleted"
	EventPaymentSucceeded      = "payment_intent.succeeded"
	EventPaymentFailed         = "payment_intent.payment_failed"
)

// Event represents a Stripe webhook event
type Event struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Data    json.RawMessage `json:"data"`
	Created int64           `json:"created"`
}

// EventData represents the data field of an event
type EventData struct {
	Object json.RawMessage `json:"object"`
}

// CheckoutSessionEvent extracts checkout session from event data
func (e *Event) CheckoutSessionEvent() (*CheckoutSession, error) {
	var data EventData
	if err := json.Unmarshal(e.Data, &data); err != nil {
		return nil, err
	}

	var session CheckoutSession
	if err := json.Unmarshal(data.Object, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

// SubscriptionEvent extracts subscription from event data
func (e *Event) SubscriptionEvent() (*Subscription, error) {
	var data EventData
	if err := json.Unmarshal(e.Data, &data); err != nil {
		return nil, err
	}

	var sub Subscription
	if err := json.Unmarshal(data.Object, &sub); err != nil {
		return nil, err
	}

	return &sub, nil
}

// Metadata extracts metadata from checkout session event
func (e *Event) Metadata() (map[string]string, error) {
	var data struct {
		Object struct {
			Metadata map[string]string `json:"metadata"`
		} `json:"object"`
	}
	if err := json.Unmarshal(e.Data, &data); err != nil {
		return nil, err
	}
	return data.Object.Metadata, nil
}

// VerifyWebhook verifies the webhook signature and returns the event
func (c *Client) VerifyWebhook(payload []byte, signature string) (*Event, error) {
	if c.webhookSecret == "" {
		return nil, fmt.Errorf("webhook secret not configured")
	}

	// Parse signature header
	// Format: t=timestamp,v1=signature
	parts := strings.Split(signature, ",")
	var timestamp, sig string
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			timestamp = kv[1]
		case "v1":
			sig = kv[1]
		}
	}

	if timestamp == "" || sig == "" {
		return nil, fmt.Errorf("invalid signature header")
	}

	// Verify timestamp is recent (within 5 minutes)
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid timestamp")
	}

	diff := time.Now().Unix() - ts
	if diff < -300 || diff > 300 {
		return nil, fmt.Errorf("timestamp outside tolerance window")
	}

	// Compute expected signature
	signedPayload := timestamp + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(c.webhookSecret))
	mac.Write([]byte(signedPayload))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	// Compare signatures
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return nil, fmt.Errorf("signature verification failed")
	}

	// Parse event
	var event Event
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, err
	}

	return &event, nil
}
