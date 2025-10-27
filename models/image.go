package models

import "github.com/The-Skyscape/devtools/pkg/application"

type Image struct {
	application.Model
	AppID   string
	GitHash string
	Status  string
	Error   string
}

func (*Image) Table() string { return "images" }

func (i *Image) App() *App {
	app, err := Apps.Get(i.AppID)
	if err != nil {
		return nil
	}

	return app
}

func (i *Image) Repo() *Repo {
	app := i.App()
	if app == nil {
		return nil
	}

	return app.Repo()
}
