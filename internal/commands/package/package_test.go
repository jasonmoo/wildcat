package package_cmd

import (
	"fmt"
	"testing"

	"github.com/jasonmoo/wildcat/internal/commands"
)

func TestPackageExecute(t *testing.T) {

	wc, err := commands.LoadWildcat(t.Context(), ".")
	if err != nil {
		t.Error(err)
	}

	pc := NewPackageCommand()

	res, e := pc.Execute(t.Context(), wc, WithPackage("internal/commands/package"))
	if e != nil {
		t.Error(e)
	}

	data, err := res.MarshalMarkdown()
	if err != nil {
		t.Error(err)
	}
	fmt.Println(string(data))

}
