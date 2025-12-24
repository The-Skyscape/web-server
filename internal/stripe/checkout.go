package stripe

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// CheckoutMode represents the type of checkout session
type CheckoutMode string

const (
	ModeSubscription CheckoutMode = "subscription"
	ModePayment      CheckoutMode = "payment"
)

// CheckoutSession represents a Stripe Checkout session
type CheckoutSession struct {
	ID             string `json:"id"`
	URL            string `json:"url"`
	Status         string `json:"status"`
	CustomerID     string `json:"customer"`
	SubscriptionID string `json:"subscription"`
	PaymentStatus  string `json:"payment_status"`
}

// LineItem represents a line item for checkout
type LineItem struct {
	PriceID  string // Stripe Price ID (from catalog)
	Quantity int64
}

// CheckoutOptions configures a checkout session
type CheckoutOptions struct {
	Mode          CheckoutMode
	CustomerID    string            // Existing customer ID (optional)
	CustomerEmail string            // For new customers
	SuccessURL    string
	CancelURL     string
	LineItems     []LineItem
	Metadata      map[string]string

	// For subscriptions
	TrialDays int
}

// CreateCheckoutSession creates a new Stripe Checkout session
func (c *Client) CreateCheckoutSession(opts CheckoutOptions) (*CheckoutSession, error) {
	params := url.Values{}

	// Set mode
	if opts.Mode == "" {
		opts.Mode = ModePayment
	}
	params.Set("mode", string(opts.Mode))

	// Set customer
	if opts.CustomerID != "" {
		params.Set("customer", opts.CustomerID)
	} else if opts.CustomerEmail != "" {
		params.Set("customer_email", opts.CustomerEmail)
	}

	// Set URLs
	params.Set("success_url", opts.SuccessURL)
	params.Set("cancel_url", opts.CancelURL)

	// Set line items (using pre-configured Stripe prices)
	for i, item := range opts.LineItems {
		prefix := fmt.Sprintf("line_items[%d]", i)
		params.Set(prefix+"[price]", item.PriceID)
		params.Set(prefix+"[quantity]", fmt.Sprintf("%d", item.Quantity))
	}

	// Set metadata
	for k, v := range opts.Metadata {
		params.Set("metadata["+k+"]", v)
	}

	// Set trial for subscriptions
	if opts.Mode == ModeSubscription && opts.TrialDays > 0 {
		params.Set("subscription_data[trial_period_days]", fmt.Sprintf("%d", opts.TrialDays))
	}

	data, err := c.request("POST", "/checkout/sessions", params)
	if err != nil {
		return nil, err
	}

	var session CheckoutSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

// GetCheckoutSession retrieves a checkout session by ID
func (c *Client) GetCheckoutSession(id string) (*CheckoutSession, error) {
	data, err := c.request("GET", "/checkout/sessions/"+id, nil)
	if err != nil {
		return nil, err
	}

	var session CheckoutSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

// Subscription represents a Stripe subscription
type Subscription struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	CustomerID       string `json:"customer"`
	CurrentPeriodEnd int64  `json:"current_period_end"`
	CanceledAt       *int64 `json:"canceled_at"`
}

// GetSubscription retrieves a subscription by ID
func (c *Client) GetSubscription(id string) (*Subscription, error) {
	data, err := c.request("GET", "/subscriptions/"+id, nil)
	if err != nil {
		return nil, err
	}

	var sub Subscription
	if err := json.Unmarshal(data, &sub); err != nil {
		return nil, err
	}

	return &sub, nil
}

// CancelSubscription cancels a subscription
func (c *Client) CancelSubscription(id string) error {
	_, err := c.request("DELETE", "/subscriptions/"+id, nil)
	return err
}
