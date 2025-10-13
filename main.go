package main

import (
	"embed"
	"os"

	"github.com/The-Skyscape/devtools/pkg/application"
	"www.theskyscape.com/controllers"
)

//go:embed all:views
var views embed.FS

func main() {
	application.Serve(views,
		application.WithDaisyTheme("dark"),
		application.WithHostPrefix(os.Getenv("PREFIX")),
		application.WithController(controllers.Auth()),
		application.WithController(controllers.Feed()),
	)
}
