package stripe

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Client is a Stripe API client
type Client struct {
	secretKey     string
	publishKey    string
	webhookSecret string
	baseURL       string
	httpClient    *http.Client
}

// New creates a new Stripe client from environment variables
func New() *Client {
	return &Client{
		secretKey:     os.Getenv("STRIPE_SECRET_KEY"),
		publishKey:    os.Getenv("STRIPE_PUBLISHABLE_KEY"),
		webhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		baseURL:       "https://api.stripe.com/v1",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// PublishableKey returns the publishable key for client-side use
func (c *Client) PublishableKey() string {
	return c.publishKey
}

// IsConfigured returns true if Stripe credentials are set
func (c *Client) IsConfigured() bool {
	return c.secretKey != "" && c.publishKey != ""
}

// request makes an authenticated request to the Stripe API
func (c *Client) request(method, endpoint string, params url.Values) ([]byte, error) {
	reqURL := c.baseURL + endpoint

	var body io.Reader
	if params != nil && len(params) > 0 {
		body = strings.NewReader(params.Encode())
	}

	req, err := http.NewRequest(method, reqURL, body)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(c.secretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		var apiErr struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
		}
		if err := json.Unmarshal(responseBody, &apiErr); err == nil && apiErr.Error.Message != "" {
			return nil, fmt.Errorf("stripe: %s", apiErr.Error.Message)
		}
		return nil, fmt.Errorf("stripe: request failed with status %d", resp.StatusCode)
	}

	return responseBody, nil
}

// Customer represents a Stripe customer
type Customer struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// CreateCustomer creates a new Stripe customer
func (c *Client) CreateCustomer(email, name string, metadata map[string]string) (*Customer, error) {
	params := url.Values{}
	params.Set("email", email)
	params.Set("name", name)

	for k, v := range metadata {
		params.Set("metadata["+k+"]", v)
	}

	data, err := c.request("POST", "/customers", params)
	if err != nil {
		return nil, err
	}

	var customer Customer
	if err := json.Unmarshal(data, &customer); err != nil {
		return nil, err
	}

	return &customer, nil
}

// CreatePortalSession creates a Stripe Customer Portal session
func (c *Client) CreatePortalSession(customerID, returnURL string) (string, error) {
	params := url.Values{}
	params.Set("customer", customerID)
	params.Set("return_url", returnURL)

	data, err := c.request("POST", "/billing_portal/sessions", params)
	if err != nil {
		return "", err
	}

	var session struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(data, &session); err != nil {
		return "", err
	}

	return session.URL, nil
}
