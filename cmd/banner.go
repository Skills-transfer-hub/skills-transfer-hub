package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"sth/internal/fs"
)

// Largeur extérieure de la bannière (incluant les bordures `│`/`╭`/etc.).
const bannerOuterWidth = 78

// bannerFrame est un instantané du buddy à un moment de l'animation.
type bannerFrame struct {
	face  string
	emoji string
}

// playLaunchBanner affiche la bannière au démarrage. En TTY, joue une mini
// animation "réveil" (3 frames). Sinon, écrit uniquement la frame finale.
func (a *app) playLaunchBanner(finalState fs.BuddyState) {
	final := bannerFrameFor(finalState)

	if !isWriterTTY(a.stdout) {
		fmt.Fprintln(a.stdout, renderBannerFrame(final))
		return
	}

	frames := []bannerFrame{
		{face: "- -", emoji: "💤"}, // dort
		{face: "o.o", emoji: "👀"}, // se réveille
		final,
	}

	first := renderBannerFrame(frames[0])
	fmt.Fprint(a.stdout, first)
	height := strings.Count(first, "\n") + 1

	for _, f := range frames[1:] {
		time.Sleep(220 * time.Millisecond)
		// Remonte le curseur au début de la bannière, puis redessine.
		fmt.Fprintf(a.stdout, "\x1b[%dA\r", height)
		fmt.Fprint(a.stdout, renderBannerFrame(f))
	}
	fmt.Fprintln(a.stdout)
}

// renderBanner produit la bannière statique pour un état buddy donné.
func renderBanner(state fs.BuddyState) string {
	return renderBannerFrame(bannerFrameFor(state))
}

func bannerFrameFor(state fs.BuddyState) bannerFrame {
	v := buddyViewFor(state)
	return bannerFrame{face: v.face, emoji: v.emoji}
}

func renderBannerFrame(f bannerFrame) string {
	title := "STH " + valueOrDefault(version, "dev")
	tagline := "Skills Transfer Hub."
	tip := "Astuce : tapez `sth help` pour voir la liste des commandes."
	footer := "STH gère .claude/skills depuis un dépôt distant."

	innerFace := padFace(f.face, 3)         // toujours 3 chars
	innerBoxWidth := len(innerFace) + 2     // " >/< " entouré de barres = 5 + 2 = 7

	// Toutes les lignes de contenu (avec les marges gauches déjà appliquées).
	rows := []string{
		"",
		"      " + f.emoji,
		"    ╭" + strings.Repeat("─", innerBoxWidth-2) + "╮   " + title,
		"    │" + innerFace + "│   " + tagline,
		"    ╰" + strings.Repeat("─", innerBoxWidth-2) + "╯",
		"",
		"  " + tip,
		"  " + footer,
		"",
	}

	innerWidth := bannerOuterWidth - 2 // espace entre les bordures `│`
	var b strings.Builder
	b.WriteString("╭" + strings.Repeat("─", innerWidth) + "╮\n")
	for _, row := range rows {
		pad := innerWidth - displayCells(row)
		if pad < 0 {
			pad = 0
		}
		b.WriteString("│")
		b.WriteString(row)
		b.WriteString(strings.Repeat(" ", pad))
		b.WriteString("│\n")
	}
	b.WriteString("╰" + strings.Repeat("─", innerWidth) + "╯")
	return b.String()
}

// padFace centre un visage de longueur ≤3 dans une chaîne de longueur fixe
// (espace + face + espace = 5 cellules visuelles), pour que l'intérieur
// du cadre du buddy reste aligné quel que soit l'état.
func padFace(face string, faceCells int) string {
	if displayCells(face) >= faceCells {
		return " " + face + " "
	}
	pad := faceCells - displayCells(face)
	left := pad / 2
	right := pad - left
	return " " + strings.Repeat(" ", left) + face + strings.Repeat(" ", right) + " "
}

// displayCells approxime la largeur visuelle d'une chaîne en cellules de
// terminal. ASCII et caractères de boîte = 1 cellule ; emojis et symboles
// pictographiques = 2 cellules ; sélecteurs de variation = 0.
func displayCells(s string) int {
	width := 0
	for _, r := range s {
		switch {
		case r == 0xFE0F || r == 0x200D:
			// Sélecteur de variation / zero-width joiner : invisible.
		case isWideRune(r):
			width += 2
		default:
			width += 1
		}
	}
	return width
}

func isWideRune(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F: // hangul
		return true
	case r >= 0x2E80 && r <= 0x303E: // CJK
		return true
	case r >= 0x3041 && r <= 0x33FF:
		return true
	case r >= 0x3400 && r <= 0x4DBF:
		return true
	case r >= 0x4E00 && r <= 0x9FFF:
		return true
	case r >= 0xA000 && r <= 0xA4CF:
		return true
	case r >= 0xAC00 && r <= 0xD7A3:
		return true
	case r >= 0xF900 && r <= 0xFAFF:
		return true
	case r >= 0xFE30 && r <= 0xFE4F:
		return true
	case r >= 0xFF00 && r <= 0xFF60:
		return true
	case r >= 0xFFE0 && r <= 0xFFE6:
		return true
	case r >= 0x231A && r <= 0x23FF: // sablier ⌛ ⏳, horloges, contrôles média
		return true
	case r >= 0x2600 && r <= 0x26FF: // symboles divers (⚠ ✅ ❌)
		return true
	case r >= 0x2700 && r <= 0x27BF: // dingbats
		return true
	case r >= 0x1F300 && r <= 0x1FAFF: // pictogrammes (💧 😶 💤 👀 …)
		return true
	}
	return false
}

func isWriterTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return stat.Mode()&os.ModeCharDevice != 0
}
