package controllers

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"www.theskyscape.com/internal/payments"
	"www.theskyscape.com/models"
)

func Payments() (string, *PaymentsController) {
	return "payments", &PaymentsController{}
}

type PaymentsController struct {
	application.Controller
	stripe *payments.Client
}

func (c *PaymentsController) Setup(app *application.App) {
	c.Controller.Setup(app)
	c.stripe = payments.New()

	// Initialize Stripe products idempotently
	if err := c.stripe.InitProducts(); err != nil {
		log.Printf("Warning: Failed to initialize Stripe products: %v", err)
	}

	auth := c.Use("auth").(*AuthController)

	// Checkout session creation
	http.Handle("POST /checkout/verified", c.ProtectFunc(c.checkoutVerified, auth.Required))
	http.Handle("POST /checkout/promotion/{app}", c.ProtectFunc(c.checkoutPromotion, auth.Required))
	http.Handle("POST /checkout/upgrade/{app}", c.ProtectFunc(c.checkoutUpgrade, auth.Required))

	// Stripe webhook (no CSRF protection needed - Stripe signs requests)
	http.Handle("POST /webhooks/stripe", http.HandlerFunc(c.handleWebhook))

	// Success/Cancel pages
	http.Handle("GET /checkout/success", app.Serve("checkout-success.html", auth.Required))
	http.Handle("GET /checkout/cancel", app.Serve("checkout-cancel.html", auth.Optional))

	// Billing management
	http.Handle("GET /billing", app.Serve("billing.html", auth.Required))
	http.Handle("POST /billing/portal", c.ProtectFunc(c.billingPortal, auth.Required))
}

