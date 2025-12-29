package markup

import (
	"bytes"
	"html/template"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
)

var sanitizer = bluemonday.UGCPolicy()

// RenderMarkdown converts markdown to sanitized HTML
func RenderMarkdown(content string) template.HTML {
	var buf bytes.Buffer
	if err := md.Convert([]byte(content), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(content))
	}
	return template.HTML(sanitizer.Sanitize(buf.String()))
}
