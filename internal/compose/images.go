package compose

import (
	"context"
	"slices"
	"strings"
)

type BuiltImage struct {
	Service string
	Image   string
}

func BuiltImages(config map[string]any) []BuiltImage {
	services, _ := config["services"].(map[string]any)
	var out []BuiltImage
	for name, raw := range services {
		svc, _ := raw.(map[string]any)
		if _, builds := svc["build"]; !builds {
			continue
		}
		image, _ := svc["image"].(string)
		out = append(out, BuiltImage{Service: name, Image: image})
	}
	slices.SortFunc(out, func(a, b BuiltImage) int { return strings.Compare(a.Service, b.Service) })
	return out
}

func KeyedTag(image string, keys map[string]string) bool {
	slash, colon := strings.LastIndex(image, "/"), strings.LastIndex(image, ":")
	if colon <= slash {
		return false
	}
	tag := image[colon+1:]
	for _, key := range keys {
		if tag == key {
			return true
		}
	}
	return false
}

func (p Project) ImagePresent(ctx context.Context, ref string) bool {
	res, err := p.Docker.Run(ctx, Command{Args: []string{"image", "inspect", "--format", "{{.Id}}", ref}})
	return err == nil && res.Code == 0 && strings.HasPrefix(strings.TrimSpace(string(res.Stdout)), "sha256:")
}