func (c PaymentsController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

// Template methods

// StripePublishableKey returns the Stripe publishable key for client-side use
func (c *PaymentsController) StripePublishableKey() string {
	return c.stripe.PublishableKey()
}

// IsStripeConfigured returns true if Stripe is properly configured
func (c *PaymentsController) IsStripeConfigured() bool {
	return c.stripe.IsConfigured()
}

// UserSubscriptions returns all subscriptions for the current user
func (c *PaymentsController) UserSubscriptions() []*models.Subscription {
	auth := c.Use("auth").(*AuthController)
	user, _, _ := auth.Authenticate(c.Request)
	if user == nil {
		return nil
	}
	return models.UserSubscriptions(user.ID, 50)
}

// ActiveSubscriptions returns only active subscriptions for the current user
func (c *PaymentsController) ActiveSubscriptions() []*models.Subscription {
	subs := c.UserSubscriptions()
	var active []*models.Subscription
	for _, s := range subs {
		if s.IsActive() {
			active = append(active, s)
		}
	}
	return active
}

// UserPayments returns payment history for the current user
func (c *PaymentsController) UserPayments() []*models.Payment {
	auth := c.Use("auth").(*AuthController)
	user, _, _ := auth.Authenticate(c.Request)
	if user == nil {
		return nil
	}
	return models.UserPayments(user.ID, 50)
}

// HasVerifiedSubscription returns true if current user has active verified subscription
func (c *PaymentsController) HasVerifiedSubscription() bool {
	auth := c.Use("auth").(*AuthController)
	user, _, _ := auth.Authenticate(c.Request)
	if user == nil {
		return false
	}
	return models.GetUserVerifiedSubscription(user.ID) != nil
}

// Checkout handlers

func (c *PaymentsController) checkoutVerified(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, _ := auth.Authenticate(r)

	// Get profile
	profile, err := models.Profiles.First("WHERE UserID = ?", user.ID)
	if err != nil {
		c.RenderError(w, r, errors.New("profile not found"))
		return
	}

	// Check if already verified
	if profile.Verified {
		c.RenderError(w, r, errors.New("you are already verified"))
		return
	}

	// Create or get Stripe customer
	customerID := profile.StripeCustomerID
	if customerID == "" {
		customer, err := c.stripe.CreateCustomer(user.Email, user.Name, map[string]string{
			"user_id": user.ID,
		})
		if err != nil {
			c.RenderError(w, r, fmt.Errorf("failed to create customer: %w", err))
			return
		}
		customerID = customer.ID
		profile.StripeCustomerID = customerID
		models.Profiles.Update(profile)
	}

	baseURL := "https://www.theskyscape.com"
	if prefix := os.Getenv("PREFIX"); prefix != "" {
		baseURL = "https://" + prefix + ".theskyscape.com"
	}

	// Create checkout session using pre-initialized price
	catalog, err := c.stripe.GetCatalog()
	if err != nil {
		c.RenderError(w, r, fmt.Errorf("payment system not configured: %w", err))
		return
	}
	session, err := c.stripe.CreateCheckoutSession(payments.CheckoutOptions{
		Mode:       payments.ModeSubscription,
		CustomerID: customerID,
		SuccessURL: baseURL + "/checkout/success?session_id={CHECKOUT_SESSION_ID}",
		CancelURL:  baseURL + "/checkout/cancel",
		LineItems: []payments.LineItem{{
			PriceID:  catalog.VerifiedPriceID,
			Quantity: 1,
		}},
		Metadata: map[string]string{
			"user_id":      user.ID,
			"product_type": models.PaymentVerified,
		},
	})
	if err != nil {
		c.RenderError(w, r, fmt.Errorf("failed to create checkout: %w", err))
		return
	}

	// Record pending payment
	models.Payments.Insert(&models.Payment{
		UserID:          user.ID,
		StripePaymentID: session.ID,
		ProductType:     models.PaymentVerified,
		Amount:          800,
		Currency:        "usd",
		Status:          models.PaymentPending,
	})

	// Use http.Redirect for external Stripe URLs (not c.Redirect which is for internal HTMX navigation)
	http.Redirect(w, r, session.URL, http.StatusSeeOther)
}

func (c *PaymentsController) checkoutPromotion(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, _ := auth.Authenticate(r)

	appID := r.PathValue("app")
	app, err := models.Apps.Get(appID)
	if err != nil {
		c.RenderError(w, r, errors.New("app not found"))
		return
	}

	// Verify ownership
	repo := app.Repo()
	if repo == nil || repo.OwnerID != user.ID {
		c.RenderError(w, r, errors.New("you can only promote your own apps"))
		return
	}

	// Check for existing promotion
	if existing := app.ActivePromotion(); existing != nil {
		c.RenderError(w, r, errors.New("this app already has an active promotion"))
		return
	}

	// Get days from form (default 7)
	days, _ := strconv.Atoi(r.FormValue("days"))
	if days < 1 {
		days = 7
	}
	if days > 30 {
		days = 30
	}

	content := r.FormValue("content")
	amount := int64(days * 100) // $1 per day in cents

	// Get profile for Stripe customer
	profile, _ := models.Profiles.First("WHERE UserID = ?", user.ID)
	customerID := ""
	if profile != nil && profile.StripeCustomerID != "" {
		customerID = profile.StripeCustomerID
	}

	baseURL := "https://www.theskyscape.com"
	if prefix := os.Getenv("PREFIX"); prefix != "" {
		baseURL = "https://" + prefix + ".theskyscape.com"
	}

	catalog, err := c.stripe.GetCatalog()
	if err != nil {
		c.RenderError(w, r, fmt.Errorf("payment system not configured: %w", err))
		return
	}
	opts := payments.CheckoutOptions{
		Mode:       payments.ModePayment,
		SuccessURL: baseURL + "/checkout/success?session_id={CHECKOUT_SESSION_ID}",
		CancelURL:  baseURL + "/app/" + appID + "/manage",
		LineItems: []payments.LineItem{{
			PriceID:  catalog.PromotionPriceID,
			Quantity: int64(days),
		}},
		Metadata: map[string]string{
			"user_id":      user.ID,
			"product_type": models.PaymentPromotion,
			"app_id":       appID,
			"days":         strconv.Itoa(days),
			"content":      content,
		},
	}

	if customerID != "" {
		opts.CustomerID = customerID
	} else {
		opts.CustomerEmail = user.Email
	}

	session, err := c.stripe.CreateCheckoutSession(opts)
	if err != nil {
		c.RenderError(w, r, fmt.Errorf("failed to create checkout: %w", err))
		return
	}

	// Record pending payment
	models.Payments.Insert(&models.Payment{
		UserID:          user.ID,
		StripePaymentID: session.ID,
		ProductType:     models.PaymentPromotion,
		SubjectID:       appID,
		Amount:          amount,
		Currency:        "usd",
		Status:          models.PaymentPending,
	})

	// Use http.Redirect for external Stripe URLs (not c.Redirect which is for internal HTMX navigation)
	http.Redirect(w, r, session.URL, http.StatusSeeOther)
}

func (c *PaymentsController) checkoutUpgrade(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, _ := auth.Authenticate(r)

	appID := r.PathValue("app")
	app, err := models.Apps.Get(appID)
	if err != nil {
		c.RenderError(w, r, errors.New("app not found"))
		return
	}

	// Verify ownership
	repo := app.Repo()
	if repo == nil || repo.OwnerID != user.ID {
		c.RenderError(w, r, errors.New("you can only upgrade your own apps"))
		return
	}

	// Parse upgrade options
	cpuCores, _ := strconv.ParseFloat(r.FormValue("cpu"), 64)
	storageGB, _ := strconv.Atoi(r.FormValue("storage"))

	// Calculate total price for validation
	// CPU: $5/core/month, Storage: $0.25/GB/month
	totalPrice := int64(cpuCores*500) + int64(storageGB*25)

	if totalPrice <= 0 {
		c.RenderError(w, r, errors.New("please select resources to upgrade"))
		return
	}

	// Get profile for Stripe customer
	profile, _ := models.Profiles.First("WHERE UserID = ?", user.ID)
	customerID := ""
	if profile != nil && profile.StripeCustomerID != "" {
		customerID = profile.StripeCustomerID
	}

	baseURL := "https://www.theskyscape.com"
	if prefix := os.Getenv("PREFIX"); prefix != "" {
		baseURL = "https://" + prefix + ".theskyscape.com"
	}

	// Build line items using pre-configured Stripe prices
	catalog, err := c.stripe.GetCatalog()
	if err != nil {
		c.RenderError(w, r, fmt.Errorf("payment system not configured: %w", err))
		return
	}
	var lineItems []payments.LineItem
	if cpuCores > 0 {
		// Use half-cores as unit ($2.50 per 0.5 cores = $5/core)
		halfCores := int64(cpuCores * 2)
		lineItems = append(lineItems, payments.LineItem{
			PriceID:  catalog.CPUPriceID,
			Quantity: halfCores,
		})
	}
	if storageGB > 0 {
		lineItems = append(lineItems, payments.LineItem{
			PriceID:  catalog.StoragePriceID,
			Quantity: int64(storageGB),
		})
	}

	opts := payments.CheckoutOptions{
		Mode:       payments.ModeSubscription,
		SuccessURL: baseURL + "/checkout/success?session_id={CHECKOUT_SESSION_ID}",
		CancelURL:  baseURL + "/app/" + appID + "/manage",
		LineItems:  lineItems,
		Metadata: map[string]string{
			"user_id":      user.ID,
			"product_type": models.PaymentResourceUpgrade,
			"app_id":       appID,
			"cpu_cores":    fmt.Sprintf("%.1f", cpuCores),
			"storage_gb":   strconv.Itoa(storageGB),
		},
	}

	if customerID != "" {
		opts.CustomerID = customerID
	} else {
		opts.CustomerEmail = user.Email
	}

	session, err := c.stripe.CreateCheckoutSession(opts)
	if err != nil {
		c.RenderError(w, r, fmt.Errorf("failed to create checkout: %w", err))
		return
	}

	// Record pending payment
	models.Payments.Insert(&models.Payment{
		UserID:          user.ID,
		StripePaymentID: session.ID,
		ProductType:     models.PaymentResourceUpgrade,
		SubjectID:       appID,
		Amount:          totalPrice,
		Currency:        "usd",
		Status:          models.PaymentPending,
	})

	// Use http.Redirect for external Stripe URLs (not c.Redirect which is for internal HTMX navigation)
	http.Redirect(w, r, session.URL, http.StatusSeeOther)
}

// Webhook handler

func (c *PaymentsController) handleWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	signature := r.Header.Get("Stripe-Signature")
	event, err := c.stripe.VerifyWebhook(payload, signature)
	if err != nil {
		log.Printf("[Stripe Webhook] Signature verification failed: %v", err)
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}

	log.Printf("[Stripe Webhook] Received event: %s", event.Type)

	switch event.Type {
	case payments.EventCheckoutCompleted:
		c.handleCheckoutCompleted(event)
	case payments.EventSubscriptionUpdated:
		c.handleSubscriptionUpdated(event)
	case payments.EventSubscriptionDeleted:
		c.handleSubscriptionDeleted(event)
	}

	w.WriteHeader(http.StatusOK)
}

