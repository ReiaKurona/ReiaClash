package mihomo

import "go.yaml.in/yaml/v3"

func ProfileGroupOrder(profile []byte) ([]string, error) {
	var document struct {
		Groups []struct {
			Name string `yaml:"name"`
		} `yaml:"proxy-groups"`
	}
	if err := yaml.Unmarshal(profile, &document); err != nil {
		return nil, err
	}
	order := make([]string, 0, len(document.Groups))
	seen := map[string]bool{}
	for _, group := range document.Groups {
		if group.Name != "" && !seen[group.Name] {
			order = append(order, group.Name)
			seen[group.Name] = true
		}
	}
	return order, nil
}
