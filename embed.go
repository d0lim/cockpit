package cockpit

import "embed"

//go:embed sdk/*.d.ts sdk/tsconfig.base.json
var SDK embed.FS

//go:embed schema/declaration.json
var DeclarationSchema []byte

//go:embed schema/*.json
var Schemas embed.FS

//go:embed skill
var Skill embed.FS