func (c *PaymentsController) handleCheckoutCompleted(event *payments.Event) {
	metadata, err := event.Metadata()
	if err != nil {
		log.Printf("[Stripe Webhook] Failed to get metadata: %v", err)
		return
	}

	userID := metadata["user_id"]
	productType := metadata["product_type"]

	session, err := event.CheckoutSessionEvent()
	if err != nil {
		log.Printf("[Stripe Webhook] Failed to parse checkout session: %v", err)
		return
	}

	// Update payment record
	payment := models.GetPaymentByStripeID(session.ID)
	if payment != nil {
		payment.MarkCompleted()
	}

	switch productType {
	case models.PaymentVerified:
		c.activateVerified(userID, session)

	case models.PaymentPromotion:
		appID := metadata["app_id"]
		daysStr := metadata["days"]
		content := metadata["content"]
		days, _ := strconv.Atoi(daysStr)
		if days < 1 {
			days = 7
		}
		c.createPromotion(userID, appID, content, days, payment)

	case models.PaymentResourceUpgrade:
		appID := metadata["app_id"]
		cpuCores, _ := strconv.ParseFloat(metadata["cpu_cores"], 64)
		storageGB, _ := strconv.Atoi(metadata["storage_gb"])
		c.activateResourceUpgrade(userID, appID, session, cpuCores, storageGB)
	}
}

