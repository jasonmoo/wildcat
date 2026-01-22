package golang

import (
	"context"
	"testing"
)

func TestCountNonCallReferences(t *testing.T) {
	project, err := LoadModulePackages(context.Background(), "../..", nil)
	if err != nil {
		t.Fatalf("LoadModulePackages: %v", err)
	}

	idx := CollectSymbols(project.Packages)

	tests := []struct {
		name         string
		symbol       string
		wantTotal    int  // total refs (calls + non-calls)
		wantNonCall  int  // non-call refs only
		wantNonCallGT int // or at least this many non-call refs (-1 to skip)
	}{
		{
			// runReadme is passed as struct field value: Run: runReadme
			// It's not called directly in our code - cobra calls it
			name:        "runReadme - passed to cobra",
			symbol:      "runReadme",
			wantNonCall: 1,
		},
		{
			// Execute is called from main: cmd.Execute()
			// It should have calls but no non-call refs
			name:        "Execute - called from main",
			symbol:      "Execute",
			wantNonCall: 0,
		},
		{
			// printFullReadme is called from runReadme
			// It should have calls but no non-call refs
			name:        "printFullReadme - only called",
			symbol:      "printFullReadme",
			wantNonCall: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sym := idx.Lookup(tc.symbol)
			if sym == nil {
				t.Fatalf("symbol %q not found", tc.symbol)
			}

			total := CountReferences(project.Packages, sym).Total()
			nonCall := CountNonCallReferences(project.Packages, sym)

			t.Logf("%s: total refs=%d, non-call refs=%d", tc.symbol, total, nonCall)

			if tc.wantNonCall >= 0 && nonCall != tc.wantNonCall {
				t.Errorf("CountNonCallReferences(%q) = %d, want %d", tc.symbol, nonCall, tc.wantNonCall)
			}
			if tc.wantNonCallGT >= 0 && nonCall < tc.wantNonCallGT {
				t.Errorf("CountNonCallReferences(%q) = %d, want >= %d", tc.symbol, nonCall, tc.wantNonCallGT)
			}
		})
	}
}
