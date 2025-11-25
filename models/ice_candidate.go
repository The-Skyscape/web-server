package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
)

// ICECandidate stores WebRTC ICE candidates for call signaling
type ICECandidate struct {
	application.Model
	CallID        string // Reference to the call
	SenderID      string // User who sent this candidate
	Candidate     string // ICE candidate string
	SDPMid        string // SDP media stream ID
	SDPMLineIndex int    // SDP media line index
}

func (*ICECandidate) Table() string {
	return "ice_candidates"
}

// Call returns the associated call
func (ic *ICECandidate) Call() *Call {
	call, _ := Calls.Get(ic.CallID)
	return call
}

// Sender returns the profile of the user who sent this candidate
func (ic *ICECandidate) Sender() *Profile {
	profile, _ := Profiles.Get(ic.SenderID)
	return profile
}
