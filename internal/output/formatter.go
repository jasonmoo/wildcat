package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
)

// Formatter transforms output into a specific format.
type Formatter interface {
	// Format transforms the result into the desired output format
	Format(result any) ([]byte, error)

	// Name returns the formatter name (e.g., "json", "yaml")
	Name() string

	// Description returns help text for --help
	Description() string
}

// Registry manages available formatters.
type Registry struct {
	mu         sync.RWMutex
	formatters map[string]Formatter
}

// NewRegistry creates a new formatter registry with built-in formatters.
func NewRegistry() *Registry {
	r := &Registry{
		formatters: make(map[string]Formatter),
	}
	// Register built-in formatters
	r.Register(&JSONFormatter{Pretty: true})
	r.Register(&YAMLFormatter{})
	r.Register(&MarkdownFormatter{})
	return r
}

// Register adds a formatter to the registry.
func (r *Registry) Register(f Formatter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.formatters[f.Name()] = f
}

// Get returns a formatter by name.
func (r *Registry) Get(name string) (Formatter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check for built-in formatter
	if f, ok := r.formatters[name]; ok {
		return f, nil
	}

	// Check for template: prefix
	if strings.HasPrefix(name, "template:") {
		tmplPath := strings.TrimPrefix(name, "template:")
		return NewTemplateFormatter(tmplPath)
	}

	// Check for plugin: prefix
	if strings.HasPrefix(name, "plugin:") {
		pluginName := strings.TrimPrefix(name, "plugin:")
		return NewPluginFormatter(pluginName)
	}

	// Try to find as external plugin
	if cmd := findPlugin(name); cmd != "" {
		return &PluginFormatter{Command: cmd}, nil
	}

	return nil, fmt.Errorf("formatter %q not found", name)
}

// List returns all registered formatter names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.formatters))
	for name := range r.formatters {
		names = append(names, name)
	}
	return names
}

// All returns all formatters with descriptions.
func (r *Registry) All() []Formatter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	formatters := make([]Formatter, 0, len(r.formatters))
	for _, f := range r.formatters {
		formatters = append(formatters, f)
	}
	return formatters
}

// JSONFormatter outputs JSON.
type JSONFormatter struct {
	Pretty bool
}

func (f *JSONFormatter) Name() string        { return "json" }
func (f *JSONFormatter) Description() string { return "JSON output (default)" }

func (f *JSONFormatter) Format(result any) ([]byte, error) {
	if f.Pretty {
		return json.MarshalIndent(result, "", "  ")
	}
	return json.Marshal(result)
}

// YAMLFormatter outputs YAML.
type YAMLFormatter struct{}

func (f *YAMLFormatter) Name() string        { return "yaml" }
func (f *YAMLFormatter) Description() string { return "YAML output" }

func (f *YAMLFormatter) Format(result any) ([]byte, error) {
	// Simple YAML conversion from JSON - not full YAML but good enough
	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	// Convert to simple YAML-like format
	return jsonToYAML(jsonBytes)
}

// jsonToYAML converts JSON to simple YAML format.
func jsonToYAML(jsonBytes []byte) ([]byte, error) {
	var data any
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	writeYAML(&buf, data, 0)
	return buf.Bytes(), nil
}

func writeYAML(buf *bytes.Buffer, data any, indent int) {
	prefix := strings.Repeat("  ", indent)

	switch v := data.(type) {
	case map[string]any:
		for key, val := range v {
			switch child := val.(type) {
			case map[string]any, []any:
				buf.WriteString(prefix + key + ":\n")
				writeYAML(buf, child, indent+1)
			default:
				buf.WriteString(fmt.Sprintf("%s%s: %v\n", prefix, key, formatYAMLValue(val)))
			}
		}
	case []any:
		for _, item := range v {
			switch child := item.(type) {
			case map[string]any, []any:
				buf.WriteString(prefix + "-\n")
				writeYAML(buf, child, indent+1)
			default:
				buf.WriteString(fmt.Sprintf("%s- %v\n", prefix, formatYAMLValue(item)))
			}
		}
	default:
		buf.WriteString(fmt.Sprintf("%s%v\n", prefix, formatYAMLValue(v)))
	}
}

func formatYAMLValue(v any) string {
	switch val := v.(type) {
	case string:
		if strings.Contains(val, "\n") || strings.Contains(val, ":") {
			return fmt.Sprintf("%q", val)
		}
		return val
	case nil:
		return "null"
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", val)
	}
}

