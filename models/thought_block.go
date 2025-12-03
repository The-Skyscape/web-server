package models

import (
	"encoding/json"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// ThoughtBlock represents a content block within a thought
type ThoughtBlock struct {
	application.Model
	ThoughtID string // Parent thought ID
	Type      string // Block type: paragraph, heading, quote, code, list, image, file
	Content   string // Text content or caption/alt for media blocks
	FileID    string // Optional file reference for image/file blocks
	Position  int    // Order within the thought
	Metadata  string // JSON for type-specific data (heading level, code language, etc)
}

func (*ThoughtBlock) Table() string { return "thought_blocks" }

// File returns the associated file for image/file blocks
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

// Meta parses the metadata JSON into a map
func (b *ThoughtBlock) Meta() map[string]any {
	if b.Metadata == "" {
		return nil
	}
	var meta map[string]any
	json.Unmarshal([]byte(b.Metadata), &meta)
	return meta
}

// SetMeta serializes a map to JSON metadata
func (b *ThoughtBlock) SetMeta(meta map[string]any) {
	if meta == nil {
		b.Metadata = ""
		return
	}
	data, _ := json.Marshal(meta)
	b.Metadata = string(data)
}

// HeadingLevel returns the heading level (1-3) for heading blocks
func (b *ThoughtBlock) HeadingLevel() int {
	if b.Type != "heading" {
		return 0
	}
	meta := b.Meta()
	if meta == nil {
		return 1
	}
	if level, ok := meta["level"].(float64); ok {
		return int(level)
	}
	return 1
}

// CodeLanguage returns the language for code blocks
func (b *ThoughtBlock) CodeLanguage() string {
	if b.Type != "code" {
		return ""
	}
	meta := b.Meta()
	if meta == nil {
		return ""
	}
	if lang, ok := meta["language"].(string); ok {
		return lang
	}
	return ""
}

// ListItems returns the items for list blocks
func (b *ThoughtBlock) ListItems() []string {
	if b.Type != "list" {
		return nil
	}
	meta := b.Meta()
	if meta == nil {
		return nil
	}
	if items, ok := meta["items"].([]any); ok {
		result := make([]string, len(items))
		for i, item := range items {
			if s, ok := item.(string); ok {
				result[i] = s
			}
		}
		return result
	}
	return nil
}

// IsOrdered returns true if this is an ordered list
func (b *ThoughtBlock) IsOrdered() bool {
	if b.Type != "list" {
		return false
	}
	meta := b.Meta()
	if meta == nil {
		return false
	}
	if ordered, ok := meta["ordered"].(bool); ok {
		return ordered
	}
	return false
}
