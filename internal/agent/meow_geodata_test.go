package agent

import "testing"

func TestMeowCountryDatabaseReferences(t *testing.T) {
	cases := []struct {
		profile string
		want    bool
	}{
		{"rules: [MATCH,DIRECT]", false},
		{"rules: ['GEOIP,CN,DIRECT']", true},
		{"rules: ['SRC-GEOIP,US,DIRECT']", true},
		{"dns: {enable: true, fallback: [1.1.1.1]}", true},
		{"dns: {enable: true, fallback: [1.1.1.1], fallback-filter: {geoip: false}}", false},
	}
	for _, test := range cases {
		if got := needsCountryDatabase([]byte(test.profile)); got != test.want {
			t.Errorf("needsCountryDatabase(%q) = %t, want %t", test.profile, got, test.want)
		}
	}
}
