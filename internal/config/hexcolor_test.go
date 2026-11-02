package config

import (
	"strings"
	"testing"
)

type colorOnly struct {
	Items []struct {
		Color string `yaml:"color"`
	} `yaml:"items"`
	Color string `yaml:"color"`
}

func TestDecodeYAMLUnquotedColor(t *testing.T) {
	bad := []string{
		"color: #112233\n",
		"items:\n  - {code: a, color: #112233}\n",
		"items:\n  - { color: #112233 }\n",
	}
	for _, in := range bad {
		var v colorOnly
		if err := DecodeYAML([]byte(in), &v); err == nil || !strings.Contains(err.Error(), "v uvozovkách") {
			t.Errorf("%q: %v", in, err)
		}
	}
	ok := []string{
		"color: \"#112233\"\n",
		"items:\n  - {color: \"#112233\"}\n",
		"# example: {color: #112233}\ncolor: red\n",
		"  # color: #112233\ncolor: red\n",
	}
	for _, in := range ok {
		var v colorOnly
		if err := DecodeYAML([]byte(in), &v); err != nil {
			t.Errorf("%q: %v", in, err)
		}
	}
}
