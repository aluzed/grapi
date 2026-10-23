package session

import "regexp"

// The device label of @strapi/utils' user-agent.js: a heuristic for the
// sessions list, not a full parser. Order matters, as in Strapi.
var (
	browsers = []struct {
		re   *regexp.Regexp
		name string
	}{
		{regexp.MustCompile(`\bEdg(?:e|A|iOS)?/`), "Edge"},
		{regexp.MustCompile(`\b(?:OPR|Opera)/`), "Opera"},
		{regexp.MustCompile(`\bSamsungBrowser/`), "Samsung Internet"},
		{regexp.MustCompile(`\bVivaldi/`), "Vivaldi"},
		{regexp.MustCompile(`\b(?:YaBrowser|Yowser)/`), "Yandex Browser"},
		{regexp.MustCompile(`\bBrave/`), "Brave"},
		{regexp.MustCompile(`\b(?:Firefox|FxiOS)/`), "Firefox"},
		{regexp.MustCompile(`\b(?:Chrome|CriOS|Chromium)/`), "Chrome"},
		{regexp.MustCompile(`\bSafari/`), "Safari"},
	}
	systems = []struct {
		re   *regexp.Regexp
		name string
	}{
		{regexp.MustCompile(`\bWindows NT\b`), "Windows"},
		{regexp.MustCompile(`\b(?:iPhone|iPad|iPod)\b`), "iOS"},
		{regexp.MustCompile(`\bAndroid\b`), "Android"},
		{regexp.MustCompile(`\b(?:Macintosh|Mac OS X)\b`), "macOS"},
		{regexp.MustCompile(`\bCrOS\b`), "ChromeOS"},
		{regexp.MustCompile(`\bLinux\b`), "Linux"},
	}
)

// DeviceName is "Chrome on macOS", the browser or the system alone, or "".
func DeviceName(userAgent string) string {
	if len(userAgent) > 1024 {
		userAgent = userAgent[:1024]
	}
	first := func(list []struct {
		re   *regexp.Regexp
		name string
	},
	) string {
		for _, x := range list {
			if x.re.MatchString(userAgent) {
				return x.name
			}
		}
		return ""
	}
	browser, os := first(browsers), first(systems)
	switch {
	case browser != "" && os != "":
		return browser + " on " + os
	case browser != "":
		return browser
	}
	return os
}
