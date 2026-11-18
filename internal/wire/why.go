package wire

import "strconv"

func (h Head) Why(span, step string, parent *Head) string {
	switch h.Link.Kind {
	case LinkExact:
		return whyExact(h, span, step)
	case LinkNested:
		if parent == nil {
			return h.Caller + " was answering a call of step " + step + " for the whole of this call."
		}
		return h.Caller + " was answering call " + strconv.FormatInt(parent.Seq, 10) + " (" + parent.Caller + " to " + parent.Target + ") for the whole of this call, so it belongs to step " + step + "."
	case LinkTime:
		return "This call started while step " + step + " ran, and nothing else ties it to the step."
	}
	return "This call started outside every step."
}

func whyExact(h Head, span, step string) string {
	switch {
	case h.Caller == runnerCaller && span != "":
		return "The runner sent this request for step " + step + ", and it carried span " + span + "."
	case h.Caller == runnerCaller:
		return "The runner sent this request while step " + step + " ran."
	case span != "":
		return "This call carried span " + span + ", the span of a request the runner sent for step " + step + "."
	}
	return "This call carried the span of a request the runner sent for step " + step + "."
}
