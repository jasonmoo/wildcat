package channels_cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jasonmoo/wildcat/internal/output"
)

// ChannelOp represents a single channel operation with its location
type ChannelOp struct {
	Operation string `json:"operation"`
	Location  string `json:"location"`
}

// ChannelGroup groups operations by element type
type ChannelGroup struct {
	ElementType    string      `json:"element_type"`
	Makes          []ChannelOp `json:"makes,omitempty"`
	Sends          []ChannelOp `json:"sends,omitempty"`
	Receives       []ChannelOp `json:"receives,omitempty"`
	Closes         []ChannelOp `json:"closes,omitempty"`
	SelectSends    []ChannelOp `json:"select_sends,omitempty"`
	SelectReceives []ChannelOp `json:"select_receives,omitempty"`
}

// PackageChannels groups channel operations by package
type PackageChannels struct {
	Package  string         `json:"package"`
	Channels []ChannelGroup `json:"channels,omitempty"`
	Message  string         `json:"message,omitempty"`
}

type ChannelsSummary struct {
	TotalOps int            `json:"total_ops"`
	ByKind   map[string]int `json:"by_kind"`
	Packages int            `json:"packages"`
	Types    int            `json:"types"`
}

type ChannelsCommandResponse struct {
	Query      output.QueryInfo  `json:"query"`
	Operations []PackageChannels `json:"operations"`
	Summary    ChannelsSummary   `json:"summary"`
}

func (r *ChannelsCommandResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Query      output.QueryInfo  `json:"query"`
		Summary    ChannelsSummary   `json:"summary"`
		Operations []PackageChannels `json:"operations"`
	}{
		Query:      r.Query,
		Summary:    r.Summary,
		Operations: r.Operations,
	})
}

func (r *ChannelsCommandResponse) MarshalMarkdown() ([]byte, error) {
	var sb strings.Builder

	// Header
	sb.WriteString("# channels ")
	sb.WriteString(r.Query.Target)
	sb.WriteString("\n")

	// Summary
	fmt.Fprintf(&sb, "\n# Summary (%d ops)\n", r.Summary.TotalOps)
	fmt.Fprintf(&sb, "packages: %d\n", r.Summary.Packages)
	fmt.Fprintf(&sb, "types: %d\n", r.Summary.Types)
	if r.Summary.ByKind["make"] > 0 {
		fmt.Fprintf(&sb, "make: %d\n", r.Summary.ByKind["make"])
	}
	if r.Summary.ByKind["send"] > 0 {
		fmt.Fprintf(&sb, "send: %d\n", r.Summary.ByKind["send"])
	}
	if r.Summary.ByKind["receive"] > 0 {
		fmt.Fprintf(&sb, "receive: %d\n", r.Summary.ByKind["receive"])
	}
	if r.Summary.ByKind["close"] > 0 {
		fmt.Fprintf(&sb, "close: %d\n", r.Summary.ByKind["close"])
	}
	if r.Summary.ByKind["select_send"] > 0 {
		fmt.Fprintf(&sb, "select_send: %d\n", r.Summary.ByKind["select_send"])
	}
	if r.Summary.ByKind["select_receive"] > 0 {
		fmt.Fprintf(&sb, "select_receive: %d\n", r.Summary.ByKind["select_receive"])
	}

	// Operations by package
	fmt.Fprintf(&sb, "\n# Operations (%d)\n", r.Summary.TotalOps)
	for _, pkg := range r.Operations {
		sb.WriteString("\n## ")
		sb.WriteString(pkg.Package)
		sb.WriteString("\n")

		for _, group := range pkg.Channels {
			sb.WriteString("\n### chan ")
			sb.WriteString(group.ElementType)
			sb.WriteString("\n")

			if len(group.Makes) > 0 {
				sb.WriteString("**make**\n")
				for _, op := range group.Makes {
					writeOpMd(&sb, op)
				}
			}

			if len(group.Sends) > 0 {
				sb.WriteString("**send**\n")
				for _, op := range group.Sends {
					writeOpMd(&sb, op)
				}
			}

			if len(group.Receives) > 0 {
				sb.WriteString("**receive**\n")
				for _, op := range group.Receives {
					writeOpMd(&sb, op)
				}
			}

			if len(group.Closes) > 0 {
				sb.WriteString("**close**\n")
				for _, op := range group.Closes {
					writeOpMd(&sb, op)
				}
			}

			if len(group.SelectSends) > 0 {
				sb.WriteString("**select send**\n")
				for _, op := range group.SelectSends {
					writeOpMd(&sb, op)
				}
			}

			if len(group.SelectReceives) > 0 {
				sb.WriteString("**select receive**\n")
				for _, op := range group.SelectReceives {
					writeOpMd(&sb, op)
				}
			}
		}
	}

	return []byte(sb.String()), nil
}

func writeOpMd(sb *strings.Builder, op ChannelOp) {
	sb.WriteString("- ")
	sb.WriteString(op.Operation)
	sb.WriteString(" // ")
	sb.WriteString(op.Location)
	sb.WriteString("\n")
}
