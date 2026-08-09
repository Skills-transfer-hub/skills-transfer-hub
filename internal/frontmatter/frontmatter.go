package frontmatter

import (
	"bytes"
	"strings"

	"gopkg.in/yaml.v3"
)

type Values struct {
	Name        string
	Description string
	ApplyTo     []string
	Lang        string
}

func Extract(content []byte) (Values, bool) {
	mapping, valid := parseMapping(content)
	if !valid {
		return Values{}, false
	}
	name, _ := stringFromMapping(mapping, "name")
	description, _ := stringFromMapping(mapping, "description")
	language, _ := stringFromMapping(mapping, "lang")
	applyTo, _ := stringsFromMapping(mapping, "applyTo")
	if len(applyTo) == 0 {
		applyTo, _ = stringsFromMapping(mapping, "globs")
	}
	if len(applyTo) == 0 {
		applyTo, _ = stringsFromMapping(mapping, "paths")
	}
	return Values{
		Name:        name,
		Description: description,
		ApplyTo:     applyTo,
		Lang:        language,
	}, true
}

func String(content []byte, key string) (string, bool) {
	value, present, valid := LookupString(content, key)
	if !present || !valid {
		return "", false
	}
	return value, true
}

func LookupString(content []byte, key string) (string, bool, bool) {
	mapping, valid := parseMapping(content)
	if !valid {
		return "", false, false
	}
	if _, present := mapping[key]; !present {
		return "", false, true
	}
	value, valid := stringFromMapping(mapping, key)
	return value, true, valid
}

func parseMapping(content []byte) (map[string]*yaml.Node, bool) {
	document, valid := frontmatterDocument(content)
	if !valid {
		return nil, false
	}
	if len(bytes.TrimSpace(document)) == 0 {
		return map[string]*yaml.Node{}, true
	}
	root := yaml.Node{}
	if err := yaml.Unmarshal(document, &root); err != nil || len(root.Content) != 1 {
		return nil, false
	}
	mappingNode := root.Content[0]
	if mappingNode.Kind != yaml.MappingNode || len(mappingNode.Content)%2 != 0 {
		return nil, false
	}
	mapping := make(map[string]*yaml.Node, len(mappingNode.Content)/2)
	for nodeIndex := 0; nodeIndex < len(mappingNode.Content); nodeIndex += 2 {
		keyNode := mappingNode.Content[nodeIndex]
		valueNode := mappingNode.Content[nodeIndex+1]
		if keyNode.Kind != yaml.ScalarNode || keyNode.Tag != "!!str" || keyNode.Value == "" {
			return nil, false
		}
		if _, duplicate := mapping[keyNode.Value]; duplicate {
			return nil, false
		}
		mapping[keyNode.Value] = valueNode
	}
	return mapping, true
}

func frontmatterDocument(content []byte) ([]byte, bool) {
	content = bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})
	openingLength := 0
	switch {
	case bytes.HasPrefix(content, []byte("---\n")):
		openingLength = 4
	case bytes.HasPrefix(content, []byte("---\r\n")):
		openingLength = 5
	default:
		return nil, false
	}
	body := content[openingLength:]
	for offset := 0; offset <= len(body); {
		newlineIndex := bytes.IndexByte(body[offset:], '\n')
		line := body[offset:]
		nextOffset := len(body)
		if newlineIndex >= 0 {
			line = body[offset : offset+newlineIndex]
			nextOffset = offset + newlineIndex + 1
		}
		if bytes.Equal(bytes.TrimSuffix(line, []byte("\r")), []byte("---")) {
			return body[:offset], true
		}
		if newlineIndex < 0 {
			break
		}
		offset = nextOffset
	}
	return nil, false
}

func stringFromMapping(mapping map[string]*yaml.Node, key string) (string, bool) {
	valueNode, exists := mapping[key]
	if !exists || valueNode.Kind != yaml.ScalarNode || valueNode.Tag != "!!str" {
		return "", false
	}
	return valueNode.Value, true
}

func stringsFromMapping(mapping map[string]*yaml.Node, key string) ([]string, bool) {
	valueNode, exists := mapping[key]
	if !exists {
		return nil, false
	}
	switch valueNode.Kind {
	case yaml.ScalarNode:
		if valueNode.Tag != "!!str" {
			return nil, false
		}
		return splitStrings(valueNode.Value), true
	case yaml.SequenceNode:
		values := make([]string, 0, len(valueNode.Content))
		for _, itemNode := range valueNode.Content {
			if itemNode.Kind != yaml.ScalarNode || itemNode.Tag != "!!str" {
				return nil, false
			}
			value := strings.TrimSpace(itemNode.Value)
			if value != "" {
				values = append(values, value)
			}
		}
		return values, true
	default:
		return nil, false
	}
}

func splitStrings(value string) []string {
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}
