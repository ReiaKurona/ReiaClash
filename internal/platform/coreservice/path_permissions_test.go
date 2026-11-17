package coreservice

import (
	"os"
	"strings"
	"testing"
)

func TestServicePathPermissions(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		mode                  os.FileMode
		owner                 int
		directory, executable bool
		want                  os.FileMode
		fail                  bool
	}{
		{"user executable from umask 002", 0775, 1000, false, true, 0755, false},
		{"user config directory", os.ModeDir | 0775, 1000, true, false, 0755, false},
		{"user private directory", os.ModeDir | 0700, 1000, true, false, 0700, false},
		{"system installed executable", 0755, 0, false, true, 0755, false},
		{"writable system executable", 0775, 0, false, true, 0, true},
		{"another user's executable", 0755, 1001, false, true, 0, true},
		{"root owned configuration", os.ModeDir | 0700, 0, true, false, 0, true},
		{"root owned token", 0600, 0, false, false, 0, true},
		{"non executable", 0644, 1000, false, true, 0, true},
		{"inaccessible directory", os.ModeDir | 0500, 1000, true, false, 0, true},
		{"not regular", os.ModeNamedPipe | 0600, 1000, false, false, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mode, err := servicePathMode("/example path", tc.mode, tc.owner, 1000, tc.directory, tc.executable)
			if (err != nil) != tc.fail || !tc.fail && mode != tc.want {
				t.Fatalf("mode=%04o err=%v", mode, err)
			}
			if err != nil && !strings.Contains(err.Error(), "/example path") {
				t.Fatal("missing failing path", err)
			}
		})
	}
}
