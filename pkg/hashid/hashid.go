package hashid

import (
	"fmt"

	"github.com/speps/go-hashids"
)

type Config struct {
	Salt      string
	Alphabet  string
	MinLength int
}

type Encoder struct {
	hash *hashids.HashID
}

func New(cfg Config) (*Encoder, error) {
	if cfg.Salt == "" {
		return nil, fmt.Errorf("%w: salt is required", ErrInvalidConfig)
	}
	data := hashids.NewData()
	data.Salt = cfg.Salt
	data.MinLength = cfg.MinLength
	if cfg.Alphabet != "" {
		data.Alphabet = cfg.Alphabet
	}
	hash, err := hashids.NewWithData(data)
	if err != nil {
		return nil, err
	}
	return &Encoder{hash: hash}, nil
}

func (e *Encoder) Encode(values ...int) (string, error) {
	if e == nil || e.hash == nil {
		return "", fmt.Errorf("%w: nil encoder", ErrInvalidConfig)
	}
	if len(values) == 0 {
		return "", fmt.Errorf("%w: empty values", ErrInvalidValue)
	}
	return e.hash.Encode(values)
}

func (e *Encoder) Decode(raw string) ([]int, error) {
	if e == nil || e.hash == nil {
		return nil, fmt.Errorf("%w: nil encoder", ErrInvalidConfig)
	}
	values, err := e.hash.DecodeWithError(raw)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, ErrInvalidValue
	}
	return values, nil
}

func (e *Encoder) EncodeTyped(id uint64, typ int) (string, error) {
	if id > uint64(^uint(0)>>1) {
		return "", fmt.Errorf("%w: id overflows int", ErrInvalidValue)
	}
	return e.Encode(int(id), typ)
}

func (e *Encoder) DecodeTyped(raw string, typ int) (uint64, error) {
	values, err := e.Decode(raw)
	if err != nil {
		return 0, err
	}
	if len(values) != 2 || values[1] != typ || values[0] < 0 {
		return 0, ErrTypeMismatch
	}
	return uint64(values[0]), nil
}
