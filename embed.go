package cockpit

import "embed"

//go:embed sdk/*.d.ts sdk/tsconfig.base.json
var SDK embed.FS

//go:embed schema/declaration.json
var DeclarationSchema []byte
