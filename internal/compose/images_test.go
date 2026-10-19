package compose

import (
	"context"
	"slices"
	"testing"
)

func TestBuiltImagesListOnlyServicesWithABuildSectionInServiceOrder(t *testing.T) {
	config := map[string]any{"services": map[string]any{
		"worker": map[string]any{"image": "toy/worker:bbbbbbbbbbbb", "build": map[string]any{"context": "w"}},
		"db":     map[string]any{"image": "postgres:17"},
		"app":    map[string]any{"image": "toy/app:aaaaaaaaaaaa", "build": map[string]any{"context": "a"}},
		"plain":  map[string]any{"build": map[string]any{"context": "p"}},
	}}
	want := []BuiltImage{{"app", "toy/app:aaaaaaaaaaaa"}, {"plain", ""}, {"worker", "toy/worker:bbbbbbbbbbbb"}}
	if got := BuiltImages(config); !slices.Equal(got, want) {
		t.Fatalf("%+v", got)
	}
	if got := BuiltImages(map[string]any{}); len(got) != 0 {
		t.Fatalf("no services gave %+v", got)
	}
}

func TestOnlyATagEqualToAnImageKeyCountsAsAContentAddress(t *testing.T) {
	keys := map[string]string{"IMG_APP": "aaaaaaaaaaaa"}
	cases := map[string]bool{
		"toy/app:aaaaaaaaaaaa":                true,
		"localhost:5000/toy/app:aaaaaaaaaaaa": true,
		"toy/app:latest":                      false,
		"aaaaaaaaaaaa/app:latest":             false,
		"localhost:5000/toy/app":              false,
		"toy/app":                             false,
		"":                                    false,
	}
	for image, want := range cases {
		if got := KeyedTag(image, keys); got != want {
			t.Errorf("%q: %v, want %v", image, got, want)
		}
	}
	if len(cases) != 7 {
		t.Fatal("case count")
	}
}

type inspectDocker map[string]string

func (d inspectDocker) Run(_ context.Context, cmd Command) (Result, error) {
	if id, ok := d[cmd.Args[len(cmd.Args)-1]]; ok && cmd.Args[0] == "image" && cmd.Args[1] == "inspect" {
		return Result{Stdout: []byte(id + "\n")}, nil
	}
	return Result{Code: 1}, nil
}

func TestImagePresentNeedsAnImageIDFromImageInspect(t *testing.T) {
	p := Project{Docker: inspectDocker{"toy/app:aaaaaaaaaaaa": "sha256:0123", "toy/odd:aaaaaaaaaaaa": "not-an-id"}}
	if !p.ImagePresent(context.Background(), "toy/app:aaaaaaaaaaaa") {
		t.Fatal("a present image reads as missing")
	}
	for _, ref := range []string{"toy/missing:aaaaaaaaaaaa", "toy/odd:aaaaaaaaaaaa"} {
		if p.ImagePresent(context.Background(), ref) {
			t.Errorf("%s reads as present", ref)
		}
	}
}
