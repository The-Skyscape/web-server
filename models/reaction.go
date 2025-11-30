package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// Supported emoji reactions (GitHub-style)
var ValidReactions = []string{"thumbsup", "heart", "tada", "smile", "confused", "rocket"}

// Emoji mappings for display
var ReactionEmojis = map[string]string{
	"thumbsup": "👍",
	"heart":    "❤️",
	"tada":     "🎉",
	"smile":    "😄",
	"confused": "😕",
	"rocket":   "🚀",
}

type Reaction struct {
	application.Model
	UserID     string
	ActivityID string
	Emoji      string
}

func (*Reaction) Table() string {
	return "reactions"
}

func (r *Reaction) User() *authentication.User {
	user, _ := Auth.Users.Get(r.UserID)
	return user
}

func (r *Reaction) Activity() *Activity {
	activity, _ := Activities.Get(r.ActivityID)
	return activity
}

func (r *Reaction) EmojiDisplay() string {
	if emoji, ok := ReactionEmojis[r.Emoji]; ok {
		return emoji
	}
	return r.Emoji
}

// IsValidReaction checks if the emoji is a supported reaction type
func IsValidReaction(emoji string) bool {
	for _, valid := range ValidReactions {
		if emoji == valid {
			return true
		}
	}
	return false
}
