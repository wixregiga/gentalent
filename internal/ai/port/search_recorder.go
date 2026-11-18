package port

import "gentalent/internal/ai/domain"

type SearchRecorder interface {
	Record(subject domain.Subject, manifest domain.Manifest, result domain.Result) error
}
