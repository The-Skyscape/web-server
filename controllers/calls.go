package controllers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"www.theskyscape.com/models"
)

func Calls() (string, *CallsController) {
	return "calls", &CallsController{
		sseClients: make(map[string]chan CallEvent),
		mutex:      &sync.RWMutex{},
	}
}

type CallsController struct {
	application.Controller
	sseClients map[string]chan CallEvent // userID -> event channel
	mutex      *sync.RWMutex
}

// CallEvent represents an SSE event for calls
type CallEvent struct {
	Type    string      `json:"type"`
	CallID  string      `json:"callId"`
	Payload interface{} `json:"payload"`
}

func (c *CallsController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := c.Use("auth").(*AuthController)

	// SSE endpoint for receiving call events
	http.Handle("GET /calls/events", c.ProtectFunc(c.sseHandler, auth.Required))

	// Call lifecycle endpoints
	http.Handle("POST /calls/{id}/initiate", c.ProtectFunc(c.initiateCall, auth.Required))
	http.Handle("POST /calls/{id}/accept", c.ProtectFunc(c.acceptCall, auth.Required))
	http.Handle("POST /calls/{id}/reject", c.ProtectFunc(c.rejectCall, auth.Required))
	http.Handle("POST /calls/{id}/end", c.ProtectFunc(c.endCall, auth.Required))

	// WebRTC signaling endpoints
	http.Handle("POST /calls/{id}/sdp", c.ProtectFunc(c.exchangeSDP, auth.Required))
	http.Handle("POST /calls/{id}/ice", c.ProtectFunc(c.addICECandidate, auth.Required))

	// Get TURN credentials
	http.Handle("GET /calls/turn-credentials", c.ProtectFunc(c.getTURNCredentials, auth.Required))
}

func (c CallsController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

func (c *CallsController) currentUser(r *http.Request) *models.Profile {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		return nil
	}
	profile, _ := models.Profiles.Get(user.ID)
	return profile
}

// sseHandler maintains SSE connection for real-time call events
func (c *CallsController) sseHandler(w http.ResponseWriter, r *http.Request) {
	user := c.currentUser(r)
	if user == nil {
		log.Printf("[SSE] Auth failed for %s", r.RemoteAddr)
		JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	log.Printf("[SSE] Connection from user %s (%s)", user.Handle(), user.ID)

	// Set SSE headers - must be set before WriteHeader
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	// Try to prevent LB from buffering
	w.Header().Set("Transfer-Encoding", "chunked")

	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Printf("[SSE] Flusher not supported for user %s", user.ID)
		JSONError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Create event channel for this user
	eventChan := make(chan CallEvent, 10)

	c.mutex.Lock()
	// Close existing channel if user reconnects
	if oldChan, exists := c.sseClients[user.ID]; exists {
		log.Printf("[SSE] Closing old channel for user %s", user.ID)
		close(oldChan)
	}
	c.sseClients[user.ID] = eventChan
	c.mutex.Unlock()

	defer func() {
		log.Printf("[SSE] Connection closed for user %s", user.ID)
		c.mutex.Lock()
		if c.sseClients[user.ID] == eventChan {
			delete(c.sseClients, user.ID)
		}
		c.mutex.Unlock()
	}()

	// Send initial ping with retry hint for client
	fmt.Fprintf(w, "retry: 5000\nevent: ping\ndata: connected\n\n")
	flusher.Flush()
	log.Printf("[SSE] Sent initial ping to user %s", user.ID)

	// Keep-alive ticker - very aggressive (1 second) to work around LB timeouts
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				log.Printf("[SSE] Channel closed for user %s", user.ID)
				return // Channel closed
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
			flusher.Flush()
			log.Printf("[SSE] Sent event %s to user %s", event.Type, user.ID)

		case <-ticker.C:
			// Send comment instead of event (lighter weight, still keeps connection alive)
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()

		case <-r.Context().Done():
			log.Printf("[SSE] Context done for user %s: %v", user.ID, r.Context().Err())
			return
		}
	}
}

