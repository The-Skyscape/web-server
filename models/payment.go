package models

import (
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// PaymentProductType identifies what the payment is for
const (
	PaymentPromotion       = "promotion"
	PaymentVerified        = "verified"
	PaymentResourceUpgrade = "resource_upgrade"
)

// PaymentStatus represents the state of a payment
const (
	PaymentPending   = "pending"
	PaymentCompleted = "completed"
	PaymentFailed    = "failed"
	PaymentRefunded  = "refunded"
)

type Payment struct {
	application.Model
	UserID          string
	StripePaymentID string // Checkout session ID or payment intent ID
	ProductType     string // "promotion", "verified", "resource_upgrade"
	SubjectID       string // App ID for promotions/upgrades
	Amount          int64  // Amount in cents
	Currency        string // "usd"
	Status          string // "pending", "completed", "failed", "refunded"
	CompletedAt     *time.Time
}

func (*Payment) Table() string { return "payments" }

// User returns the payment owner
func (p *Payment) User() *authentication.User {
	user, _ := Auth.Users.Get(p.UserID)
	return user
}

// App returns the app if this payment is for an app
func (p *Payment) App() *App {
	if p.SubjectID == "" {
		return nil
	}
	app, _ := Apps.Get(p.SubjectID)
	return app
}

// MarkCompleted marks the payment as completed
func (p *Payment) MarkCompleted() error {
	p.Status = PaymentCompleted
	now := time.Now()
	p.CompletedAt = &now
	return Payments.Update(p)
}

// MarkFailed marks the payment as failed
func (p *Payment) MarkFailed() error {
	p.Status = PaymentFailed
	return Payments.Update(p)
}

// GetPaymentByStripeID retrieves a payment by Stripe ID
func GetPaymentByStripeID(stripeID string) *Payment {
	payment, err := Payments.First("WHERE StripePaymentID = ?", stripeID)
	if err != nil {
		return nil
	}
	return payment
}

// UserPayments returns all payments for a user
func UserPayments(userID string, limit int) []*Payment {
	payments, _ := Payments.Search(`
		WHERE UserID = ?
		ORDER BY CreatedAt DESC
		LIMIT ?
	`, userID, limit)
	return payments
}

// FormatAmount returns the amount formatted as currency
func (p *Payment) FormatAmount() string {
	dollars := float64(p.Amount) / 100
	return "$" + formatFloat(dollars)
}

func formatFloat(f float64) string {
	dollars := int64(f)
	cents := int64(f*100+0.5) % 100
	if cents == 0 {
		return intToString(dollars)
	}
	return intToString(dollars) + "." + padZero(cents)
}

func intToString(n int64) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func padZero(n int64) string {
	if n < 10 {
		return "0" + intToString(n)
	}
	return intToString(n)
}
