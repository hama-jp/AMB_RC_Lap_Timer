// Package webassets exposes the embedded SPA bundle (built by `web/`) as
// an `fs.FS` ready to hand to `http.FileServer`.
//
// Build flow (docs/architecture.md §4.1):
//
//  1. `web/` is built with `npm run build`, producing `web/dist/`.
//  2. `scripts/build.ps1` copies `web/dist/*` into
//     `gateway/internal/webassets/dist/`.
//  3. `go build ./cmd/gateway` embeds those files via the directive below.
//
// In the repo we only track `dist/.gitkeep`; everything else under `dist/`
// is gitignored. Tests inject their own `fs.FS` rather than relying on the
// embedded bundle.
package webassets

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// FS returns the embedded asset tree rooted at `dist/`.
// Returns an error only on internal misconfiguration (the embed pattern
// always matches at least `.gitkeep`).
func FS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
