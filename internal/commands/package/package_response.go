package package_cmd

import (
	"encoding/json"

	"github.com/jasonmoo/wildcat/internal/output"
)

type PackageCommandResponse struct {
	Query      output.QueryInfo       `json:"query"`
	Package    output.PackageInfo     `json:"package"`
	Summary    output.PackageSummary  `json:"summary"`
	Files      []output.FileInfo      `json:"files"`
	Constants  []output.PackageSymbol `json:"constants"`
	Variables  []output.PackageSymbol `json:"variables"`
	Functions  []output.PackageSymbol `json:"functions"`
	Types      []output.PackageType   `json:"types"`
	Imports    []output.DepResult     `json:"imports"`
	ImportedBy []output.DepResult     `json:"imported_by"`
}

func (resp *PackageCommandResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(resp)
}

func (resp *PackageCommandResponse) MarshalMarkdown() ([]byte, error) {
	return []byte(renderPackageMarkdown(resp)), nil
}
