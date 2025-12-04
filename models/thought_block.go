package models

import "github.com/The-Skyscape/devtools/pkg/application"

// ThoughtBlock represents a content block within a thought
type ThoughtBlock struct {
	application.Model
	ThoughtID string // Parent thought ID
	Type      string // Block type: paragraph, image
	Content   string // Markdown text or image caption
	FileID    string // File reference for image blocks
	Position  int    // Order within the thought
}

func (*ThoughtBlock) Table() string { return "thought_blocks" }

// File returns the associated file for image blocks
func (b *ThoughtBlock) File() *File {
	if b.FileID == "" {
		return nil
	}
	file, err := Files.Get(b.FileID)
	if err != nil {
		return nil
	}
	return file
}
