package engine

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestConformance_코퍼스의_모든_파일은_goja에서_통과하고_통과_수는_파일_수와_같다(t *testing.T) {
	dir := filepath.Join("testdata", "conformance")
	paths, err := filepath.Glob(filepath.Join(dir, "*.ts"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("코퍼스 파일 %d 개, %v", len(paths), err)
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
		t.Fatalf("통과 %d / 파일 %d", passed, len(paths))
	}
}
