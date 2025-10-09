package controllers

import (
	"net/http"

	"github.com/The-Skyscape/devtools/pkg/application"
)

func Home() (string, *HomeController) {
	return "home", &HomeController{}
}

type HomeController struct {
	application.Controller
}

func (c *HomeController) Setup(app *application.App) {
	c.Controller.Setup(app)

	http.Handle("/", app.Serve("homepage.html", nil))
}

func (c HomeController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}
