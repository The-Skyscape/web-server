package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

type File struct {
	application.Model
	OwnerID  string
	FilePath string
	MimeType string
	Content  []byte
}

func (*File) Table() string { return "files" }

func (f *File) Owner() *authentication.User {
	user, err := Auth.Users.Get(f.OwnerID)
	if err != nil {
		return nil
	}

	return user
}
