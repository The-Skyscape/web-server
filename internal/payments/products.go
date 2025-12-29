package payments

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sync"
)

// Product represents a Stripe product
type Product struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Active      bool              `json:"active"`
	Metadata    map[string]string `json:"metadata"`
}

// Price represents a Stripe price
type Price struct {
	ID        string `json:"id"`
	ProductID string `json:"product"`
	Active    bool   `json:"active"`
	UnitAmount int64  `json:"unit_amount"`
	Currency  string `json:"currency"`
	Recurring *struct {
		Interval string `json:"interval"`
	} `json:"recurring"`
	LookupKey string `json:"lookup_key"`
}

// ProductCatalog holds our initialized Stripe product/price IDs
type ProductCatalog struct {
	// Verified subscription - $8/month
	VerifiedProductID string
	VerifiedPriceID   string

	// App promotion - $1/day one-time
	PromotionProductID string
	PromotionPriceID   string // $1 per day

	// Resource upgrades - monthly subscriptions
	CPUProductID     string
	CPUPriceID       string // $2.50 per half-core/month
	StorageProductID string
	StoragePriceID   string // $0.25 per GB/month

	initialized bool
	mu          sync.RWMutex
}

var catalog = &ProductCatalog{}

// GetCatalog returns the initialized product catalog
// It performs lazy initialization if not already done
func (c *Client) GetCatalog() (*ProductCatalog, error) {
	catalog.mu.RLock()
	if catalog.initialized {
		catalog.mu.RUnlock()
		return catalog, nil
	}
	catalog.mu.RUnlock()

	// Try to initialize
	if err := c.InitProducts(); err != nil {
		return nil, err
	}

	if !catalog.initialized {
		return nil, fmt.Errorf("stripe products not initialized - check Stripe configuration")
	}

	return catalog, nil
}

// InitProducts idempotently creates Stripe products and prices
// Call this on application startup
func (c *Client) InitProducts() error {
	if !c.IsConfigured() {
		log.Println("Stripe not configured, skipping product initialization")
		return nil
	}

	catalog.mu.Lock()
	defer catalog.mu.Unlock()

	if catalog.initialized {
		return nil
	}

	log.Println("Initializing Stripe products...")

	// 1. Verified Subscription - $8/month
	verifiedProduct, err := c.ensureProduct("skyscape_verified", "Verified Badge", "Verified badge, 2x replicas, priority support")
	if err != nil {
		return err
	}
	catalog.VerifiedProductID = verifiedProduct.ID

	verifiedPrice, err := c.ensurePrice("skyscape_verified_monthly", verifiedProduct.ID, 800, "usd", "month")
	if err != nil {
		return err
	}
	catalog.VerifiedPriceID = verifiedPrice.ID
	log.Printf("  Verified: product=%s price=%s", verifiedProduct.ID, verifiedPrice.ID)

	// 2. App Promotion - $1/day one-time payment
	promotionProduct, err := c.ensureProduct("skyscape_promotion", "App Promotion", "Promote your app in the activity feed")
	if err != nil {
		return err
	}
	catalog.PromotionProductID = promotionProduct.ID

	promotionPrice, err := c.ensurePrice("skyscape_promotion_daily", promotionProduct.ID, 100, "usd", "") // $1/day, one-time
	if err != nil {
		return err
	}
	catalog.PromotionPriceID = promotionPrice.ID
	log.Printf("  Promotion: product=%s price=%s", promotionProduct.ID, promotionPrice.ID)

	// 3. CPU Upgrade - $2.50/half-core/month (so $5/core/month)
	cpuProduct, err := c.ensureProduct("skyscape_cpu", "CPU Cores", "Additional CPU for your app")
	if err != nil {
		return err
	}
	catalog.CPUProductID = cpuProduct.ID

	cpuPrice, err := c.ensurePrice("skyscape_cpu_monthly", cpuProduct.ID, 250, "usd", "month") // $2.50 per half-core
	if err != nil {
		return err
	}
	catalog.CPUPriceID = cpuPrice.ID
	log.Printf("  CPU: product=%s price=%s", cpuProduct.ID, cpuPrice.ID)

	// 4. Storage Upgrade - $0.25/GB/month
	storageProduct, err := c.ensureProduct("skyscape_storage", "Storage", "Additional storage for your app")
	if err != nil {
		return err
	}
	catalog.StorageProductID = storageProduct.ID

	storagePrice, err := c.ensurePrice("skyscape_storage_monthly", storageProduct.ID, 25, "usd", "month") // $0.25/GB
	if err != nil {
		return err
	}
	catalog.StoragePriceID = storagePrice.ID
	log.Printf("  Storage: product=%s price=%s", storageProduct.ID, storagePrice.ID)

	catalog.initialized = true
	log.Println("Stripe products initialized")
	return nil
}

// ensureProduct creates a product if it doesn't exist, returns existing if it does
func (c *Client) ensureProduct(lookupKey, name, description string) (*Product, error) {
	// Search for existing product by metadata lookup_key
	products, err := c.listProducts(lookupKey)
	if err != nil {
		return nil, err
	}

	if len(products) > 0 {
		return &products[0], nil
	}

	// Create new product
	return c.createProduct(lookupKey, name, description)
}

// listProducts searches for products by metadata lookup_key
func (c *Client) listProducts(lookupKey string) ([]Product, error) {
	params := url.Values{}
	params.Set("active", "true")
	params.Set("limit", "100")

	data, err := c.request("GET", "/products?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data []Product `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	// Filter by metadata lookup_key
	var filtered []Product
	for _, p := range resp.Data {
		if p.Metadata["lookup_key"] == lookupKey {
			filtered = append(filtered, p)
		}
	}

	return filtered, nil
}

// createProduct creates a new Stripe product
func (c *Client) createProduct(lookupKey, name, description string) (*Product, error) {
	params := url.Values{}
	params.Set("name", name)
	params.Set("description", description)
	params.Set("metadata[lookup_key]", lookupKey)

	data, err := c.request("POST", "/products", params)
	if err != nil {
		return nil, err
	}

	var product Product
	if err := json.Unmarshal(data, &product); err != nil {
		return nil, err
	}

	return &product, nil
}

// ensurePrice creates a price if it doesn't exist
func (c *Client) ensurePrice(lookupKey, productID string, amount int64, currency, interval string) (*Price, error) {
	// Search for existing price by lookup_key
	price, err := c.getPriceByLookupKey(lookupKey)
	if err != nil {
		return nil, err
	}

	if price != nil {
		return price, nil
	}

	// Create new price
	return c.createPrice(lookupKey, productID, amount, currency, interval)
}

// getPriceByLookupKey retrieves a price by its lookup key
func (c *Client) getPriceByLookupKey(lookupKey string) (*Price, error) {
	params := url.Values{}
	params.Set("lookup_keys[]", lookupKey)

	data, err := c.request("GET", "/prices?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data []Price `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	if len(resp.Data) > 0 {
		return &resp.Data[0], nil
	}

	return nil, nil
}

// createPrice creates a new Stripe price
func (c *Client) createPrice(lookupKey, productID string, amount int64, currency, interval string) (*Price, error) {
	params := url.Values{}
	params.Set("product", productID)
	params.Set("unit_amount", fmt.Sprintf("%d", amount))
	params.Set("currency", currency)
	params.Set("lookup_key", lookupKey)

	if interval != "" {
		params.Set("recurring[interval]", interval)
	}

	data, err := c.request("POST", "/prices", params)
	if err != nil {
		return nil, err
	}

	var price Price
	if err := json.Unmarshal(data, &price); err != nil {
		return nil, err
	}

	return &price, nil
}
