// Package web embeds the static UI (ADR-002) so the binary is self-contained.
// Everything under static/ is served at "/".
package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var embedded embed.FS

// Static returns the UI files, rooted at static/.
func Static() fs.FS {
	sub, err := fs.Sub(embedded, "static")
	if err != nil {
		panic(err) // "static" is a constant, valid path
	}
	return sub
}
