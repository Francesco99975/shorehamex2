package helpers

import "github.com/mileusna/useragent"

func GetUaOS(ua string) string {
	parsedUa := useragent.Parse(ua)

	return parsedUa.OS
}

func GetUaDeviceType(ua string) string {
	parsedUa := useragent.Parse(ua)

	var deviceType string
	if parsedUa.Mobile {
		deviceType = "mobile"
	} else if parsedUa.Tablet {
		deviceType = "tablet"
	} else if parsedUa.Desktop {
		deviceType = "desktop"
	} else {
		deviceType = "unknown"
	}

	return deviceType
}

func GetUaBrowser(ua string) string {
	parsedUa := useragent.Parse(ua)

	var browser string
	if parsedUa.IsChrome() {
		browser = "chrome"
	} else if parsedUa.IsFirefox() {
		browser = "firefox"
	} else if parsedUa.IsSafari() {
		browser = "safari"
	} else if parsedUa.IsEdge() {
		browser = "edge"
	} else if parsedUa.IsInternetExplorer() {
		browser = "ie"
	} else if parsedUa.IsOpera() {
		browser = "opera"
	} else {
		browser = "unknown"
	}

	return browser
}