// MarkdownFormatter outputs Markdown tables and lists.
type MarkdownFormatter struct{}

func (f *MarkdownFormatter) Name() string        { return "markdown" }
func (f *MarkdownFormatter) Description() string { return "Markdown tables and lists" }

func (f *MarkdownFormatter) Format(result any) ([]byte, error) {
	var buf bytes.Buffer

	// Try JSON marshal and convert to determine type
	jsonBytes, _ := json.Marshal(result)

	// Check if it's an array (multi-symbol query)
	var dataArray []map[string]any
	if err := json.Unmarshal(jsonBytes, &dataArray); err == nil && len(dataArray) > 0 {
		// Format each response
		for i, data := range dataArray {
			if i > 0 {
				buf.WriteString("\n---\n\n")
			}
			f.formatSingleResponse(&buf, data)
		}
		return buf.Bytes(), nil
	}

	// Single response
	var data map[string]any
	json.Unmarshal(jsonBytes, &data)
	f.formatSingleResponse(&buf, data)

	return buf.Bytes(), nil
}

func (f *MarkdownFormatter) formatSingleResponse(buf *bytes.Buffer, data map[string]any) {
	// Check for error response
	if errData, ok := data["error"].(map[string]any); ok {
		msg, _ := errData["message"].(string)
		buf.WriteString(fmt.Sprintf("# Error\n\n**%s**\n\n", msg))
		if suggestions, ok := errData["suggestions"].([]any); ok && len(suggestions) > 0 {
			buf.WriteString("Did you mean:\n")
			for _, s := range suggestions {
				buf.WriteString(fmt.Sprintf("- %v\n", s))
			}
		}
		return
	}

	// Get query info for title
	if query, ok := data["query"].(map[string]any); ok {
		cmd, _ := query["command"].(string)
		// Try target first (symbol), then pattern (search)
		title, _ := query["target"].(string)
		if title == "" {
			title, _ = query["pattern"].(string)
		}
		buf.WriteString(fmt.Sprintf("# %s: %s\n\n", strings.Title(cmd), title))
	}

	// Format bidirectional tree (tree command output)
	// Check for callers or calls at top level (new structure)
	callers, hasCallers := data["callers"].([]any)
	calls, hasCalls := data["calls"].([]any)
	if hasCallers || hasCalls {
		// Get target symbol
		symbol := ""
		if target, ok := data["target"].(map[string]any); ok {
			symbol, _ = target["symbol"].(string)
		}

		buf.WriteString("## Call Tree\n\n")
		buf.WriteString("```\n")

		// Render callers (what calls target)
		if hasCallers && len(callers) > 0 {
			buf.WriteString("Callers:\n")
			writeCallersTree(buf, callers, "")
			buf.WriteString("    │\n")
			buf.WriteString("    ▼\n")
		}

		// Render target symbol (center point)
		buf.WriteString("◆ " + symbol + "\n")

		// Render callees (what target calls)
		if hasCalls && len(calls) > 0 {
			buf.WriteString("    │\n")
			buf.WriteString("    ▼\n")
			buf.WriteString("Calls:\n")
			for i, call := range calls {
				if callMap, ok := call.(map[string]any); ok {
					writeNestedTree(buf, callMap, "", i == len(calls)-1)
				}
			}
		}

		buf.WriteString("```\n\n")
	}

	// Format results as table with snippets
	if results, ok := data["results"].([]any); ok && len(results) > 0 {
		buf.WriteString("| Symbol | File | Line | Snippet |\n")
		buf.WriteString("|--------|------|------|------|\n")

		for _, r := range results {
			if row, ok := r.(map[string]any); ok {
				symbol, _ := row["symbol"].(string)
				file, _ := row["file"].(string)

				// Handle both single line and merged lines array
				lineStr := ""
				if line, ok := row["line"].(float64); ok && line > 0 {
					lineStr = fmt.Sprintf("%.0f", line)
				} else if lines, ok := row["lines"].([]any); ok && len(lines) > 0 {
					// Format merged lines as range or list
					if len(lines) == 1 {
						lineStr = fmt.Sprintf("%.0f", lines[0].(float64))
					} else {
						first := lines[0].(float64)
						last := lines[len(lines)-1].(float64)
						lineStr = fmt.Sprintf("%.0f-%.0f", first, last)
					}
				}

				// Shorten file path
				if len(file) > 40 {
					file = "..." + file[len(file)-37:]
				}

				// Get snippet range for display
				snippetRange := ""
				if start, ok := row["snippet_start"].(float64); ok {
					if end, ok := row["snippet_end"].(float64); ok {
						snippetRange = fmt.Sprintf("L%.0f-%.0f", start, end)
					}
				}

				buf.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", symbol, file, lineStr, snippetRange))
			}
		}
		buf.WriteString("\n")

		// Add detailed snippets section
		hasSnippets := false
		for _, r := range results {
			if row, ok := r.(map[string]any); ok {
				if snippet, ok := row["snippet"].(string); ok && snippet != "" {
					hasSnippets = true
					break
				}
			}
		}

		if hasSnippets {
			buf.WriteString("## Snippets\n\n")
			for i, r := range results {
				if row, ok := r.(map[string]any); ok {
					snippet, _ := row["snippet"].(string)
					if snippet == "" {
						continue
					}

					file, _ := row["file"].(string)
					snippetStart, _ := row["snippet_start"].(float64)
					snippetEnd, _ := row["snippet_end"].(float64)

					// Create header with file and line range
					header := filepath.Base(file)
					if snippetStart > 0 {
						header = fmt.Sprintf("%s:%.0f-%.0f", header, snippetStart, snippetEnd)
					}

					buf.WriteString(fmt.Sprintf("### %d. %s\n\n", i+1, header))
					buf.WriteString("```go\n")
					buf.WriteString(snippet)
					if !strings.HasSuffix(snippet, "\n") {
						buf.WriteString("\n")
					}
					buf.WriteString("```\n\n")
				}
			}
		}
	}

	// Format packages array (search and symbol commands)
	if packages, ok := data["packages"].([]any); ok && len(packages) > 0 {
		for _, pkg := range packages {
			pkgMap, ok := pkg.(map[string]any)
			if !ok {
				continue
			}
			pkgName, _ := pkgMap["package"].(string)
			pkgDir, _ := pkgMap["dir"].(string)

			buf.WriteString(fmt.Sprintf("## %s\n\n", pkgName))
			if pkgDir != "" {
				buf.WriteString(fmt.Sprintf("**Dir:** `%s`\n\n", pkgDir))
			}

			// Handle search matches
			if matches, ok := pkgMap["matches"].([]any); ok && len(matches) > 0 {
				buf.WriteString("| Symbol | Kind | Location |\n")
				buf.WriteString("|--------|------|----------|\n")
				for _, m := range matches {
					if match, ok := m.(map[string]any); ok {
						symbol, _ := match["symbol"].(string)
						kind, _ := match["kind"].(string)
						location, _ := match["location"].(string)
						buf.WriteString(fmt.Sprintf("| %s | %s | %s |\n", symbol, kind, location))
					}
				}
				buf.WriteString("\n")
			}

			// Handle symbol callers
			if callers, ok := pkgMap["callers"].([]any); ok && len(callers) > 0 {
				f.formatPackageLocations(buf, callers, "Callers")
			}

			// Handle symbol references
			if refs, ok := pkgMap["references"].([]any); ok && len(refs) > 0 {
				f.formatPackageLocations(buf, refs, "References")
			}

			// Handle tree symbols (functions with signatures/definitions)
			if symbols, ok := pkgMap["symbols"].([]any); ok && len(symbols) > 0 {
				buf.WriteString("| Symbol | Signature | Definition |\n")
				buf.WriteString("|--------|-----------|------------|\n")
				for _, s := range symbols {
					if sym, ok := s.(map[string]any); ok {
						symbol, _ := sym["symbol"].(string)
						signature, _ := sym["signature"].(string)
						definition, _ := sym["definition"].(string)
						// Escape pipes in signature
						signature = strings.ReplaceAll(signature, "|", "\\|")
						buf.WriteString(fmt.Sprintf("| %s | `%s` | %s |\n", symbol, signature, definition))
					}
				}
				buf.WriteString("\n")
			}
		}
	}

	// Format target info (symbol command)
	if target, ok := data["target"].(map[string]any); ok {
		symbol, _ := target["symbol"].(string)
		kind, _ := target["kind"].(string)
		file, _ := target["file"].(string)
		line, _ := target["line"].(float64)
		buf.WriteString("## Definition\n\n")
		buf.WriteString(fmt.Sprintf("- **Symbol:** %s\n", symbol))
		buf.WriteString(fmt.Sprintf("- **Kind:** %s\n", kind))
		buf.WriteString(fmt.Sprintf("- **Location:** %s:%.0f\n\n", file, line))
	}

	// Format implementations (for implements command)
	if impls, ok := data["implementations"].([]any); ok && len(impls) > 0 {
		f.formatResultsTable(buf, impls, "Implementations")
	}

	// Format interfaces (for satisfies command)
	if ifaces, ok := data["interfaces"].([]any); ok && len(ifaces) > 0 {
		f.formatResultsTable(buf, ifaces, "Interfaces")
	}

	// Format impact sections (legacy)
	if impact, ok := data["impact"].(map[string]any); ok {
		if callers, ok := impact["callers"].([]any); ok && len(callers) > 0 {
			f.formatResultsTable(buf, callers, "Callers")
		}
		if refs, ok := impact["references"].([]any); ok && len(refs) > 0 {
			f.formatResultsTable(buf, refs, "References")
		}
		if impls, ok := impact["implementations"].([]any); ok && len(impls) > 0 {
			f.formatResultsTable(buf, impls, "Implementations")
		}
	}

	// Format package info
	if pkgInfo, ok := data["package"].(map[string]any); ok {
		importPath, _ := pkgInfo["import_path"].(string)
		dir, _ := pkgInfo["dir"].(string)
		if dir != "" {
			buf.WriteString(fmt.Sprintf("**Dir:** `%s`\n\n", dir))
		}
		if importPath != "" && importPath != dir {
			buf.WriteString(fmt.Sprintf("**Import:** `%s`\n\n", importPath))
		}
	}

	// Format package symbols (constants, functions, types)
	f.formatPackageSymbols(buf, data, "constants", "Constants")
	f.formatPackageSymbols(buf, data, "functions", "Functions")
	f.formatPackageSymbols(buf, data, "types", "Types")

	// Format imports/imported_by
	if imports, ok := data["imports"].([]any); ok && len(imports) > 0 {
		buf.WriteString("## Imports\n\n")
		for _, imp := range imports {
			if impMap, ok := imp.(map[string]any); ok {
				path, _ := impMap["path"].(string)
				buf.WriteString(fmt.Sprintf("- `%s`\n", path))
			}
		}
		buf.WriteString("\n")
	}

	if importedBy, ok := data["imported_by"].([]any); ok && len(importedBy) > 0 {
		buf.WriteString("## Imported By\n\n")
		for _, imp := range importedBy {
			if impMap, ok := imp.(map[string]any); ok {
				path, _ := impMap["path"].(string)
				buf.WriteString(fmt.Sprintf("- `%s`\n", path))
			}
		}
		buf.WriteString("\n")
	}

	// Summary
	if summary, ok := data["summary"].(map[string]any); ok {
		buf.WriteString("## Summary\n\n")
		for key, val := range summary {
			buf.WriteString(fmt.Sprintf("- **%s**: %v\n", key, val))
		}
	}
}

