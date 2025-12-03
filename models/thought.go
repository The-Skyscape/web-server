package models

import (
	"bytes"
	"html/template"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

// Thought represents a long-form blog post by a user
type Thought struct {
	application.Model
	UserID      string
	Title       string
	Content     string // Legacy markdown content (deprecated, use Blocks)
	Slug        string // URL-friendly slug
	Published   bool   // Draft vs published
	ViewsCount  int    // Cached view count
	StarsCount  int    // Cached star count
	HeaderImage string // Optional header image file ID
}

func (*Thought) Table() string { return "thoughts" }

// User returns the author of this thought
func (t *Thought) User() *authentication.User {
	user, err := Auth.Users.Get(t.UserID)
	if err != nil {
		return nil
	}
	return user
}

// Profile returns the author's profile
func (t *Thought) Profile() *Profile {
	profile, err := Profiles.First("WHERE UserID = ?", t.UserID)
	if err != nil {
		return nil
	}
	return profile
}

// Stars returns all stars on this thought
func (t *Thought) Stars() []*ThoughtStar {
	stars, _ := ThoughtStars.Search("WHERE ThoughtID = ?", t.ID)
	return stars
}

// IsStarredBy returns true if the user has starred this thought
func (t *Thought) IsStarredBy(userID string) bool {
	star, _ := ThoughtStars.First("WHERE ThoughtID = ? AND UserID = ?", t.ID, userID)
	return star != nil
}

// Views returns all views on this thought
func (t *Thought) Views() []*ThoughtView {
	views, _ := ThoughtViews.Search("WHERE ThoughtID = ?", t.ID)
	return views
}

// RecordView records a view from a user (or anonymous via IP)
func (t *Thought) RecordView(userID, ipAddress string) {
	// Check if already viewed
	var existing *ThoughtView
	var err error
	if userID != "" {
		existing, err = ThoughtViews.First("WHERE ThoughtID = ? AND UserID = ?", t.ID, userID)
	} else {
		existing, err = ThoughtViews.First("WHERE ThoughtID = ? AND IPAddress = ?", t.ID, ipAddress)
	}

	if err == nil && existing != nil {
		return // Already viewed
	}

	// Record new view
	ThoughtViews.Insert(&ThoughtView{
		ThoughtID: t.ID,
		UserID:    userID,
		IPAddress: ipAddress,
	})

	// Update cached count
	t.ViewsCount++
	Thoughts.Update(t)
}

// Comments returns all comments on this thought
func (t *Thought) Comments() []*Comment {
	comments, _ := Comments.Search(`
		WHERE SubjectID = ?
		ORDER BY CreatedAt ASC
	`, t.ID)
	return comments
}

// CommentsCount returns the number of comments
func (t *Thought) CommentsCount() int {
	return Comments.Count("WHERE SubjectID = ?", t.ID)
}

// Blocks returns all blocks for this thought ordered by position
func (t *Thought) Blocks() []*ThoughtBlock {
	blocks, _ := ThoughtBlocks.Search("WHERE ThoughtID = ? ORDER BY Position", t.ID)
	return blocks
}

// HasBlocks returns true if this thought uses block-based content
func (t *Thought) HasBlocks() bool {
	return ThoughtBlocks.Count("WHERE ThoughtID = ?", t.ID) > 0
}

// BlocksToMarkdown converts blocks to markdown string
func (t *Thought) BlocksToMarkdown() string {
	blocks := t.Blocks()
	if len(blocks) == 0 {
		return t.Content // Fall back to legacy content
	}

	var result bytes.Buffer
	for i, block := range blocks {
		if i > 0 {
			result.WriteString("\n\n")
		}

		switch block.Type {
		case "heading":
			level := block.HeadingLevel()
			for j := 0; j < level; j++ {
				result.WriteString("#")
			}
			result.WriteString(" ")
			result.WriteString(block.Content)

		case "quote":
			result.WriteString("> ")
			result.WriteString(block.Content)

		case "code":
			result.WriteString("```")
			result.WriteString(block.CodeLanguage())
			result.WriteString("\n")
			result.WriteString(block.Content)
			result.WriteString("\n```")

		case "list":
			items := block.ListItems()
			ordered := block.IsOrdered()
			for j, item := range items {
				if ordered {
					result.WriteString(string(rune('1' + j)))
					result.WriteString(". ")
				} else {
					result.WriteString("- ")
				}
				result.WriteString(item)
				if j < len(items)-1 {
					result.WriteString("\n")
				}
			}

		case "image":
			if file := block.File(); file != nil {
				result.WriteString("![")
				result.WriteString(block.Content) // Alt text
				result.WriteString("](/file/")
				result.WriteString(block.FileID)
				result.WriteString(")")
			}

		case "file":
			if file := block.File(); file != nil {
				result.WriteString("[")
				if block.Content != "" {
					result.WriteString(block.Content) // Label
				} else {
					result.WriteString(file.FilePath)
				}
				result.WriteString("](/file/")
				result.WriteString(block.FileID)
				result.WriteString(")")
			}

		default: // paragraph
			result.WriteString(block.Content)
		}
	}

	return result.String()
}

// Markdown parses the content as markdown and returns sanitized HTML
func (t *Thought) Markdown() template.HTML {
	// Use blocks if available, otherwise fall back to legacy Content
	content := t.BlocksToMarkdown()

	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM, // GitHub Flavored Markdown
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)

	var buf bytes.Buffer
	if err := md.Convert([]byte(content), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(content))
	}

	p := bluemonday.UGCPolicy()
	return template.HTML(p.Sanitize(buf.String()))
}

// ThoughtView tracks individual views of a thought
type ThoughtView struct {
	application.Model
	ThoughtID string
	UserID    string // Empty for anonymous views
	IPAddress string
}

func (*ThoughtView) Table() string { return "thought_views" }

// ThoughtStar represents a user starring a thought
type ThoughtStar struct {
	application.Model
	ThoughtID string
	UserID    string
}

func (*ThoughtStar) Table() string { return "thought_stars" }

// User returns the user who starred
func (s *ThoughtStar) User() *authentication.User {
	user, _ := Auth.Users.Get(s.UserID)
	return user
}
