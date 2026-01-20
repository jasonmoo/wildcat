package tree_cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands"
	"github.com/jasonmoo/wildcat/internal/output"
)

var _ commands.Result = (*TreeCommandResponse)(nil)

type TreeCommandResponse struct {
	Query       output.TreeQuery      `json:"query"`
	Target      output.TreeTargetInfo `json:"target"`
	Summary     output.TreeSummary    `json:"summary"`
	Callers     []*output.CallNode    `json:"callers"`
	Calls       []*output.CallNode    `json:"calls"`
	Definitions []output.TreePackage  `json:"definitions"`
}

func (r *TreeCommandResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Query       output.TreeQuery      `json:"query"`
		Target      output.TreeTargetInfo `json:"target"`
		Summary     output.TreeSummary    `json:"summary"`
		Callers     []*output.CallNode    `json:"callers"`
		Calls       []*output.CallNode    `json:"calls"`
		Definitions []output.TreePackage  `json:"definitions"`
	}{
		Query:       r.Query,
		Target:      r.Target,
		Summary:     r.Summary,
		Callers:     r.Callers,
		Calls:       r.Calls,
		Definitions: r.Definitions,
	})
}

func (r *TreeCommandResponse) MarshalMarkdown() ([]byte, error) {
	var sb strings.Builder

	// Header
	fmt.Fprintf(&sb, "# Tree: %s\n\n", r.Query.Target)

	// Target section
	sb.WriteString("## Target\n\n")
	fmt.Fprintf(&sb, "- **Symbol:** %s\n", r.Target.Symbol)
	fmt.Fprintf(&sb, "- **Signature:** `%s`\n", r.Target.Signature)
	fmt.Fprintf(&sb, "- **Definition:** %s\n\n", r.Target.Definition)

	// Summary section
	sb.WriteString("## Summary\n\n")
	fmt.Fprintf(&sb, "- **Callers:** %d\n", r.Summary.Callers)
	fmt.Fprintf(&sb, "- **Callees:** %d\n", r.Summary.Callees)
	if r.Summary.MaxUpDepth > 0 {
		fmt.Fprintf(&sb, "- **Max Up Depth:** %d\n", r.Summary.MaxUpDepth)
	}
	if r.Summary.MaxDownDepth > 0 {
		fmt.Fprintf(&sb, "- **Max Down Depth:** %d\n", r.Summary.MaxDownDepth)
	}
	if r.Summary.UpTruncated {
		sb.WriteString("- **Up Truncated:** true\n")
	}
	if r.Summary.DownTruncated {
		sb.WriteString("- **Down Truncated:** true\n")
	}
	sb.WriteString("\n")

	// Callers section
	if len(r.Callers) > 0 {
		sb.WriteString("## Callers\n\n```\n")
		for _, caller := range r.Callers {
			writeCallNode(&sb, caller, "", true, false)
		}
		sb.WriteString("```\n\n")
	}

	// Calls section
	if len(r.Calls) > 0 {
		sb.WriteString("## Calls\n\n```\n")
		for _, call := range r.Calls {
			writeCallNode(&sb, call, "", true, false)
		}
		sb.WriteString("```\n\n")
	}

	// Definitions section
	if len(r.Definitions) > 0 {
		sb.WriteString("## Definitions\n\n")
		for _, pkg := range r.Definitions {
			fmt.Fprintf(&sb, "### %s\n\n", pkg.Package)
			if pkg.Dir != "" {
				fmt.Fprintf(&sb, "**Dir:** `%s`\n\n", pkg.Dir)
			}
			if len(pkg.Symbols) > 0 {
				sb.WriteString("| Symbol | Signature | Definition |\n")
				sb.WriteString("|--------|-----------|------------|\n")
				for _, sym := range pkg.Symbols {
					sig := strings.ReplaceAll(sym.Signature, "|", "\\|")
					fmt.Fprintf(&sb, "| %s | `%s` | %s |\n", sym.Symbol, sig, sym.Definition)
				}
				sb.WriteString("\n")
			}
		}
	}

	return []byte(sb.String()), nil
}

// writeCallNode renders a call node as ASCII tree
func writeCallNode(sb *strings.Builder, node *output.CallNode, prefix string, isRoot bool, isLast bool) {
	// Render this node
	if isRoot {
		if node.Callsite != "" {
			fmt.Fprintf(sb, "%s (%s)\n", node.Symbol, node.Callsite)
		} else {
			fmt.Fprintf(sb, "%s\n", node.Symbol)
		}
	} else {
		connector := "├── "
		if isLast {
			connector = "└── "
		}
		if node.Callsite != "" {
			fmt.Fprintf(sb, "%s%s%s (%s)\n", prefix, connector, node.Symbol, node.Callsite)
		} else {
			fmt.Fprintf(sb, "%s%s%s\n", prefix, connector, node.Symbol)
		}
	}

	// Recurse into children
	if len(node.Calls) > 0 {
		childPrefix := prefix
		if !isRoot {
			if isLast {
				childPrefix = prefix + "    "
			} else {
				childPrefix = prefix + "│   "
			}
		}
		for i, call := range node.Calls {
			writeCallNode(sb, call, childPrefix, false, i == len(node.Calls)-1)
		}
	}
}