func (c *PaymentsController) activateVerified(userID string, session *payments.CheckoutSession) {
	profile, err := models.Profiles.First("WHERE UserID = ?", userID)
	if err != nil {
		log.Printf("[Stripe Webhook] Profile not found for user %s", userID)
		return
	}

	profile.Verified = true
	if session.CustomerID != "" {
		profile.StripeCustomerID = session.CustomerID
	}
	models.Profiles.Update(profile)

	// Create subscription record
	if session.SubscriptionID != "" {
		sub, err := c.stripe.GetSubscription(session.SubscriptionID)
		if err == nil {
			models.Subscriptions.Insert(&models.Subscription{
				UserID:               userID,
				StripeCustomerID:     session.CustomerID,
				StripeSubscriptionID: session.SubscriptionID,
				ProductType:          models.ProductVerified,
				Status:               sub.Status,
				CurrentPeriodEnd:     time.Unix(sub.CurrentPeriodEnd, 0),
			})
		}
	}

	log.Printf("[Stripe Webhook] Activated verification for user %s", userID)
}

func (c *PaymentsController) createPromotion(userID, appID, content string, days int, payment *models.Payment) {
	paymentID := ""
	if payment != nil {
		paymentID = payment.ID
	}

	duration := time.Duration(days) * 24 * time.Hour
	models.Promotions.Insert(&models.Promotion{
		UserID:      userID,
		SubjectType: "app",
		SubjectID:   appID,
		Content:     content,
		ExpiresAt:   time.Now().Add(duration),
		PaymentID:   paymentID,
		IsPaid:      true,
	})

	log.Printf("[Stripe Webhook] Created %d-day promotion for app %s", days, appID)
}

