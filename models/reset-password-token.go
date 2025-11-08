package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

type ResetPasswordToken struct {
	application.Model
	UserID string
	Token  string
}

func (*ResetPasswordToken) Table() string {
	return "password_reset_tokens"
}

func (p *ResetPasswordToken) User() *authentication.User {
	user, err := Auth.Users.Get(p.UserID)
	if err != nil {
		return nil
	}

	return user
}
