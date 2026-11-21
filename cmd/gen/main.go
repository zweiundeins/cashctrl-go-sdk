// Command gen writes the endpoint methods of package cashctrl from spec/.
// Run it from the module root, or through go generate.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zweiundeins/cashctrl-go-sdk/internal/gen"
)

func main() {
	spec := flag.String("spec", "spec", "directory with api.json and responses.json")
	out := flag.String("out", ".", "package directory to write")
	flag.Parse()
	files, stats, err := gen.Generate(*spec)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
	// Generated files the spec no longer produces go away.
	old, _ := filepath.Glob(filepath.Join(*out, "*_gen.go"))
	oldTests, _ := filepath.Glob(filepath.Join(*out, "*_gen_test.go"))
	for _, f := range append(old, oldTests...) {
		if _, keep := files[filepath.Base(f)]; !keep {
			if err := os.Remove(f); err != nil {
				fmt.Fprintln(os.Stderr, "gen:", err)
				os.Exit(1)
			}
		}
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(*out, name), src, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "gen:", err)
			os.Exit(1)
		}
	}
	fmt.Printf("gen: %d endpoints in %d groups (%d resources), %d typed and %d untyped JSON answers, %d structs, %d enums\n",
		stats.Endpoints, stats.Groups, stats.Resources, stats.TypedResponses, stats.RawResponses, stats.Structs, stats.Enums)
}
