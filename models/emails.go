package models

import (
	"os"

	"github.com/The-Skyscape/devtools/pkg/emailing"
	"github.com/The-Skyscape/devtools/pkg/emailing/providers"
)

var Emails = emailing.Manage(DB, emailing.WithProvider(
	providers.NewResendProvider(
		os.Getenv("RESEND_API_KEY"),
		"hello@theskyscape.com",
		"The Skyscape",
	),
))
