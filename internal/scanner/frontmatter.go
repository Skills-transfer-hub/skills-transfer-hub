package scanner

import metadata "sth/internal/frontmatter"

type Frontmatter struct {
	Name        string
	Description string
	ApplyTo     []string
	Lang        string
}

func ExtractFrontmatter(content []byte) (Frontmatter, bool) {
	values, valid := metadata.Extract(content)
	if !valid {
		return Frontmatter{}, false
	}
	return Frontmatter{
		Name:        values.Name,
		Description: values.Description,
		ApplyTo:     values.ApplyTo,
		Lang:        values.Lang,
	}, true
}
