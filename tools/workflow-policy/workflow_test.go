package workflowpolicy

import (
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPrivateContractOnlyRunsOnCanonicalPushes(t *testing.T) {
	workflow := readWorkflow(t, "ci.yml")
	job := workflow.Jobs["sth-code-contract"]
	var condition string
	for _, step := range job.Steps {
		if step.ID == "contract-source" {
			condition = step.If
		}
	}
	if condition == "" {
		t.Fatal("private contract download must have a condition")
	}
	for _, repository := range []string{"Skills-transfer-hub/skills-transfer-hub", "contributor/skills-transfer-hub"} {
		for _, event := range []string{"push", "pull_request", "workflow_dispatch"} {
			t.Run(repository+"/"+event, func(t *testing.T) {
				context := workflowContext(repository, event, "refs/heads/main")
				expected := repository == "Skills-transfer-hub/skills-transfer-hub" && event == "push"
				if actual := evaluateCondition(t, condition, context); actual != expected {
					t.Fatalf("private contract download = %v, want %v", actual, expected)
				}
			})
		}
	}
}

func TestPublicationJobsRejectForksAndPreserveCanonicalReleases(t *testing.T) {
	for _, scenario := range []struct {
		filename string
		job      string
		events   []string
	}{
		{"ci.yml", "auto-release", []string{"push"}},
		{"auto-release.yml", "release-eligibility", []string{"push"}},
		{"auto-release.yml", "create-release", []string{"push"}},
		{"release.yml", "goreleaser", []string{"release", "workflow_dispatch"}},
		{"release.yml", "dashboard-sync", []string{"release", "workflow_dispatch"}},
		{"release.yml", "chocolatey", []string{"release", "workflow_dispatch"}},
		{"publish-chocolatey.yml", "chocolatey", []string{"release", "workflow_dispatch"}},
	} {
		workflow := readWorkflow(t, scenario.filename)
		job, exists := workflow.Jobs[scenario.job]
		if !exists {
			t.Fatalf("missing job %s in %s", scenario.job, scenario.filename)
		}
		for _, event := range scenario.events {
			for _, repository := range []string{"Skills-transfer-hub/skills-transfer-hub", "contributor/skills-transfer-hub"} {
				t.Run(scenario.filename+"/"+scenario.job+"/"+event+"/"+repository, func(t *testing.T) {
					context := workflowContext(repository, event, "refs/heads/main")
					expected := repository == "Skills-transfer-hub/skills-transfer-hub"
					if actual := evaluateCondition(t, job.If, context); actual != expected {
						t.Fatalf("publication = %v, want %v", actual, expected)
					}
				})
			}
		}
	}
}

type workflowDefinition struct {
	Jobs map[string]struct {
		If    string
		Steps []struct {
			ID string `yaml:"id"`
			If string
		}
	}
}

func readWorkflow(t *testing.T, filename string) workflowDefinition {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", filename))
	if err != nil {
		t.Fatal(err)
	}
	var workflow workflowDefinition
	if err := yaml.Unmarshal(content, &workflow); err != nil {
		t.Fatal(err)
	}
	return workflow
}

func workflowContext(repository, event, ref string) map[string]string {
	return map[string]string{
		"github.repository":                         repository,
		"github.event_name":                         event,
		"github.ref":                                ref,
		"needs.ci-gate.result":                      "success",
		"needs.release-eligibility.outputs.current": "true",
		"needs.goreleaser.outputs.prerelease":       "false",
	}
}

func evaluateCondition(t *testing.T, expression string, context map[string]string) bool {
	t.Helper()
	if expression == "" {
		return true
	}
	expression = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(expression, "${{"), "}}"))
	expression = strings.ReplaceAll(expression, "'", "\"")
	for name, value := range context {
		expression = strings.ReplaceAll(expression, name, strconv.Quote(value))
	}
	result, err := types.Eval(token.NewFileSet(), nil, token.NoPos, expression)
	if err != nil {
		t.Fatal(err)
	}
	if result.Value == nil || result.Value.Kind() != constant.Bool {
		t.Fatalf("condition is not boolean: %s", expression)
	}
	return constant.BoolVal(result.Value)
}