// sendEvent sends an event to a specific user
func (c *CallsController) sendEvent(userID string, event CallEvent) {
	c.mutex.RLock()
	ch, ok := c.sseClients[userID]
	c.mutex.RUnlock()

	if ok {
		select {
		case ch <- event:
		default:
			// Channel full, drop event (user might be offline)
		}
	}
}

// initiateCall starts a new call to another user
func (c *CallsController) initiateCall(w http.ResponseWriter, r *http.Request) {
	caller := c.currentUser(r)
	if caller == nil {
		JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	calleeID := r.PathValue("id")

	// Check callee exists
	_, err := models.Profiles.Get(calleeID)
	if err != nil {
		JSONError(w, http.StatusNotFound, "user not found")
		return
	}

	// Can't call yourself
	if caller.ID == calleeID {
		JSONError(w, http.StatusBadRequest, "cannot call yourself")
		return
	}

	// Check for existing active call for caller
	existingCall, _ := models.Calls.First(
		"WHERE (CallerID = ? OR CalleeID = ?) AND Status IN ('pending', 'ringing', 'active')",
		caller.ID, caller.ID,
	)
	if existingCall != nil {
		JSONError(w, http.StatusConflict, "already in a call")
		return
	}

	// Create call record
	call, err := models.Calls.Insert(&models.Call{
		CallerID: caller.ID,
		CalleeID: calleeID,
		Status:   "pending",
	})
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "failed to create call")
		return
	}

	// Notify callee via SSE
	c.sendEvent(calleeID, CallEvent{
		Type:   "call_incoming",
		CallID: call.ID,
		Payload: map[string]interface{}{
			"callerId":     caller.ID,
			"callerName":   caller.Name,
			"callerHandle": caller.Handle,
			"callerAvatar": caller.Avatar,
		},
	})

	JSONSuccess(w, map[string]string{
		"callId": call.ID,
		"status": "pending",
	})
}

