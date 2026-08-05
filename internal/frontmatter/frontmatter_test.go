package frontmatter

import (
	"reflect"
	"testing"
)

func TestExtractParsesNativeYAMLValues(t *testing.T) {
	content := []byte("---\nname: review\ndescription: >-\n  Review code\n  and tests.\napplyTo:\n  - '**/*.go'\n  - '**/*.md'\nlang: fr\n---\nbody\n")
	values, valid := Extract(content)
	if !valid {
		t.Fatal("frontmatter rejected")
	}
	if values.Name != "review" || values.Description != "Review code and tests." || values.Lang != "fr" {
		t.Fatalf("values = %+v", values)
	}
	if !reflect.DeepEqual(values.ApplyTo, []string{"**/*.go", "**/*.md"}) {
		t.Fatalf("applyTo = %v", values.ApplyTo)
	}
}

func TestStringRejectsInvalidOrNonStringValues(t *testing.T) {
	testCases := []string{
		"---\ndescription: foo: bar\n---\n",
		"---\ndescription: [foo, bar]\n---\n",
		"---\ndescription: null\n---\n",
		"---\ndescription: 42\n---\n",
		"---\ndescription: first\ndescription: second\n---\n",
		"---\nmetadata:\n  description: hidden\n---\n",
	}
	for _, content := range testCases {
		if value, valid := String([]byte(content), "description"); valid || value != "" {
			t.Fatalf("value = %q, valid = %t for %q", value, valid, content)
		}
	}
}

func TestStringParsesMultilineQuotedAndPlainScalars(t *testing.T) {
	testCases := []struct {
		content  string
		expected string
	}{
		{content: "---\ndescription: Review\n  code carefully\n---\n", expected: "Review code carefully"},
		{content: "---\ndescription: \"Review\n  code carefully\"\n---\n", expected: "Review code carefully"},
	}
	for _, testCase := range testCases {
		value, valid := String([]byte(testCase.content), "description")
		if !valid || value != testCase.expected {
			t.Fatalf("value = %q, valid = %t, want %q", value, valid, testCase.expected)
		}
	}
}

func TestLookupStringDistinguishesAbsentInvalidAndStringValues(t *testing.T) {
	testCases := []struct {
		name            string
		content         string
		expectedValue   string
		expectedPresent bool
		expectedValid   bool
	}{
		{name: "absent", content: "---\ndescription: ready\n---\n", expectedValid: true},
		{name: "invalid frontmatter", content: "---\nname: [invalid\n---\n"},
		{name: "non string", content: "---\nname: 42\n---\n", expectedPresent: true},
		{name: "string", content: "---\nname: runtime-skill\n---\n", expectedValue: "runtime-skill", expectedPresent: true, expectedValid: true},
	}
	for _, testCase := range testCases {
		value, present, valid := LookupString([]byte(testCase.content), "name")
		if value != testCase.expectedValue || present != testCase.expectedPresent || valid != testCase.expectedValid {
			t.Errorf("%s: value = %q, present = %t, valid = %t", testCase.name, value, present, valid)
		}
	}
}

func TestExtractAcceptsEmptyFrontmatter(t *testing.T) {
	values, valid := Extract([]byte("---\n---\nbody\n"))
	if !valid || !reflect.DeepEqual(values, Values{}) {
		t.Fatalf("values = %+v, valid = %t", values, valid)
	}
}
