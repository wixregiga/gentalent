package agents

import "gentalent/internal/ai/port"

func Programs() map[string]port.Agent {
	return map[string]port.Agent{
		"codex":  Codex{},
		"claude": Claude{},
		"agy":    Agy{},
	}
}