// acceptCall accepts an incoming call
func (c *CallsController) acceptCall(w http.ResponseWriter, r *http.Request) {
	user := c.currentUser(r)
	if user == nil {
		JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	callID := r.PathValue("id")
	call, err := models.Calls.Get(callID)
	if err != nil {
		JSONError(w, http.StatusNotFound, "call not found")
		return
	}

	if call.CalleeID != user.ID {
		JSONError(w, http.StatusForbidden, "not your call")
		return
	}

	if !call.IsPending() {
		JSONError(w, http.StatusBadRequest, "call cannot be accepted")
		return
	}

	if err := call.Accept(); err != nil {
		JSONError(w, http.StatusInternalServerError, "failed to accept call")
		return
	}

	// Notify caller that call was accepted
	c.sendEvent(call.CallerID, CallEvent{
		Type:   "call_accepted",
		CallID: call.ID,
		Payload: map[string]interface{}{
			"calleeId": user.ID,
		},
	})

	JSONSuccess(w, map[string]string{"status": "active"})
}

// rejectCall rejects an incoming call or cancels an outgoing call
func (c *CallsController) rejectCall(w http.ResponseWriter, r *http.Request) {
	user := c.currentUser(r)
	if user == nil {
		JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	callID := r.PathValue("id")
	call, err := models.Calls.Get(callID)
	if err != nil {
		JSONError(w, http.StatusNotFound, "call not found")
		return
	}

	if call.CalleeID != user.ID && call.CallerID != user.ID {
		JSONError(w, http.StatusForbidden, "not your call")
		return
	}

	reason := "rejected"
	otherID := call.CallerID
	if user.ID == call.CallerID {
		otherID = call.CalleeID
		reason = "cancelled"
	}

	if err := call.End(reason); err != nil {
		JSONError(w, http.StatusInternalServerError, "failed to reject call")
		return
	}

	// Notify the other party
	c.sendEvent(otherID, CallEvent{
		Type:   "call_ended",
		CallID: call.ID,
		Payload: map[string]string{
			"reason": reason,
		},
	})

	JSONSuccess(w, map[string]string{"status": "ended"})
}

// endCall ends an active call
func (c *CallsController) endCall(w http.ResponseWriter, r *http.Request) {
	user := c.currentUser(r)
	if user == nil {
		JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	callID := r.PathValue("id")
	call, err := models.Calls.Get(callID)
	if err != nil {
		JSONError(w, http.StatusNotFound, "call not found")
		return
	}

	if call.CalleeID != user.ID && call.CallerID != user.ID {
		JSONError(w, http.StatusForbidden, "not your call")
		return
	}

	if err := call.End("completed"); err != nil {
		JSONError(w, http.StatusInternalServerError, "failed to end call")
		return
	}

	otherID := call.CallerID
	if user.ID == call.CallerID {
		otherID = call.CalleeID
	}

	c.sendEvent(otherID, CallEvent{
		Type:   "call_ended",
		CallID: call.ID,
		Payload: map[string]interface{}{
			"reason":   "completed",
			"duration": call.Duration,
		},
	})

	JSONSuccess(w, map[string]interface{}{
		"status":   "ended",
		"duration": call.Duration,
	})
}

// exchangeSDP handles SDP offer/answer exchange
func (c *CallsController) exchangeSDP(w http.ResponseWriter, r *http.Request) {
	user := c.currentUser(r)
	if user == nil {
		JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	callID := r.PathValue("id")
	call, err := models.Calls.Get(callID)
	if err != nil {
		JSONError(w, http.StatusNotFound, "call not found")
		return
	}

	if call.CalleeID != user.ID && call.CallerID != user.ID {
		JSONError(w, http.StatusForbidden, "not your call")
		return
	}

	var payload struct {
		Type string `json:"type"`
		SDP  string `json:"sdp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid request")
		return
	}

	otherID := call.CallerID
	eventType := "sdp_answer"

	if user.ID == call.CallerID {
		otherID = call.CalleeID
		eventType = "sdp_offer"
	}

	c.sendEvent(otherID, CallEvent{
		Type:   eventType,
		CallID: call.ID,
		Payload: map[string]string{
			"type": payload.Type,
			"sdp":  payload.SDP,
		},
	})

	JSONSuccess(w, map[string]string{"status": "sent"})
}

// addICECandidate handles ICE candidate exchange
func (c *CallsController) addICECandidate(w http.ResponseWriter, r *http.Request) {
	user := c.currentUser(r)
	if user == nil {
		JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	callID := r.PathValue("id")
	call, err := models.Calls.Get(callID)
	if err != nil {
		JSONError(w, http.StatusNotFound, "call not found")
		return
	}

	if call.CalleeID != user.ID && call.CallerID != user.ID {
		JSONError(w, http.StatusForbidden, "not your call")
		return
	}

	var payload struct {
		Candidate     string `json:"candidate"`
		SDPMid        string `json:"sdpMid"`
		SDPMLineIndex int    `json:"sdpMLineIndex"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid request")
		return
	}

	// Store the ICE candidate
	models.ICECandidates.Insert(&models.ICECandidate{
		CallID:        callID,
		SenderID:      user.ID,
		Candidate:     payload.Candidate,
		SDPMid:        payload.SDPMid,
		SDPMLineIndex: payload.SDPMLineIndex,
	})

	otherID := call.CallerID
	if user.ID == call.CallerID {
		otherID = call.CalleeID
	}

	c.sendEvent(otherID, CallEvent{
		Type:    "ice_candidate",
		CallID:  call.ID,
		Payload: payload,
	})

	JSONSuccess(w, map[string]string{"status": "sent"})
}

// getTURNCredentials returns ICE server configuration
// TODO: Add TURN server support for users behind strict NAT/firewalls
func (c *CallsController) getTURNCredentials(w http.ResponseWriter, r *http.Request) {
	user := c.currentUser(r)
	if user == nil {
		JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// For now, only use public STUN servers
	// This works for most users but won't work behind strict NAT/firewalls
	iceServers := []map[string]interface{}{
		{
			"urls": []string{
				"stun:stun.l.google.com:19302",
				"stun:stun1.l.google.com:19302",
				"stun:stun2.l.google.com:19302",
			},
		},
	}

	JSONSuccess(w, map[string]interface{}{
		"iceServers": iceServers,
	})
}
