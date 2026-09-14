package browserheaders

import (
	"net/http"
	"testing"
)

type headerExpectation struct {
	key   string
	value string
	// present=false asserts the header stays unset.
	present bool
}

func checkHeaders(t *testing.T, header http.Header, expectations []headerExpectation) {
	t.Helper()
	for _, expectation := range expectations {
		got, ok := header[expectation.key]
		if !ok && expectation.present {
			t.Fatalf("%s = unset, want %q", expectation.key, expectation.value)
		}
		if !ok {
			continue
		}
		if got[0] != expectation.value {
			t.Fatalf("%s = %q, want %q", expectation.key, got[0], expectation.value)
		}
	}
}

func TestApplyChromiumClientHintsDesktopChrome(t *testing.T) {
	header := http.Header{}
	ApplyChromiumClientHints(header, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	checkHeaders(t, header, []headerExpectation{
		{key: "Sec-Ch-Ua", value: `"Google Chrome";v="126", "Chromium";v="126", "Not(A:Brand";v="24"`, present: true},
		{key: "Sec-Ch-Ua-Mobile", value: "?0", present: true},
		{key: "Sec-Ch-Ua-Platform", value: `"Windows"`, present: true},
		{key: "Sec-Ch-Ua-Arch", value: "x86", present: true},
		{key: "Sec-Ch-Ua-Bitness", value: "64", present: true},
	})
}

func TestApplyChromiumClientHintsEdgeBrand(t *testing.T) {
	header := http.Header{}
	ApplyChromiumClientHints(header, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 Edg/126.0.0.0")
	checkHeaders(t, header, []headerExpectation{
		{key: "Sec-Ch-Ua", value: `"Microsoft Edge";v="126", "Chromium";v="126", "Not(A:Brand";v="24"`, present: true},
	})
}

func TestApplyChromiumClientHintsMobileChrome(t *testing.T) {
	header := http.Header{}
	ApplyChromiumClientHints(header, "Mozilla/5.0 (Linux; Android 14; aarch64; Pixel 8 Build/UD1A.230803.041) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36")
	checkHeaders(t, header, []headerExpectation{
		{key: "Sec-Ch-Ua-Mobile", value: "?1", present: true},
		{key: "Sec-Ch-Ua-Platform", value: `"Android"`, present: true},
		{key: "Sec-Ch-Ua-Arch", value: "arm", present: true},
	})
}

func TestApplyChromiumClientHintsIOSCrios(t *testing.T) {
	header := http.Header{}
	ApplyChromiumClientHints(header, "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/126.0.6478.54 Mobile/15E148 Safari/604.1")
	checkHeaders(t, header, []headerExpectation{
		{key: "Sec-Ch-Ua", value: `"Google Chrome";v="126", "Chromium";v="126", "Not(A:Brand";v="24"`, present: true},
		{key: "Sec-Ch-Ua-Mobile", value: "?1", present: true},
		{key: "Sec-Ch-Ua-Platform", value: `"iOS"`, present: true},
	})
}

func TestApplyChromiumClientHintsChromiumBrandOnLinux(t *testing.T) {
	header := http.Header{}
	ApplyChromiumClientHints(header, "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chromium/125.0.0.0 Safari/537.36")
	checkHeaders(t, header, []headerExpectation{
		{key: "Sec-Ch-Ua", value: `"Chromium";v="125", "Chromium";v="125", "Not(A:Brand";v="24"`, present: true},
		{key: "Sec-Ch-Ua-Platform", value: `"Linux"`, present: true},
		{key: "Sec-Ch-Ua-Arch", value: "x86", present: true},
	})
}

func TestApplyChromiumClientHintsNonChromiumUserAgentsGetNoHints(t *testing.T) {
	for _, userAgent := range []string{
		"curl/7.68.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15",
		"",
	} {
		header := http.Header{}
		ApplyChromiumClientHints(header, userAgent)
		checkHeaders(t, header, []headerExpectation{
			{key: "Sec-Ch-Ua", present: false},
			{key: "Sec-Ch-Ua-Mobile", present: false},
			{key: "Sec-Ch-Ua-Platform", present: false},
		})
	}
}
