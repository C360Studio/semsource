// Package svelte provides Svelte 5 AST parsing.
// This file contains template analysis utilities. Currently, only component extraction
// is used by the parser. The additional extractTemplateInfo function provides extended
// template analysis (slots, each/if blocks) for future enhancements.
package svelte

import (
	sitter "github.com/smacker/go-tree-sitter"
)

// TemplateInfo holds information extracted from Svelte template sections
type TemplateInfo struct {
	// Components are the component names used in the template (PascalCase tags)
	Components []string

	// Slots are slot usages in the template
	Slots []SlotInfo

	// EachBlocks are {#each} blocks that iterate over data
	EachBlocks []EachBlockInfo

	// IfBlocks are {#if} conditional blocks
	IfBlocks []IfBlockInfo
}

// SlotInfo represents a <slot> usage in a template
type SlotInfo struct {
	Name     string // slot name (empty for default slot)
	Fallback bool   // whether slot has fallback content
}

// EachBlockInfo represents an {#each} block
type EachBlockInfo struct {
	ItemName string // the iteration variable name
}

// IfBlockInfo represents an {#if} block
type IfBlockInfo struct {
	HasElse bool // whether the block has :else
}

// extractTemplateInfo extracts comprehensive template information from a Svelte AST
func extractTemplateInfo(root *sitter.Node, source []byte) *TemplateInfo {
	info := &TemplateInfo{
		Components: make([]string, 0),
		Slots:      make([]SlotInfo, 0),
		EachBlocks: make([]EachBlockInfo, 0),
		IfBlocks:   make([]IfBlockInfo, 0),
	}

	cursor := sitter.NewTreeCursor(root)
	defer cursor.Close()

	seen := make(map[string]bool)
	walkTemplateTree(cursor, source, info, seen)

	return info
}

// walkTemplateTree recursively walks the template AST to extract information
func walkTemplateTree(cursor *sitter.TreeCursor, source []byte, info *TemplateInfo, seen map[string]bool) {
	node := cursor.CurrentNode()
	nodeType := node.Type()

	switch nodeType {
	case "element", "self_closing_element":
		extractElementInfo(node, source, info, seen)

	case "each_block":
		info.EachBlocks = append(info.EachBlocks, EachBlockInfo{})

	case "if_block":
		hasElse := false
		// Check for else clause
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "else_clause" {
				hasElse = true
				break
			}
		}
		info.IfBlocks = append(info.IfBlocks, IfBlockInfo{HasElse: hasElse})
	}

	// Recurse into children
	if cursor.GoToFirstChild() {
		for {
			walkTemplateTree(cursor, source, info, seen)
			if !cursor.GoToNextSibling() {
				break
			}
		}
		cursor.GoToParent()
	}
}

// extractElementInfo extracts information from an element node
func extractElementInfo(node *sitter.Node, source []byte, info *TemplateInfo, seen map[string]bool) {
	tagName := ""

	// Find the tag name from start_tag or self_closing_tag
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		childType := child.Type()

		if childType == "start_tag" || childType == "self_closing_tag" {
			// Extract tag name
			for j := 0; j < int(child.ChildCount()); j++ {
				tagChild := child.Child(j)
				if tagChild.Type() == "tag_name" {
					tagName = tagChild.Content(source)
					break
				}
			}
			break
		}
	}

	if tagName == "" {
		return
	}

	// Check for slot element
	if tagName == "slot" {
		slotInfo := SlotInfo{}
		// Check for name attribute
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "start_tag" {
				for j := 0; j < int(child.ChildCount()); j++ {
					attr := child.Child(j)
					if attr.Type() == "attribute" {
						attrText := attr.Content(source)
						if len(attrText) > 5 && attrText[:5] == "name=" {
							slotInfo.Name = extractAttributeValue(attrText)
						}
					}
				}
			}
		}
		// Check for fallback content (has children)
		slotInfo.Fallback = hasChildContent(node)
		info.Slots = append(info.Slots, slotInfo)
		return
	}

	// Check for component (PascalCase tag)
	if isComponentName(tagName) && !seen[tagName] {
		seen[tagName] = true
		info.Components = append(info.Components, tagName)
	}
}

// extractAttributeValue extracts the value from an attribute string like 'name="value"'
func extractAttributeValue(attr string) string {
	idx := -1
	for i, ch := range attr {
		if ch == '=' {
			idx = i
			break
		}
	}
	if idx < 0 || idx+1 >= len(attr) {
		return ""
	}
	value := attr[idx+1:]
	// Remove quotes
	value = trimQuotes(value)
	return value
}

// trimQuotes removes surrounding quotes from a string
func trimQuotes(s string) string {
	if len(s) < 2 {
		return s
	}
	first := s[0]
	last := s[len(s)-1]
	if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}

// hasChildContent checks if a node has meaningful child content
func hasChildContent(node *sitter.Node) bool {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		childType := child.Type()
		// Skip start/end tags, look for actual content
		if childType != "start_tag" && childType != "end_tag" && childType != "self_closing_tag" {
			return true
		}
	}
	return false
}
