//go:build embed

package soundscape

import "embed"

// Builtin holds the bundled themes, their samples, and the mappings.
//
//go:embed themes/*/theme.yaml themes/*/samples mappings/*.yaml
var Builtin embed.FS

// Embedded is whether Builtin has them: whether this was built with
// -tags embed.
const Embedded = true
