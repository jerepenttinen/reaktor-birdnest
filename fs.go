package reaktor_birdnest

import "embed"

// Embed html templates to binary
// . and .. are not allowed in embed statement

//go:embed ui/html
var TemplateFS embed.FS
