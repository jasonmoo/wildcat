package package_cmd

import (
	"testing"

	"github.com/jasonmoo/wildcat/internal/commands"
)

func TestPackageExecute(t *testing.T) {

	wc, err := commands.LoadWildcat(t.Context(), ".")
	if err != nil {
		t.Error(err)
	}

	pc := NewPackageCommand()

	_, e := pc.Execute(t.Context(), wc, WithPackage("internal/lsp"))
	if e != nil {
		t.Error(e)
	}

}