func (c *PaymentsController) activateResourceUpgrade(userID, appID string, session *payments.CheckoutSession, cpuCores float64, storageGB int) {
	// Create subscription record
	if session.SubscriptionID != "" {
		sub, err := c.stripe.GetSubscription(session.SubscriptionID)
		if err == nil {
			models.Subscriptions.Insert(&models.Subscription{
				UserID:               userID,
				StripeCustomerID:     session.CustomerID,
				StripeSubscriptionID: session.SubscriptionID,
				ProductType:          models.ProductAppResources,
				SubjectID:            appID,
				Status:               sub.Status,
				CurrentPeriodEnd:     time.Unix(sub.CurrentPeriodEnd, 0),
			})
		}
	}

	// TODO: Apply the actual resource upgrade via headquarters
	// This would update XFS quotas and container resource limits
	log.Printf("[Stripe Webhook] Resource upgrade for app %s: CPU=%.1f, Storage=%dGB", appID, cpuCores, storageGB)
}

func (c *PaymentsController) handleSubscriptionUpdated(event *payments.Event) {
	sub, err := event.SubscriptionEvent()
	if err != nil {
		log.Printf("[Stripe Webhook] Failed to parse subscription: %v", err)
		return
	}

	// Find subscription by Stripe ID
	subscription, err := models.Subscriptions.First("WHERE StripeSubscriptionID = ?", sub.ID)
	if err != nil {
		log.Printf("[Stripe Webhook] Subscription not found: %s", sub.ID)
		return
	}

	// Update status
	subscription.Status = sub.Status
	subscription.CurrentPeriodEnd = time.Unix(sub.CurrentPeriodEnd, 0)
	if sub.CanceledAt != nil {
		t := time.Unix(*sub.CanceledAt, 0)
		subscription.CanceledAt = &t
	}
	models.Subscriptions.Update(subscription)

	log.Printf("[Stripe Webhook] Updated subscription %s: status=%s", sub.ID, sub.Status)
}

func (c *PaymentsController) handleSubscriptionDeleted(event *payments.Event) {
	sub, err := event.SubscriptionEvent()
	if err != nil {
		log.Printf("[Stripe Webhook] Failed to parse subscription: %v", err)
		return
	}

	// Find subscription by Stripe ID
	subscription, err := models.Subscriptions.First("WHERE StripeSubscriptionID = ?", sub.ID)
	if err != nil {
		log.Printf("[Stripe Webhook] Subscription not found for deletion: %s", sub.ID)
		return
	}

	// Mark as canceled
	subscription.Status = models.StatusCanceled
	now := time.Now()
	subscription.CanceledAt = &now
	models.Subscriptions.Update(subscription)

	// If this was a verified subscription, remove verification
	if subscription.ProductType == models.ProductVerified {
		profile, err := models.Profiles.First("WHERE UserID = ?", subscription.UserID)
		if err == nil {
			profile.Verified = false
			models.Profiles.Update(profile)
		}
	}

	log.Printf("[Stripe Webhook] Deleted subscription %s", sub.ID)
}

// Billing portal

func (c *PaymentsController) billingPortal(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, _ := auth.Authenticate(r)

	profile, err := models.Profiles.First("WHERE UserID = ?", user.ID)
	if err != nil || profile.StripeCustomerID == "" {
		c.RenderError(w, r, errors.New("no billing account found"))
		return
	}

	baseURL := "https://www.theskyscape.com"
	if prefix := os.Getenv("PREFIX"); prefix != "" {
		baseURL = "https://" + prefix + ".theskyscape.com"
	}

	portalURL, err := c.stripe.CreatePortalSession(profile.StripeCustomerID, baseURL+"/billing")
	if err != nil {
		c.RenderError(w, r, fmt.Errorf("failed to create portal session: %w", err))
		return
	}

	// Use http.Redirect for external Stripe portal URL
	http.Redirect(w, r, portalURL, http.StatusSeeOther)
}
