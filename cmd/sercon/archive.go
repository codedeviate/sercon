package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// archiveNamespace wires `fs.archive.*`. Both members are async because
// each call may walk a directory tree on disk and serialise it; running on
// the event loop's goroutine keeps the JS side responsive.
func archiveNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"create":  scriptengine.PromisifyAsync(vm, loop, archiveCreate),
		"extract": scriptengine.PromisifyAsync(vm, loop, archiveExtract),
	}
}

// archiveSource is one entry in the create() sources list: an on-disk
// path plus the name it should take inside the archive (defaults to the
// basename when callers pass a bare string).
type archiveSource struct {
	path string // disk path
	name string // in-archive path
}

// archiveCreate writes a new archive at destPath. Format is inferred from
// the destination's extension (.zip / .tar / .tar.gz / .tgz). Sources are
// strings (use basename inside the archive) or `{path, name?}` objects.
// Directory sources are recursed; the directory's basename is used as the
// archive subdir, with the rest of the tree appearing relative to it.
func archiveCreate(_ context.Context, call goja.FunctionCall) (map[string]any, error) {
	destPath := call.Argument(0).String()
	if destPath == "" {
		return nil, errors.New("create: destination path required")
	}
	sources, err := parseSources(call.Argument(1))
	if err != nil {
		return nil, fmt.Errorf("create: %w", err)
	}
	if len(sources) == 0 {
		return nil, errors.New("create: no sources")
	}

	format := detectArchiveFormat(destPath)
	if format == "" {
		return nil, fmt.Errorf("create: cannot infer format from %q (supported: .zip, .tar, .tar.gz, .tgz)", destPath)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var entries []string
	switch format {
	case "zip":
		entries, err = writeZip(f, sources)
	case "tar":
		entries, err = writeTar(f, sources)
	case "tar.gz":
		gw := gzip.NewWriter(f)
		entries, err = writeTar(gw, sources)
		if err == nil {
			err = gw.Close()
		} else {
			_ = gw.Close()
		}
	}
	if err != nil {
		return nil, err
	}
	if err := f.Sync(); err != nil {
		return nil, err
	}

	fi, _ := f.Stat()
	out := map[string]any{
		"path":    destPath,
		"format":  format,
		"entries": entries,
	}
	if fi != nil {
		out["bytes"] = fi.Size()
	}
	return out, nil
}

// archiveExtract reads an archive at archivePath and writes its contents
// under destDir. Format is inferred from the archive's extension. The
// `overwrite` opt controls whether existing files at the destination are
// clobbered (default false — extraction errors out on collisions). All
// entry names are run through safeJoin so a malicious archive can't write
// outside destDir (zip-slip / tar-slip protection).
func archiveExtract(_ context.Context, call goja.FunctionCall) (map[string]any, error) {
	archivePath := call.Argument(0).String()
	destDir := call.Argument(1).String()
	if archivePath == "" || destDir == "" {
		return nil, errors.New("extract: archive path and destination required")
	}
	// archiveExtract takes 3 positional args (archivePath, destDir, opts),
	// so the 2-arg optsAsMap helper would mistake destDir for opts.
	// Pull the third arg out by hand. Same shape as the diff.compare fix.
	overwrite := false
	if len(call.Arguments) >= 3 {
		arg := call.Argument(2)
		if arg != nil && !goja.IsUndefined(arg) && !goja.IsNull(arg) {
			if m, ok := arg.Export().(map[string]any); ok {
				if b, ok := m["overwrite"].(bool); ok {
					overwrite = b
				}
			}
		}
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}
	format := detectArchiveFormat(archivePath)
	if format == "" {
		return nil, fmt.Errorf("extract: cannot infer format from %q", archivePath)
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var entries []string
	switch format {
	case "zip":
		entries, err = extractZip(f, destDir, overwrite)
	case "tar":
		entries, err = extractTar(f, destDir, overwrite)
	case "tar.gz":
		gr, gErr := gzip.NewReader(f)
		if gErr != nil {
			return nil, gErr
		}
		defer func() { _ = gr.Close() }()
		entries, err = extractTar(gr, destDir, overwrite)
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"path":    archivePath,
		"format":  format,
		"dest":    destDir,
		"entries": entries,
	}, nil
}

// parseSources accepts either a string array or an array of
// `{path: string, name?: string}` objects from JS.
func parseSources(v goja.Value) ([]archiveSource, error) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil, errors.New("sources is undefined or null")
	}
	exported := v.Export()
	switch s := exported.(type) {
	case []any:
		out := make([]archiveSource, 0, len(s))
		for _, item := range s {
			switch entry := item.(type) {
			case string:
				out = append(out, archiveSource{path: entry, name: filepath.Base(entry)})
			case map[string]any:
				p, _ := entry["path"].(string)
				if p == "" {
					return nil, errors.New("source object missing 'path'")
				}
				n, _ := entry["name"].(string)
				if n == "" {
					n = filepath.Base(p)
				}
				out = append(out, archiveSource{path: p, name: n})
			default:
				return nil, fmt.Errorf("unsupported source entry type %T", entry)
			}
		}
		return out, nil
	case []string:
		out := make([]archiveSource, 0, len(s))
		for _, p := range s {
			out = append(out, archiveSource{path: p, name: filepath.Base(p)})
		}
		return out, nil
	default:
		return nil, fmt.Errorf("sources must be an array, got %T", exported)
	}
}

