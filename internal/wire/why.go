package wire

import "strconv"

func (h Head) Why(span, step string, parent *Head) string {
	switch h.Link.Kind {
	case LinkExact:
		return whyExact(h, span, step)
	case LinkTraced:
		return whyTraced(h, step, parent)
	case LinkBackground:
		return whyBackground(h, step)
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

func whyTraced(h Head, step string, parent *Head) string {
	reached := "another recorded call"
	if parent != nil {
		reached = "call " + strconv.FormatInt(parent.Seq, 10) + " (" + parent.Caller + " to " + parent.Target + ")"
	}
	text := "The span chain of this call leads to " + reached + "."
	if h.Link.Route {
		text = "The span chain of this call starts at a request that matches " + reached + " by service, method, route and time."
	}
	if step != "" {
		text += " It is listed under step " + step + "."
	}
	return text
}

func whyBackground(h Head, step string) string {
	text := "The span chain of this call starts at a root that no recorded call started"
	if h.Link.Root != "" {
		text += ": " + h.Link.Root
	}
	text += "."
	if step != "" {
		text += " It ran while step " + step + " ran, and nothing ties it to the step."
	}
	return text
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