// formatResultsTable formats a slice of results as a markdown table with snippets
func (f *MarkdownFormatter) formatResultsTable(buf *bytes.Buffer, results []any, title string) {
	buf.WriteString(fmt.Sprintf("## %s\n\n", title))
	buf.WriteString("| Symbol | File | Line | Snippet |\n")
	buf.WriteString("|--------|------|------|------|\n")

	for _, r := range results {
		if row, ok := r.(map[string]any); ok {
			symbol, _ := row["symbol"].(string)
			file, _ := row["file"].(string)

			// Handle both single line and merged lines array
			lineStr := ""
			if line, ok := row["line"].(float64); ok && line > 0 {
				lineStr = fmt.Sprintf("%.0f", line)
			} else if lines, ok := row["lines"].([]any); ok && len(lines) > 0 {
				if len(lines) == 1 {
					lineStr = fmt.Sprintf("%.0f", lines[0].(float64))
				} else {
					first := lines[0].(float64)
					last := lines[len(lines)-1].(float64)
					lineStr = fmt.Sprintf("%.0f-%.0f", first, last)
				}
			}

			// Shorten file path
			if len(file) > 40 {
				file = "..." + file[len(file)-37:]
			}

			// Get snippet range for display
			snippetRange := ""
			if start, ok := row["snippet_start"].(float64); ok {
				if end, ok := row["snippet_end"].(float64); ok {
					snippetRange = fmt.Sprintf("L%.0f-%.0f", start, end)
				}
			}

			buf.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", symbol, file, lineStr, snippetRange))
		}
	}
	buf.WriteString("\n")

	// Add snippets section
	hasSnippets := false
	for _, r := range results {
		if row, ok := r.(map[string]any); ok {
			if snippet, ok := row["snippet"].(string); ok && snippet != "" {
				hasSnippets = true
				break
			}
		}
	}

	if hasSnippets {
		buf.WriteString(fmt.Sprintf("### %s Snippets\n\n", title))
		for i, r := range results {
			if row, ok := r.(map[string]any); ok {
				snippet, _ := row["snippet"].(string)
				if snippet == "" {
					continue
				}

				file, _ := row["file"].(string)
				snippetStart, _ := row["snippet_start"].(float64)
				snippetEnd, _ := row["snippet_end"].(float64)

				header := filepath.Base(file)
				if snippetStart > 0 {
					header = fmt.Sprintf("%s:%.0f-%.0f", header, snippetStart, snippetEnd)
				}

				buf.WriteString(fmt.Sprintf("#### %d. %s\n\n", i+1, header))
				buf.WriteString("```go\n")
				buf.WriteString(snippet)
				if !strings.HasSuffix(snippet, "\n") {
					buf.WriteString("\n")
				}
				buf.WriteString("```\n\n")
			}
		}
	}
}

