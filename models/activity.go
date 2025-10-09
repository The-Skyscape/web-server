package models

import "github.com/The-Skyscape/devtools/pkg/application"

func (*Activity) Table() string { return "admin_activities" }

type Activity struct {
	application.Model
	Name string
}
