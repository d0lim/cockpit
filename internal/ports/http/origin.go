package httpport

import (
	"net"
	"net/url"
)

func Origin(value string) (string, bool) {
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || net.ParseIP(u.Hostname()) != nil {
		return "", false
	}
	if u.Path != "" && u.Path != "/" {
		return "", false
	}
	return u.Scheme + "://" + u.Host, true
}
