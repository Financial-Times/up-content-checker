package util

import (
	"flag"
	"net/http"
	"regexp"
	"strings"
)

type (
	Checker interface {
		Check(uuid string) ([][]string, error)
	}
)

var (
	uuidMatcher     = regexp.MustCompile("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")
	uuidPathMatcher = regexp.MustCompile(".*/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$")
	auth            string
)

func init() {
	flag.StringVar(&auth, "auth", "", "Basic authentication")
}

func IsUuid(uuid string) bool {
	return uuidMatcher.MatchString(uuid)
}

func ExtractUuid(url string) (string, bool) {
	match := uuidPathMatcher.FindStringSubmatch(url)
	if match == nil {
		return "", false
	}

	return match[1], true
}

func AddBasicAuthentication(req *http.Request) {
	if len(auth) > 0 {
		cred := strings.SplitN(auth, ":", 2)
		req.SetBasicAuth(cred[0], cred[1])
	}
}