// formatPackageSymbols formats package command symbol arrays (constants, functions, types)
func (f *MarkdownFormatter) formatPackageSymbols(buf *bytes.Buffer, data map[string]any, key, title string) {
	symbols, ok := data[key].([]any)
	if !ok || len(symbols) == 0 {
		return
	}

	buf.WriteString(fmt.Sprintf("## %s\n\n", title))
	buf.WriteString("| Signature | Location |\n")
	buf.WriteString("|-----------|----------|\n")

	for _, sym := range symbols {
		if symMap, ok := sym.(map[string]any); ok {
			sig, _ := symMap["signature"].(string)
			loc, _ := symMap["location"].(string)
			// Escape pipes in signature
			sig = strings.ReplaceAll(sig, "|", "\\|")
			buf.WriteString(fmt.Sprintf("| `%s` | %s |\n", sig, loc))
		}
	}
	buf.WriteString("\n")
}

// formatPackageLocations formats Location items from symbol command packages
func (f *MarkdownFormatter) formatPackageLocations(buf *bytes.Buffer, locations []any, title string) {
	buf.WriteString(fmt.Sprintf("### %s\n\n", title))
	buf.WriteString("| Location | Symbol |\n")
	buf.WriteString("|----------|--------|\n")

	for _, loc := range locations {
		if locMap, ok := loc.(map[string]any); ok {
			location, _ := locMap["location"].(string)
			symbol, _ := locMap["symbol"].(string)
			buf.WriteString(fmt.Sprintf("| %s | %s |\n", location, symbol))
		}
	}
	buf.WriteString("\n")

	// Add snippets if present
	hasSnippets := false
	for _, loc := range locations {
		if locMap, ok := loc.(map[string]any); ok {
			if snippet, ok := locMap["snippet"].(map[string]any); ok {
				if _, ok := snippet["source"].(string); ok {
					hasSnippets = true
					break
				}
			}
		}
	}

	if hasSnippets {
		buf.WriteString(fmt.Sprintf("#### %s Snippets\n\n", title))
		for i, loc := range locations {
			if locMap, ok := loc.(map[string]any); ok {
				snippet, ok := locMap["snippet"].(map[string]any)
				if !ok {
					continue
				}
				source, _ := snippet["source"].(string)
				if source == "" {
					continue
				}
				snippetLoc, _ := snippet["location"].(string)

				buf.WriteString(fmt.Sprintf("##### %d. %s\n\n", i+1, snippetLoc))
				buf.WriteString("```go\n")
				buf.WriteString(source)
				if !strings.HasSuffix(source, "\n") {
					buf.WriteString("\n")
				}
				buf.WriteString("```\n\n")
			}
		}
	}
}

