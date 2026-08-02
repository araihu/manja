package web

import (
	"encoding/base64"
	"encoding/json"
	"html"
	"regexp"
)

type encodedSelectOption struct {
	Value string `json:"value"`
}

type encodedSelectConfig struct {
	Options        []encodedSelectOption `json:"options"`
	SelectedValues []string              `json:"selectedValues"`
}

func decodedSelectConfigs(body string) []encodedSelectConfig {
	var configs []encodedSelectConfig
	for _, match := range regexp.MustCompile(`data-select-config="([^"]+)"`).FindAllStringSubmatch(body, -1) {
		decoded, err := base64.StdEncoding.DecodeString(html.UnescapeString(match[1]))
		if err != nil {
			continue
		}
		var config encodedSelectConfig
		if err := json.Unmarshal(decoded, &config); err == nil {
			configs = append(configs, config)
		}
	}
	return configs
}

func selectConfigContainsValues(body string, values ...string) bool {
	found := make(map[string]bool, len(values))
	for _, config := range decodedSelectConfigs(body) {
		for _, option := range config.Options {
			found[option.Value] = true
		}
	}
	for _, value := range values {
		if !found[value] {
			return false
		}
	}
	return true
}

func selectConfigStartsWith(body string, values, selected []string) bool {
	for _, config := range decodedSelectConfigs(body) {
		if len(config.Options) < len(values) || len(config.SelectedValues) != len(selected) {
			continue
		}
		matches := true
		for index, value := range values {
			matches = matches && config.Options[index].Value == value
		}
		for index, value := range selected {
			matches = matches && config.SelectedValues[index] == value
		}
		if matches {
			return true
		}
	}
	return false
}
