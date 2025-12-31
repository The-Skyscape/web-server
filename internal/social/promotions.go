package social

import (
	"time"

	"github.com/pkg/errors"
	"www.theskyscape.com/models"
)

// MaxContentLength is the maximum length for promotion content
const MaxContentLength = 500

// Promotable represents an entity that can be promoted
type Promotable interface {
	GetID() string
	GetOwnerID() string
	GetSubjectType() string
	ActivePromotion() *models.Promotion
}

// appPromotable wraps an App to implement Promotable
type appPromotable struct {
	app *models.App
}

func (a *appPromotable) GetID() string      { return a.app.ID }
func (a *appPromotable) GetSubjectType() string { return "app" }
func (a *appPromotable) GetOwnerID() string {
	if repo := a.app.Repo(); repo != nil {
		return repo.OwnerID
	}
	return ""
}
func (a *appPromotable) ActivePromotion() *models.Promotion {
	return a.app.ActivePromotion()
}

// projectPromotable wraps a Project to implement Promotable
type projectPromotable struct {
	project *models.Project
}

func (p *projectPromotable) GetID() string          { return p.project.ID }
func (p *projectPromotable) GetSubjectType() string { return "project" }
func (p *projectPromotable) GetOwnerID() string     { return p.project.OwnerID }
func (p *projectPromotable) ActivePromotion() *models.Promotion {
	return p.project.ActivePromotion()
}

// WrapApp wraps an App to implement Promotable
func WrapApp(app *models.App) Promotable {
	return &appPromotable{app: app}
}

// WrapProject wraps a Project to implement Promotable
func WrapProject(project *models.Project) Promotable {
	return &projectPromotable{project: project}
}

// CreatePromotion creates a new promotion for the given entity.
// Returns an error if the user doesn't own the entity, if there's already
// an active promotion, or if the content is too long.
func CreatePromotion(userID string, entity Promotable, content string) (*models.Promotion, error) {
	// Check ownership
	if entity.GetOwnerID() != userID {
		return nil, errors.New("you can only promote your own content")
	}

	// Check for existing promotion
	if existing := entity.ActivePromotion(); existing != nil {
		return nil, errors.New("this already has an active promotion")
	}

	// Validate content length
	if len(content) > MaxContentLength {
		return nil, errors.New("promotion content too long")
	}

	// Create the promotion
	promo := &models.Promotion{
		UserID:      userID,
		SubjectType: entity.GetSubjectType(),
		SubjectID:   entity.GetID(),
		Content:     content,
		ExpiresAt:   time.Now().Add(models.DefaultPromotionDuration),
	}

	return models.Promotions.Insert(promo)
}

// CancelPromotion cancels the active promotion for the given entity.
// Returns an error if the user doesn't own the entity or if there's no active promotion.
func CancelPromotion(userID string, entity Promotable) error {
	// Check ownership
	if entity.GetOwnerID() != userID {
		return errors.New("you can only cancel your own promotions")
	}

	// Get active promotion
	promo := entity.ActivePromotion()
	if promo == nil {
		return errors.New("no active promotion found")
	}

	return models.Promotions.Delete(promo)
}
