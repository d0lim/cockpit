package screen

import (
	"slices"
	"strconv"

	"github.com/d0lim/cockpit/internal/hub/api"
)

const wireCharWidth = 6.35

type wireRect struct {
	X, Y, W, H float64
	Class      string
}

type wirePath struct {
	D     string
	Class string
}

type wireText struct {
	X, Y   float64
	Text   string
	Tag    string
	Note   string
	Class  string
	Anchor string
	Href   string
}

type wireHit struct {
	Seq   int64
	Y, H  float64
	On    bool
	Here  bool
	Href  string
	Label string
}

type wireScene struct {
	Label      string
	Width      float64
	Height     float64
	HeadHeight float64
	Head       []wireText
	HeadPaths  []wirePath
	Bands      []wireRect
	Hits       []wireHit
	Lines      []wirePath
	Bars       []wireRect
	Paths      []wirePath
	Texts      []wireText
}

type wireLinks struct {
	picked int64
	call   func(seq int64) string
	unfold func(seq int64) string
}

func wireNum(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

func wireHitLabel(h api.WireHead) string {
	return "Call " + itoa64(h.Seq) + ": " + h.Caller + " to " + h.Target + ", " + h.Method + " " + orElse(h.Path, "(path not kept)") + ", " + wireStatusText(h)
}

func (s *wireScene) hit(l wireLinks, h api.WireHead, y, height float64) {
	on := l.picked == h.Seq
	here := on && !slices.ContainsFunc(s.Hits, func(o wireHit) bool { return o.Here })
	s.Hits = append(s.Hits, wireHit{Seq: h.Seq, Y: y, H: height, On: on, Here: here, Href: l.call(h.Seq), Label: wireHitLabel(h)})
}
