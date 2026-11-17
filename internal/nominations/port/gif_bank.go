package port

type GifBank interface {
	Pick(uses map[string]int) (string, error)
}
