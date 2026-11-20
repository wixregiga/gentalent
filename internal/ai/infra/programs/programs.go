package programs

import "gentalent/internal/ai/port"

func Programs() map[string]port.Agent {
	return map[string]port.Agent{
		"codex":  Codex{},
		"claude": Claude{},
		"agy":    Agy{},
	}
}

func WhoAgents() map[string]port.WhoAgent {
	return map[string]port.WhoAgent{
		"codex":  Codex{},
		"claude": Claude{},
		"agy":    Agy{},
	}
}
