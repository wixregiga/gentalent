package port

type SourcesWriter interface {
	WriteSources(path, text string) error
}
