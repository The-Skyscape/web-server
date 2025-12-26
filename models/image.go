package models

import "github.com/The-Skyscape/devtools/pkg/application"

type Image struct {
	application.Model
	AppID     string // legacy - for App images
	ProjectID string // new - for Project images
	GitHash   string
	Status    string
	Error     string
}

func (*Image) Table() string { return "images" }

func (i *Image) App() *App {
	if i.AppID == "" {
		return nil
	}
	app, err := Apps.Get(i.AppID)
	if err != nil {
		return nil
	}
	return app
}

func (i *Image) Project() *Project {
	if i.ProjectID == "" {
		return nil
	}
	project, err := Projects.Get(i.ProjectID)
	if err != nil {
		return nil
	}
	return project
}

func (i *Image) Repo() *Repo {
	// Legacy path: App -> Repo
	if app := i.App(); app != nil {
		return app.Repo()
	}
	return nil
}
