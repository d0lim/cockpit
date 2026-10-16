package ports

import "github.com/d0lim/cockpit/internal/engine"

type Mask func(string) string

func Identity(s string) string { return s }

func Denied(mask Mask, msg string) error { return perr(mask, engine.PortDenied, msg) }

func BadArgs(mask Mask, msg string) error { return perr(mask, engine.PortBadArgs, msg) }

func Limit(mask Mask, msg string) error { return perr(mask, engine.PortLimit, msg) }

func Timeout(mask Mask, msg string) error { return perr(mask, engine.PortTimeout, msg) }

func Unavailable(mask Mask, msg string) error { return perr(mask, engine.PortUnavailable, msg) }

func perr(mask Mask, code engine.PortCode, msg string) error {
	if mask == nil {
		mask = Identity
	}
	return &engine.PortError{Code: code, Message: mask(msg)}
}
