package domain

type Provider struct {
	CLI   string
	Model string
}

func (p Provider) Label() string {
	if p.Model == "" {
		return p.CLI
	}
	return p.CLI + ":" + p.Model
}
