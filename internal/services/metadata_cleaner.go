package services

import "streamgo/internal/metadata"

// Expose metadata cleaner functions in services package for convenience
var (
	NormalizePunctuation  = metadata.NormalizePunctuation
	CleanDuplicatedPhrase = metadata.CleanDuplicatedPhrase
	CleanArtist           = metadata.CleanArtist
	CleanTitle            = metadata.CleanTitle
	CleanMetadata         = metadata.CleanMetadata
)
