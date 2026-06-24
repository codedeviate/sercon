# typst-safe.awk — make MANUAL.md renderable by recon's typst --md-to-pdf engine.
#
# recon's typst engine rejects ALL raw HTML ("raw HTML is not supported"), but
# MANUAL.md legitimately contains, outside of code, angle brackets in prose
# (generic types like Promise<Uint8Array>, placeholders like <domain>) and HTML
# comments used structurally (the <!-- BEGIN/END GENERATED REFERENCE --> splice
# markers, the version marker). This filter rewrites the stream so typst accepts
# it, WITHOUT touching MANUAL.md on disk (so make reference / version-check /
# release-prep keep working against the source):
#
#   * Fenced code blocks (``` … ```) pass through verbatim — code keeps its real
#     < / > and is rendered literally by typst already.
#   * Outside fences: single-line HTML comments (<!-- … -->) are removed.
#   * Outside fences AND outside inline-code spans (`…`): < becomes \< and >
#     becomes \> so the markdown engine renders them as literal characters
#     instead of parsing them as HTML tags.
#
# Usage (in the manual: recipe):
#   awk -f scripts/typst-safe.awk MANUAL.md | recon --md-to-pdf - -o MANUAL.pdf …
BEGIN { fence = 0 }
{
	line = $0
	# Toggle fenced code blocks; emit fence lines + code content unchanged.
	if (line ~ /^[[:space:]]*```/) { print line; fence = !fence; next }
	if (fence) { print line; next }
	# Drop single-line HTML comments.
	gsub(/<!--[^>]*-->/, "", line)
	# Escape < and > that fall outside inline-code (backtick) spans.
	out = ""; incode = 0; n = length(line)
	for (i = 1; i <= n; i++) {
		c = substr(line, i, 1)
		if (c == "`") { incode = !incode; out = out c }
		else if (c == "<" && !incode) { out = out "\\<" }
		else if (c == ">" && !incode) { out = out "\\>" }
		else { out = out c }
	}
	print out
}
