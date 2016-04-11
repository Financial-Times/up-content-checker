package util

import (
	"regexp"
)

var (
	uuidMatcher     = regexp.MustCompile("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")
	uuidPathMatcher = regexp.MustCompile(".*/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$")
)

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
