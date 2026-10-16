package engine

const driverName = "<driver>"

type Frame struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

func (h *Host) Frames() []Frame {
	var frames []Frame
	for _, sf := range h.rt.CaptureCallStack(0, nil) {
		pos := sf.Position()
		if pos.Filename == "" || pos.Filename == bundleName || pos.Filename == driverName || pos.Line < 1 {
			continue
		}
		frames = append(frames, Frame{File: pos.Filename, Line: pos.Line})
	}
	return frames
}
