package contract

type Name string

type RelPath string

type PinnedImage string

type ImageKeyName string

type OpenMode string

const (
	OpenBrowser OpenMode = "browser"
	OpenLink    OpenMode = "link"
	OpenCopy    OpenMode = "copy"
)

type DBDriver string

const (
	DriverMySQL    DBDriver = "mysql"
	DriverPostgres DBDriver = "postgres"
)

type SecretFormat string

const (
	SecretFormatDotenv SecretFormat = "dotenv"
	SecretFormatJSON   SecretFormat = "json"
	SecretFormatPEM    SecretFormat = "pem"
)

type ProjectSection struct {
	Builder          string   `json:"builder,omitempty"`
	ExpectedServices []string `json:"expectedServices"`
	NetworkOnlyKeys  []string `json:"networkOnlyKeys,omitempty"`
}

type RunnerSource struct {
	Name Name    `json:"name"`
	Path RelPath `json:"path"`
}

type InputsSection struct {
	Dirty         []RelPath      `json:"dirty,omitempty"`
	RunnerSources []RunnerSource `json:"runnerSources,omitempty"`
}

type ComposeSection struct {
	Files     []RelPath `json:"files"`
	Forbidden []string  `json:"forbidden,omitempty"`
}

type PortOffset struct {
	Name    Name     `json:"name"`
	Offset  int      `json:"offset"`
	Service string   `json:"service"`
	Port    int      `json:"port"`
	Display string   `json:"display"`
	Open    OpenMode `json:"open"`
}

type ImageKey struct {
	Key    ImageKeyName `json:"key"`
	Inputs []RelPath    `json:"inputs"`
}

type EnvSection struct {
	Command []string    `json:"command"`
	Image   PinnedImage `json:"image"`
	Outputs []RelPath   `json:"outputs"`
}

type SecretSource struct {
	Path   RelPath      `json:"path"`
	Format SecretFormat `json:"format"`
}

type SecretsSection struct {
	Sources     []SecretSource `json:"sources"`
	PublicNames []string       `json:"publicNames,omitempty"`
	Patterns    []string       `json:"patterns,omitempty"`
}

type Database struct {
	Name        Name     `json:"name"`
	Driver      DBDriver `json:"driver"`
	ReadSchemas []string `json:"readSchemas"`
	OwnerRole   string   `json:"ownerRole,omitempty"`
	RootSecret  string   `json:"rootSecret"`
	RootUser    string   `json:"rootUser,omitempty"`
	Database    string   `json:"database,omitempty"`
}

type ExecSection struct {
	Allow [][]string `json:"allow"`
}

type ScriptsSection struct {
	Dir        RelPath `json:"dir"`
	Suites     RelPath `json:"suites"`
	Collectors RelPath `json:"collectors,omitempty"`
}

type KitSection struct {
	Notes string `json:"notes"`
}

type Declaration struct {
	HostAPI   int            `json:"hostApi"`
	Project   ProjectSection `json:"project"`
	Inputs    InputsSection  `json:"inputs,omitzero"`
	Compose   ComposeSection `json:"compose"`
	Ports     []PortOffset   `json:"ports,omitempty"`
	Images    []ImageKey     `json:"images,omitempty"`
	Env       EnvSection     `json:"env,omitzero"`
	Secrets   SecretsSection `json:"secrets"`
	Databases []Database     `json:"databases,omitempty"`
	Exec      ExecSection    `json:"exec,omitzero"`
	Scripts   ScriptsSection `json:"scripts"`
	Kit       KitSection     `json:"kit,omitzero"`
}