// writeCallersTree renders the callers tree top-down (entry points at top, flowing to target).
// The JSON structure now has entry points as roots with "calls" showing what they call toward target.
func writeCallersTree(buf *bytes.Buffer, callers []any, prefix string) {
	for i, caller := range callers {
		if callerMap, ok := caller.(map[string]any); ok {
			isLast := i == len(callers)-1
			writeCallerNodeTopDown(buf, callerMap, prefix, isLast)
		}
	}
}

// writeCallerNodeTopDown renders a caller node and its calls (toward target) recursively.
func writeCallerNodeTopDown(buf *bytes.Buffer, node map[string]any, prefix string, isLast bool) {
	symbol, _ := node["symbol"].(string)
	location, _ := node["location"].(string)

	// Determine connector based on whether this is the last item at this level
	var connector, childPrefix string
	if isLast {
		connector = "└── "
		childPrefix = prefix + "    "
	} else {
		connector = "├── "
		childPrefix = prefix + "│   "
	}

	// Render this node
	if location != "" {
		buf.WriteString(fmt.Sprintf("%s%s%s (%s)\n", prefix, connector, symbol, location))
	} else {
		buf.WriteString(fmt.Sprintf("%s%s%s\n", prefix, connector, symbol))
	}

	// Render calls (what this node calls toward the target)
	if calls, ok := node["calls"].([]any); ok && len(calls) > 0 {
		for i, call := range calls {
			if callMap, ok := call.(map[string]any); ok {
				writeCallerNodeTopDown(buf, callMap, childPrefix, i == len(calls)-1)
			}
		}
	}
}

