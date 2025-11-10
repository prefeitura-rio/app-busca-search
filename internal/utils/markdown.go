package utils

import (
	"bytes"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
)

// StripMarkdown removes all markdown formatting from text and returns plain text
func StripMarkdown(text string) string {
	if text == "" {
		return ""
	}

	// Parse markdown to AST
	doc := markdown.Parse([]byte(text), nil)

	// Extract plain text from AST
	var buf bytes.Buffer
	extractText(doc, &buf)

	// Clean up extra whitespace
	result := strings.TrimSpace(buf.String())
	result = strings.ReplaceAll(result, "\n\n\n", "\n\n") // Remove triple newlines

	return result
}

// StripMarkdownArray processes an array of markdown strings
func StripMarkdownArray(texts []string) []string {
	if texts == nil {
		return nil
	}

	result := make([]string, len(texts))
	for i, text := range texts {
		result[i] = StripMarkdown(text)
	}
	return result
}

// extractText walks the AST and extracts text content
func extractText(node ast.Node, buf *bytes.Buffer) {
	// Handle leaf nodes
	switch n := node.(type) {
	case *ast.Text:
		buf.Write(n.Literal)
		return

	case *ast.Code:
		buf.Write(n.Literal)
		return

	case *ast.CodeBlock:
		buf.Write(n.Literal)
		return

	case *ast.Hardbreak:
		buf.WriteString("\n")
		return

	case *ast.Softbreak:
		buf.WriteString(" ")
		return

	case *ast.HTMLBlock:
		// Skip HTML blocks entirely
		return

	case *ast.HTMLSpan:
		// Skip HTML spans
		return
	}

	// Handle container nodes
	container := node.AsContainer()
	if container == nil {
		return
	}

	// Special handling for specific node types
	switch node.(type) {
	case *ast.ListItem:
		buf.WriteString("• ") // Use bullet point for list items
	}

	// Process children
	for _, child := range container.Children {
		extractText(child, buf)
	}

	// Add trailing formatting based on node type
	switch node.(type) {
	case *ast.Paragraph:
		buf.WriteString("\n\n")
	case *ast.Heading:
		buf.WriteString("\n\n")
	case *ast.List:
		buf.WriteString("\n")
	case *ast.BlockQuote:
		buf.WriteString("\n")
	}
}
