package webassets

import "embed"

// Frontend is embedded by the production build after pnpm build.
//
//go:embed static
var Frontend embed.FS
