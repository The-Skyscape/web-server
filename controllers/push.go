package controllers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/The-Skyscape/devtools/pkg/application"
	"www.theskyscape.com/internal/push"
	"www.theskyscape.com/models"
)

func Push() (string, *PushController) {
	return "push", &PushController{}
}

type PushController struct {
	application.Controller
}

func (c *PushController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := c.Use("auth").(*AuthController)

	// API endpoints for push subscription management
	http.Handle("GET /api/push/vapid-key", c.ProtectFunc(c.getVAPIDKey, auth.Required))
	http.Handle("POST /api/push/subscribe", c.ProtectFunc(c.subscribe, auth.Required))
	http.Handle("DELETE /api/push/subscribe", c.ProtectFunc(c.unsubscribe, auth.Required))
}

func (c PushController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

// getVAPIDKey returns the public VAPID key for client-side subscription
func (c *PushController) getVAPIDKey(w http.ResponseWriter, r *http.Request) {
	log.Println("[Push] VAPID key requested")
	publicKey := push.GetPublicKey()
	if publicKey == "" {
		log.Println("[Push] VAPID public key not configured")
		JSONError(w, http.StatusServiceUnavailable, "push notifications not configured")
		return
	}

	log.Printf("[Push] Returning VAPID public key: %s...", publicKey[:20])
	JSONSuccess(w, map[string]string{
		"publicKey": publicKey,
	})
}

// SubscriptionRequest represents the push subscription from the browser
type SubscriptionRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// subscribe saves a push subscription for the authenticated user
func (c *PushController) subscribe(w http.ResponseWriter, r *http.Request) {
	log.Println("[Push] Subscribe request received")
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		log.Printf("[Push] Subscribe auth failed: %v", err)
		JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	log.Printf("[Push] Subscribe for user: %s", user.ID)

	var req SubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[Push] Subscribe JSON decode failed: %v", err)
		JSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Endpoint == "" || req.Keys.P256dh == "" || req.Keys.Auth == "" {
		log.Println("[Push] Subscribe missing data")
		JSONError(w, http.StatusBadRequest, "missing subscription data")
		return
	}
	log.Printf("[Push] Subscription endpoint: %s...", req.Endpoint[:min(80, len(req.Endpoint))])
	log.Printf("[Push] Subscription keys - P256dh length: %d, Auth length: %d", len(req.Keys.P256dh), len(req.Keys.Auth))

	// Check if subscription already exists for this endpoint
	existing, _ := models.PushSubscriptions.First(
		"WHERE UserID = ? AND Endpoint = ?",
		user.ID, req.Endpoint,
	)

	if existing != nil {
		// Update existing subscription
		log.Printf("[Push] Updating existing subscription %s", existing.ID)
		existing.P256dh = req.Keys.P256dh
		existing.Auth = req.Keys.Auth
		if err := models.PushSubscriptions.Update(existing); err != nil {
			log.Printf("[Push] Update failed: %v", err)
			JSONError(w, http.StatusInternalServerError, "failed to update subscription")
			return
		}
	} else {
		// Create new subscription
		log.Println("[Push] Creating new subscription")
		sub, err := models.PushSubscriptions.Insert(&models.PushSubscription{
			UserID:   user.ID,
			Endpoint: req.Endpoint,
			P256dh:   req.Keys.P256dh,
			Auth:     req.Keys.Auth,
		})
		if err != nil {
			log.Printf("[Push] Insert failed: %v", err)
			JSONError(w, http.StatusInternalServerError, "failed to save subscription")
			return
		}
		log.Printf("[Push] Created subscription %s for user %s", sub.ID, user.ID)
	}

	JSONSuccess(w, map[string]string{
		"status": "subscribed",
	})
}

// unsubscribe removes a push subscription
func (c *PushController) unsubscribe(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req SubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Delete subscription by endpoint
	sub, _ := models.PushSubscriptions.First(
		"WHERE UserID = ? AND Endpoint = ?",
		user.ID, req.Endpoint,
	)

	if sub != nil {
		models.PushSubscriptions.Delete(sub)
	}

	JSONSuccess(w, map[string]string{
		"status": "unsubscribed",
	})
}
