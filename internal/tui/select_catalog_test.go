package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbletea"
)

func TestLargeCatalogKeepsSelectedOptionVisible(t *testing.T) {
	options := make([]Option, 40)
	for index := range options {
		options[index] = Option{Label: fmt.Sprintf("skill-%02d", index), Description: "Description utile"}
	}
	model := newSelectModel(SelectConfig{Options: options, PageSize: 8})
	for index := 0; index < 9; index++ {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(selectModel)
	}
	view := model.View()
	if !strings.Contains(view, "skill-09") || !strings.Contains(view, "Description utile") || strings.Contains(view, "skill-00") || strings.Contains(view, "skill-39") {
		t.Fatalf("unbounded or misplaced catalog: %s", view)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if selected := updated.(selectModel).chosenIndex; selected != 9 {
		t.Fatalf("wrong original index: %d", selected)
	}
}

func TestSameSkillNameRequiresAnExplicitSource(t *testing.T) {
	options := []Option{{Label: "alpha [public]", Aliases: []string{"alpha", "public:skills/alpha"}}, {Label: "alpha [private]", Aliases: []string{"alpha", "private:skills/alpha"}}}
	if _, err := resolveSelectToken("alpha", options, 0); err == nil {
		t.Fatal("ambiguous source selected silently")
	}
	if index, err := resolveSelectToken("private:skills/alpha", options, 0); err != nil || index != 1 {
		t.Fatalf("qualified selection failed: %d %v", index, err)
	}
}
