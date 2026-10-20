//go:build !embed

package soundscape

import "embed"

// Builtin is empty: themes and mappings are read from disk. Build with
// -tags embed to build them into the binary.
var Builtin embed.FS

// Embedded is whether Builtin has them: whether this was built with
// -tags embed.
const Embedded = false
