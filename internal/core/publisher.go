package core

// publisher is an entity that can publish a stream dynamically.
type publisher interface {
	source
	close()
	onPublisherAccepted(tracksLen int)
}
