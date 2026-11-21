package cashctrl

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/zweiundeins/cashctrl-go-sdk/internal/gen"
)

// TestGeneratedIsCurrent fails when regenerating from spec/ would change a
// generated file, or when a generated file is left that the spec no longer
// produces. Run go generate ./... and commit the result.
func TestGeneratedIsCurrent(t *testing.T) {
	files, _, err := gen.Generate("spec")
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range files {
		got, err := os.ReadFile(name)
		if err != nil {
			t.Errorf("%s: %v (run go generate ./...)", name, err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is stale: run go generate ./... and commit the result", name)
		}
	}
	for _, glob := range []string{"*_gen.go", "*_gen_test.go"} {
		onDisk, _ := filepath.Glob(glob)
		for _, name := range onDisk {
			if _, ok := files[name]; !ok {
				t.Errorf("%s is not generated any more: run go generate ./...", name)
			}
		}
	}
}
