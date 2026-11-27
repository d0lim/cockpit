package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	"go.yaml.in/yaml/v3"

	cockpit "github.com/d0lim/cockpit"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/version"
)

const DeclarationPath = ".cockpit/project.yaml"

type RejectKind string

const (
	RejectSubmodule   RejectKind = "submodule"
	RejectMissing     RejectKind = "missing"
	RejectUnreadable  RejectKind = "unreadable"
	RejectSchema      RejectKind = "schema"
	RejectConfinement RejectKind = "confinement"
	RejectHostAPI     RejectKind = "host-api"
	RejectSecrets     RejectKind = "secrets"
	RejectPorts       RejectKind = "ports"
	RejectCompose     RejectKind = "compose"
)

type Rejection struct {
	Kind    RejectKind
	Field   string
	Message string
}

func (r *Rejection) Error() string {
	if r.Field == "" {
		return fmt.Sprintf("declaration rejected (%s): %s", r.Kind, r.Message)
	}
	return fmt.Sprintf("declaration rejected (%s) at %s: %s", r.Kind, r.Field, r.Message)
}

func reject(kind RejectKind, field, format string, args ...any) *Rejection {
	return &Rejection{Kind: kind, Field: field, Message: fmt.Sprintf(format, args...)}
}

var ServedHostAPIs = []int{contract.HostAPI}

type Loaded struct {
	Declaration contract.Declaration
	File        contract.DeclarationFile
	Tree        Tree
	Model       Model
}

func ReadDeclaration(ctx context.Context, git Git, sha string, file contract.DeclarationFile) (Loaded, error) {
	name := file.Path()
	tree, err := git.ListTree(ctx, sha, name)
	if err != nil {
		return Loaded{}, err
	}
	entry, ok := tree.Entries[name]
	if !ok {
		return Loaded{}, reject(RejectMissing, "", "commit %s has no %s; add a project declaration with hostApi %s", sha[:12], name, servedText())
	}
	if link, linked := tree.Link(name); linked {
		return Loaded{}, reject(RejectUnreadable, "", "%s passes through the symbolic link %s in the commit", name, link)
	}
	if entry.Type != "blob" {
		return Loaded{}, reject(RejectUnreadable, "", "%s is not a regular file in the commit", name)
	}
	raw, err := tree.Blob(ctx, name)
	if err != nil {
		return Loaded{}, reject(RejectUnreadable, "", "cannot read %s: %v", name, err)
	}
	decl, err := ParseDeclaration(name, raw)
	if err != nil {
		return Loaded{}, err
	}
	if err := cleanDeclaredPaths(decl); err != nil {
		return Loaded{}, err
	}
	if err := tree.splice(ctx, referencedPaths(decl)); err != nil {
		return Loaded{}, err
	}
	if err := confine(decl, tree); err != nil {
		return Loaded{}, err
	}
	if err := checkCollectors(decl, tree); err != nil {
		return Loaded{}, err
	}
	if !slices.Contains(ServedHostAPIs, decl.HostAPI) {
		return Loaded{}, reject(RejectHostAPI, "hostApi", "the hub does not know host API version %d; this hub (cockpit %s) serves host API %s", decl.HostAPI, version.String(), servedText())
	}
	if len(decl.Secrets.Sources) == 0 {
		return Loaded{}, reject(RejectSecrets, "secrets.sources", "the declaration names no secret sources; list the files that hold run credentials in secrets.sources")
	}
	if err := checkPublicFields(decl); err != nil {
		return Loaded{}, err
	}
	if err := checkOffsets(decl.Ports); err != nil {
		return Loaded{}, err
	}
	if err := checkDatabasePorts(decl); err != nil {
		return Loaded{}, err
	}
	model, err := LoadModel(ctx, tree, decl)
	if err != nil {
		return Loaded{}, err
	}
	if err := model.check(decl); err != nil {
		return Loaded{}, err
	}
	return Loaded{Declaration: decl, File: file, Tree: tree, Model: model}, nil
}

func servedText() string {
	parts := make([]string, len(ServedHostAPIs))
	for i, v := range ServedHostAPIs {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ", ")
}

func ParseDeclaration(name string, raw []byte) (contract.Declaration, error) {
	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return contract.Declaration{}, reject(RejectUnreadable, "", "%s is not valid YAML: %v", name, err)
	}
	if _, ok := doc.(map[string]any); !ok {
		return contract.Declaration{}, reject(RejectUnreadable, "", "%s must hold a YAML mapping", name)
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return contract.Declaration{}, reject(RejectUnreadable, "", "%s holds values that are not plain data: %v", name, err)
	}
	var plain any
	if err := json.Unmarshal(data, &plain); err != nil {
		return contract.Declaration{}, reject(RejectUnreadable, "", "%s: %v", name, err)
	}
	schema, err := declarationSchema()
	if err != nil {
		return contract.Declaration{}, err
	}
	if err := schema.Validate(plain); err != nil {
		return contract.Declaration{}, reject(RejectSchema, "", "%v", err)
	}
	var decl contract.Declaration
	if err := contract.Decode(bytes.NewReader(data), &decl); err != nil {
		return contract.Declaration{}, reject(RejectSchema, "", "%v", err)
	}
	return decl, nil
}

var declarationSchema = sync.OnceValues(func() (*jsonschema.Resolved, error) {
	var s jsonschema.Schema
	if err := json.Unmarshal(cockpit.DeclarationSchema, &s); err != nil {
		return nil, fmt.Errorf("embedded declaration schema: %w", err)
	}
	return s.Resolve(nil)
})
