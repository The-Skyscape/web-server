package models

import (
	"log"
	"os"

	"github.com/The-Skyscape/devtools/pkg/emailing"
	"github.com/The-Skyscape/devtools/pkg/emailing/providers"
)

func init() {
	log.Println("API key", os.Getenv("RESEND_API_KEY"))
}

var Emails = emailing.Manage(DB, emailing.WithProvider(
	providers.NewResendProvider(
		os.Getenv("RESEND_API_KEY"),
		"hello@theskyscape.com",
		"The Skyscape",
	),
))
