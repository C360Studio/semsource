package svelte

import (
	"regexp"
	"strconv"
	"strings"
)

// RuneType represents the type of Svelte 5 rune
type RuneType string

// RuneProps and related constants enumerate the Svelte 5 rune identifiers.
const (
	RuneProps    RuneType = "$props"
	RuneState    RuneType = "$state"
	RuneDerived  RuneType = "$derived"
	RuneEffect   RuneType = "$effect"
	RuneBindable RuneType = "$bindable"
)

// PropInfo represents a component prop extracted from $props()
type PropInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Bindable bool   `json:"bindable,omitempty"`
	Default  string `json:"default,omitempty"`
}

// StateInfo represents a reactive state variable from $state()
type StateInfo struct {
	Name         string `json:"name"`
	InitialValue string `json:"initial_value,omitempty"`
}

// DerivedInfo represents a derived value from $derived()
type DerivedInfo struct {
	Name       string `json:"name"`
	Expression string `json:"expression,omitempty"`
}

// EffectInfo represents a side effect from $effect()
type EffectInfo struct {
	Dependencies []string `json:"dependencies,omitempty"`
}

// RuneInfo holds all rune information extracted from a Svelte component
type RuneInfo struct {
	Props   []PropInfo    `json:"props,omitempty"`
	State   []StateInfo   `json:"state,omitempty"`
	Derived []DerivedInfo `json:"derived,omitempty"`
	Effects []EffectInfo  `json:"effects,omitempty"`
}

// ToDocComment converts rune info to a documentation comment format
// This is used to store Svelte-specific metadata in the DocComment field
func (r *RuneInfo) ToDocComment() string {
	var parts []string

	if len(r.Props) > 0 {
		var propNames []string
		for _, p := range r.Props {
			propName := p.Name
			if p.Type != "" {
				propName += ": " + p.Type
			}
			if p.Bindable {
				propName += " (bindable)"
			}
			propNames = append(propNames, propName)
		}
		parts = append(parts, "Props: "+strings.Join(propNames, ", "))
	}

	if len(r.State) > 0 {
		var stateNames []string
		for _, s := range r.State {
			stateNames = append(stateNames, s.Name)
		}
		parts = append(parts, "State: "+strings.Join(stateNames, ", "))
	}

	if len(r.Derived) > 0 {
		var derivedNames []string
		for _, d := range r.Derived {
			derivedNames = append(derivedNames, d.Name)
		}
		parts = append(parts, "Derived: "+strings.Join(derivedNames, ", "))
	}

	if len(r.Effects) > 0 {
		parts = append(parts, "Effects: "+strconv.Itoa(len(r.Effects)))
	}

	return strings.Join(parts, "; ")
}

// Regular expressions for extracting runes
var (
	// Match: let { prop1, prop2 = default } = $props()
	// Or: let { prop1, prop2 }: Type = $props()
	propsPattern = regexp.MustCompile(`let\s*\{\s*([^}]+)\}\s*(?::\s*(\w+))?\s*=\s*\$props\(\)`)

	// Match: let name = $state(initialValue)
	// Or: let name = $state<Type>(initialValue)
	statePattern = regexp.MustCompile(`let\s+(\w+)\s*=\s*\$state(?:<[^>]+>)?\(([^)]*)\)`)

	// Match: const name = $derived(expression)
	// Or: let name = $derived(expression)
	derivedPattern = regexp.MustCompile(`(?:const|let)\s+(\w+)\s*=\s*\$derived\(([^)]+(?:\([^)]*\))?[^)]*)\)`)

	// Match: $effect(() => { ... })
	effectPattern = regexp.MustCompile(`\$effect\s*\(\s*\(\s*\)\s*=>`)

	// Match: prop = $bindable()
	bindablePattern = regexp.MustCompile(`(\w+)\s*=\s*\$bindable\(\)`)
)

// extractRunes parses Svelte script content and extracts all rune information
func extractRunes(scriptContent []byte) *RuneInfo {
	info := &RuneInfo{}
	script := string(scriptContent)

	// Extract $props()
	if matches := propsPattern.FindStringSubmatch(script); matches != nil {
		propsContent := matches[1]
		info.Props = parsePropsContent(propsContent)
	}

	// Extract $state()
	stateMatches := statePattern.FindAllStringSubmatch(script, -1)
	for _, match := range stateMatches {
		info.State = append(info.State, StateInfo{
			Name:         match[1],
			InitialValue: strings.TrimSpace(match[2]),
		})
	}

	// Extract $derived()
	derivedMatches := derivedPattern.FindAllStringSubmatch(script, -1)
	for _, match := range derivedMatches {
		info.Derived = append(info.Derived, DerivedInfo{
			Name:       match[1],
			Expression: strings.TrimSpace(match[2]),
		})
	}

	// Extract $effect()
	effectMatches := effectPattern.FindAllString(script, -1)
	for range effectMatches {
		info.Effects = append(info.Effects, EffectInfo{})
	}

	// Mark bindable props
	bindableMatches := bindablePattern.FindAllStringSubmatch(script, -1)
	bindableSet := make(map[string]bool)
	for _, match := range bindableMatches {
		bindableSet[match[1]] = true
	}
	for i := range info.Props {
		if bindableSet[info.Props[i].Name] {
			info.Props[i].Bindable = true
		}
	}

	return info
}

// parsePropsContent parses the destructured props content
// e.g., "prop1, prop2 = default, prop3: Type" or "plan": "Props"
func parsePropsContent(content string) []PropInfo {
	var props []PropInfo

	// Split by comma, but be careful of nested structures
	parts := splitPropsContent(content)

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		prop := PropInfo{}

		// Check for default value: name = defaultValue
		if idx := strings.Index(part, "="); idx > 0 {
			nameAndType := strings.TrimSpace(part[:idx])
			prop.Default = strings.TrimSpace(part[idx+1:])

			// Check for $bindable() in default
			if strings.Contains(prop.Default, "$bindable()") {
				prop.Bindable = true
				prop.Default = ""
			}

			part = nameAndType
		}

		// Check for type annotation: name: Type
		if idx := strings.LastIndex(part, ":"); idx > 0 {
			prop.Name = strings.TrimSpace(part[:idx])
			prop.Type = strings.TrimSpace(part[idx+1:])
		} else {
			prop.Name = strings.TrimSpace(part)
		}

		if prop.Name != "" {
			props = append(props, prop)
		}
	}

	return props
}

// splitPropsContent splits props content by comma, handling nested structures
func splitPropsContent(content string) []string {
	var parts []string
	var current strings.Builder
	depth := 0

	for _, ch := range content {
		switch ch {
		case '{', '[', '(':
			depth++
			current.WriteRune(ch)
		case '}', ']', ')':
			depth--
			current.WriteRune(ch)
		case ',':
			if depth == 0 {
				parts = append(parts, current.String())
				current.Reset()
			} else {
				current.WriteRune(ch)
			}
		default:
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}
