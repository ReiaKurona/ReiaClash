package panel

import "testing"

func TestActualFilePanelURLsAreSafeAndPreserved(t *testing.T) {
	for _, raw := range []string{
		"file:///C:/Users/tcs15/Desktop/ReiaClash/test-webui.html",
		"file:///C:/Users/My%20Name/test-webui.html?theme=dark#/nodes",
		"file:///home/user/ReiaClash/test-webui.html",
		"file:///Users/user/ReiaClash/test-webui.html",
	} {
		if target, err := NormalizeURL(raw); err != nil || target != raw {
			t.Fatalf("%s -> %s (%v)", raw, target, err)
		}
	}
	for _, raw := range []string{"file:relative.html", "file://server/share/panel.html", "file:///", "javascript:alert(1)", "file:///C:/bad%00.html"} {
		if _, err := NormalizeURL(raw); err == nil {
			t.Fatalf("accepted unsafe or invalid URL %s", raw)
		}
	}
}
