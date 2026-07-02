// Command favrobundle bundles the favro TypeScript library into a single flat
// ESM module committed to git and embedded into sercon. Run via `make favro`;
// `--check` verifies the committed bundle is current.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	esbuild "github.com/evanw/esbuild/pkg/api"
)

const (
	entry  = "cmd/sercon/favro/index.ts"
	output = "cmd/sercon/favro/favro.bundle.js"
)

func bundle() ([]byte, error) {
	res := esbuild.Build(esbuild.BuildOptions{
		EntryPoints: []string{entry},
		Bundle:      true,
		Format:      esbuild.FormatESModule,
		Platform:    esbuild.PlatformNeutral,
		LogLevel:    esbuild.LogLevelSilent,
	})
	if len(res.Errors) > 0 {
		msgs := esbuild.FormatMessages(res.Errors, esbuild.FormatMessagesOptions{})
		return nil, fmt.Errorf("esbuild: %v", msgs)
	}
	if len(res.OutputFiles) != 1 {
		return nil, fmt.Errorf("expected 1 output file, got %d", len(res.OutputFiles))
	}
	return res.OutputFiles[0].Contents, nil
}

func main() {
	check := flag.Bool("check", false, "verify the committed bundle matches a fresh build")
	flag.Parse()
	out, err := bundle()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *check {
		have, err := os.ReadFile(output)
		if err != nil || !bytes.Equal(bytes.TrimSpace(have), bytes.TrimSpace(out)) {
			fmt.Fprintln(os.Stderr, "favro bundle is stale — run `make favro` and commit "+output)
			os.Exit(1)
		}
		fmt.Println("favro bundle up to date")
		return
	}
	if err := os.WriteFile(output, out, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote", output)
}
