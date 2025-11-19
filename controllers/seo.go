package controllers

import (
	"fmt"
	"net/http"

	"github.com/The-Skyscape/devtools/pkg/application"
)

func SEO() (string, *SEOController) {
	return "seo", &SEOController{}
}

type SEOController struct {
	application.Controller
}

func (c *SEOController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := app.Use("auth").(*AuthController)

	http.Handle("GET /robots.txt", app.Serve("robots.txt", auth.Optional))
	http.Handle("GET /sitemap.xml", app.ProtectFunc(c.sitemap, auth.Optional))
	http.Handle("GET /google3c5c81d2e70ab3e1.html", app.Serve("google.html", auth.Optional))
}

func (c SEOController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

func (c *SEOController) sitemap(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`)
	c.Render(w, r, "sitemap.xml", nil)
}
