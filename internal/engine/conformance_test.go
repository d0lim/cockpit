package engine

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestEveryConformanceCorpusFilePassesInGojaAndPassCountEqualsFileCount(t *testing.T) {
	dir := filepath.Join("testdata", "conformance")
	paths, err := filepath.Glob(filepath.Join(dir, "*.ts"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("%d corpus files, %v", len(paths), err)
	}
	passed := 0
	for _, path := range paths {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			b := mustBundle(t, dir, name, SiteRunner)
			if err := newEngine(t, SiteRunner, nil, 10*time.Second).Run(context.Background(), b, "default"); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			passed++
		})
	}
	if passed != len(paths) {
		t.Fatalf("passed %d of %d files", passed, len(paths))
	}
}