// detectArchiveFormat picks the format string from a path's extension.
// Order matters: ".tar.gz" must be checked before ".tar".
func detectArchiveFormat(path string) string {
	p := strings.ToLower(path)
	switch {
	case strings.HasSuffix(p, ".tar.gz"), strings.HasSuffix(p, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(p, ".tar"):
		return "tar"
	case strings.HasSuffix(p, ".zip"):
		return "zip"
	}
	return ""
}

// walkSource invokes fn for the source itself (file or directory header)
// plus, for directories, every descendant. archive paths use forward
// slashes regardless of host OS so the produced archives are
// cross-platform.
func walkSource(src archiveSource, fn func(diskPath, archPath string, info fs.FileInfo) error) error {
	rootInfo, err := os.Stat(src.path)
	if err != nil {
		return err
	}
	if !rootInfo.IsDir() {
		return fn(src.path, filepath.ToSlash(src.name), rootInfo)
	}
	return filepath.WalkDir(src.path, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src.path, p)
		if err != nil {
			return err
		}
		archPath := src.name
		if rel != "." {
			archPath = filepath.Join(src.name, rel)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return fn(p, filepath.ToSlash(archPath), info)
	})
}

func writeZip(w io.Writer, sources []archiveSource) ([]string, error) {
	zw := zip.NewWriter(w)
	var entries []string
	for _, src := range sources {
		err := walkSource(src, func(diskPath, archPath string, info fs.FileInfo) error {
			if info.IsDir() {
				_, err := zw.Create(archPath + "/")
				return err
			}
			fh, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			fh.Name = archPath
			fh.Method = zip.Deflate
			f, err := zw.CreateHeader(fh)
			if err != nil {
				return err
			}
			in, err := os.Open(diskPath)
			if err != nil {
				return err
			}
			defer func() { _ = in.Close() }()
			if _, err := io.Copy(f, in); err != nil {
				return err
			}
			entries = append(entries, archPath)
			return nil
		})
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return entries, nil
}

func writeTar(w io.Writer, sources []archiveSource) ([]string, error) {
	tw := tar.NewWriter(w)
	var entries []string
	for _, src := range sources {
		err := walkSource(src, func(diskPath, archPath string, info fs.FileInfo) error {
			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			hdr.Name = archPath
			if info.IsDir() && !strings.HasSuffix(hdr.Name, "/") {
				hdr.Name += "/"
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			in, err := os.Open(diskPath)
			if err != nil {
				return err
			}
			defer func() { _ = in.Close() }()
			if _, err := io.Copy(tw, in); err != nil {
				return err
			}
			entries = append(entries, archPath)
			return nil
		})
		if err != nil {
			_ = tw.Close()
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return entries, nil
}

func extractZip(f *os.File, destDir string, overwrite bool) ([]string, error) {
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(f, fi.Size())
	if err != nil {
		return nil, err
	}
	var entries []string
	for _, zf := range zr.File {
		target, err := safeJoin(destDir, zf.Name)
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(zf.Name, "/") {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return nil, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}
		flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		if !overwrite {
			flag |= os.O_EXCL
		}
		mode := zf.Mode()
		if mode == 0 {
			mode = 0o644
		}
		out, err := os.OpenFile(target, flag, mode)
		if err != nil {
			return nil, err
		}
		rc, err := zf.Open()
		if err != nil {
			_ = out.Close()
			return nil, err
		}
		_, copyErr := io.Copy(out, rc)
		_ = rc.Close()
		_ = out.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		entries = append(entries, zf.Name)
	}
	return entries, nil
}

func extractTar(r io.Reader, destDir string, overwrite bool) ([]string, error) {
	tr := tar.NewReader(r)
	var entries []string
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		target, err := safeJoin(destDir, hdr.Name)
		if err != nil {
			return nil, err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return nil, err
			}
		case tar.TypeReg, tar.TypeRegA: //nolint:staticcheck // RegA is legacy but still appears in older archives
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return nil, err
			}
			flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
			if !overwrite {
				flag |= os.O_EXCL
			}
			mode := os.FileMode(hdr.Mode & 0o7777)
			if mode == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(target, flag, mode)
			if err != nil {
				return nil, err
			}
			_, copyErr := io.Copy(out, tr) //nolint:gosec // archive size bounded by caller's input
			_ = out.Close()
			if copyErr != nil {
				return nil, copyErr
			}
			entries = append(entries, hdr.Name)
		}
	}
	return entries, nil
}

// safeJoin defends against zip-slip / tar-slip — attacks where a crafted
// archive contains entries like "../../etc/passwd" that would let an
// adversary write outside destDir.
//
// We refuse to silently sanitise: any `..` segment or absolute-path
// prefix in the entry name fails the call so a malicious archive
// surfaces loudly rather than getting rewritten into a legal-looking
// entry. The redundant prefix check at the end catches edge cases on
// Windows-style paths or unusual filepath canonicalisations.
func safeJoin(destDir, name string) (string, error) {
	if name == "" {
		return "", errors.New("archive entry has empty name")
	}
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) {
		return "", fmt.Errorf("archive entry %q is absolute", name)
	}
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		if part == ".." {
			return "", fmt.Errorf("archive entry %q contains a parent-directory component", name)
		}
	}
	destAbs, err := filepath.Abs(filepath.Clean(destDir))
	if err != nil {
		return "", err
	}
	target := filepath.Join(destAbs, filepath.FromSlash(name))
	if target != destAbs && !strings.HasPrefix(target, destAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("archive entry %q escapes destination", name)
	}
	return target, nil
}
