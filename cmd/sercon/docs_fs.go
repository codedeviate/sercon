package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

func fsDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"path.dirname":    {Summary: "Directory portion of a path. POSIX-style; trailing slashes are stripped."},
		"path.basename":   {Summary: "Final segment of a path; optional suffix is stripped if it matches."},
		"archive.create":  {Summary: "Create a zip / tar / tar.gz at destPath from a list of paths. Format inferred from extension."},
		"archive.extract": {Summary: "Extract a zip / tar / tar.gz to destDir. opts.overwrite controls O_EXCL behaviour."},
	}
}
