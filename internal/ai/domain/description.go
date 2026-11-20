package domain

import "path/filepath"

const figureSlug = "public_figure"

type Description struct {
	Text string
	Dir  string
}

func (d Description) Raw() string {
	return filepath.Join(d.Dir, rawFolder)
}

func (d Description) PhotoPath(name string) string {
	return filepath.Join(d.Dir, slug(name, figureSlug)+".jpg")
}