// writeNestedTree renders a call tree node as ASCII art.
// prefix is the string to prepend before the connector (for children lines).
// isLast indicates if this node is the last child of its parent.
func writeNestedTree(buf *bytes.Buffer, node map[string]any, prefix string, isLast bool) {
	symbol, _ := node["symbol"].(string)
	location, _ := node["location"].(string)

	// Determine connector and child prefix
	var connector, childPrefix string
	if isLast {
		connector = "└── "
		childPrefix = prefix + "    "
	} else {
		connector = "├── "
		childPrefix = prefix + "│   "
	}

	// Write this node
	if location != "" {
		buf.WriteString(fmt.Sprintf("%s%s%s (%s)\n", prefix, connector, symbol, location))
	} else {
		buf.WriteString(fmt.Sprintf("%s%s%s\n", prefix, connector, symbol))
	}

	// Recurse into children
	if calls, ok := node["calls"].([]any); ok {
		for i, call := range calls {
			if callMap, ok := call.(map[string]any); ok {
				writeNestedTree(buf, callMap, childPrefix, i == len(calls)-1)
			}
		}
	}
}

// TemplateFormatter uses Go templates.
type TemplateFormatter struct {
	path string
	tmpl *template.Template
}

func NewTemplateFormatter(path string) (*TemplateFormatter, error) {
	tmpl, err := template.ParseFiles(path)
	if err != nil {
		return nil, fmt.Errorf("parsing template %s: %w", path, err)
	}
	return &TemplateFormatter{path: path, tmpl: tmpl}, nil
}

func (f *TemplateFormatter) Name() string        { return "template:" + f.path }
func (f *TemplateFormatter) Description() string { return "Custom Go template" }

func (f *TemplateFormatter) Format(result any) ([]byte, error) {
	// Convert to map for template access
	var data map[string]any
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := f.tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("executing template: %w", err)
	}
	return buf.Bytes(), nil
}

// PluginFormatter runs an external plugin.
type PluginFormatter struct {
	Command string
	Args    []string
}

func NewPluginFormatter(name string) (*PluginFormatter, error) {
	cmd := findPlugin(name)
	if cmd == "" {
		return nil, fmt.Errorf("plugin %q not found", name)
	}
	return &PluginFormatter{Command: cmd}, nil
}

func (f *PluginFormatter) Name() string        { return "plugin:" + filepath.Base(f.Command) }
func (f *PluginFormatter) Description() string { return "External plugin" }

func (f *PluginFormatter) Format(result any) ([]byte, error) {
	// Marshal result to JSON
	input, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	// Run plugin
	cmd := exec.Command(f.Command, f.Args...)
	cmd.Stdin = bytes.NewReader(input)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("plugin failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("running plugin: %w", err)
	}

	return output, nil
}

// findPlugin searches for a plugin binary.
func findPlugin(name string) string {
	// Full name with prefix
	binName := "wildcat-format-" + name

	// Check PATH
	if path, err := exec.LookPath(binName); err == nil {
		return path
	}

	// Check ~/.config/wildcat/plugins/
	if home, err := os.UserHomeDir(); err == nil {
		pluginPath := filepath.Join(home, ".config", "wildcat", "plugins", binName)
		if _, err := os.Stat(pluginPath); err == nil {
			return pluginPath
		}
	}

	// Check ./plugins/
	localPath := filepath.Join("plugins", binName)
	if _, err := os.Stat(localPath); err == nil {
		return localPath
	}

	return ""
}

// DefaultRegistry is the global formatter registry.
var DefaultRegistry = NewRegistry()
