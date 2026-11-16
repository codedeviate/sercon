// Command zstdpack compresses a file with zstd at the best-compression
// level: zstdpack <in> <out>. It exists so the build needs no external
// zstd CLI.
package main

import (
	"fmt"
	"os"

	"github.com/klauspost/compress/zstd"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: zstdpack <in> <out>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "zstdpack:", err)
		os.Exit(1)
	}
}

func run(in, out string) error {
	data, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	if err != nil {
		return err
	}
	defer func() { _ = enc.Close() }()
	return os.WriteFile(out, enc.EncodeAll(data, nil), 0o644)
}
