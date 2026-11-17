package mihomo

import (
	"reflect"
	"testing"
)

func TestProfileGroupOrderPreservesYAMLSequence(t *testing.T) {
	for _, profile := range []string{
		"proxy-groups:\n  - { name: '站点: A', type: select }\n  - name: Z\n  - name: A\n",
		`proxy-groups: [{name: "站点: A"}, {name: Z}, {name: A}]`,
	} {
		got, err := ProfileGroupOrder([]byte(profile))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, []string{"站点: A", "Z", "A"}) {
			t.Fatal(got)
		}
	}
}
