package tracking

import "github.com/google/uuid"

func NewTrackID() string {
	return uuid.New().String()
}

func GenerateTrackID(generator string) string {
	switch generator {
	case "", generatorUUID:
		return NewTrackID()
	default:
		return NewTrackID()
	}
}
