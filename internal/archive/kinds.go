package archive

import (
	"path"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/seal"
	"github.com/d0lim/cockpit/internal/signal"
	"github.com/d0lim/cockpit/internal/wire"
)

const (
	FileIndex    = "index.json"
	DirReport    = report.DirReport
	DirSchema    = "schema"
	SchemaIndex  = "archive"
	schemaSuffix = ".json"
	anyDepth     = "/**"
)

type Kind struct {
	Name   contract.ArchiveKind
	Path   string
	Schema string
}

var Kinds = []Kind{
	{contract.ArchiveKindSummary, report.FileSummary, "summary"},
	{contract.ArchiveKindMeta, report.FileMeta, "meta"},
	{contract.ArchiveKindNotProven, report.FileNotProven, ""},
	{contract.ArchiveKindSeal, seal.FileName, ""},
	{contract.ArchiveKindManifest, report.BundleRunner + "/" + report.RunnerManifest, "manifest"},
	{contract.ArchiveKindResult, report.BundleRunner + "/" + report.RunnerResult, "result"},
	{contract.ArchiveKindLedger, report.BundleRunner + "/" + report.RunnerLedger, "ledger"},
	{contract.ArchiveKindContext, report.BundleRunner + "/" + report.RunnerContext, "context"},
	{contract.ArchiveKindExec, report.BundleRunner + "/" + report.RunnerExecDir + "/*.json", "exec"},
	{contract.ArchiveKindSourceIndex, report.SourceIndexFile, ""},
	{contract.ArchiveKindSource, report.BundleSource + anyDepth, ""},
	{contract.ArchiveKindSignals, signal.FrozenPath, "signal"},
	{contract.ArchiveKindWireIndex, wire.StagingDir + "/*/" + wire.IndexFile, ""},
	{contract.ArchiveKindWireCalls, wire.StagingDir + "/*/" + wire.CallsFile, ""},
}

func KindOf(rel string) (Kind, bool) {
	for _, k := range Kinds {
		if k.takes(rel) {
			return k, true
		}
	}
	return Kind{}, false
}

func (k Kind) takes(rel string) bool {
	if dir, deep := strings.CutSuffix(k.Path, anyDepth); deep {
		return strings.HasPrefix(rel, dir+"/")
	}
	ok, err := path.Match(k.Path, rel)
	return err == nil && ok
}

func (k Kind) schemaPath() string {
	if k.Schema == "" {
		return ""
	}
	return DirSchema + "/" + k.Schema + schemaSuffix
}

func takeOf(rel string) (string, bool) {
	parts := strings.Split(rel, "/")
	if len(parts) != 3 || parts[0] != wire.StagingDir || !wire.ValidTake(parts[1]) {
		return "", false
	}
	return parts[1], parts[2] == wire.IndexFile || parts[2] == wire.CallsFile
}
