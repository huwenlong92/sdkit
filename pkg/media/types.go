// Package media defines provider-neutral media metadata probing contracts.
package media

import (
	"context"
	"time"
)

type Input struct {
	Path string
}

type ProbeOptions struct {
	OutputLimit int64
}

type Prober interface {
	Probe(ctx context.Context, input Input, opts ProbeOptions) (Info, error)
}

type Info struct {
	Container       string
	ContainerLong   string
	Duration        time.Duration
	SizeBytes       int64
	BitRate         int64
	VideoStreams    []VideoStream
	AudioStreams    []AudioStream
	SubtitleStreams []SubtitleStream
}

type VideoStream struct {
	Index                int
	Codec                string
	Profile              string
	Width                int
	Height               int
	PixelFormat          string
	FrameRate            float64
	FrameRateRatio       string
	Rotation             int
	ColorRange           string
	ColorSpace           string
	ColorTransfer        string
	ColorPrimaries       string
	HighDynamicRange     bool
	HighDynamicRangeType string
	Language             string
}

type AudioStream struct {
	Index         int
	Codec         string
	Profile       string
	SampleRate    int
	Channels      int
	ChannelLayout string
	Language      string
}

type SubtitleStream struct {
	Index    int
	Codec    string
	Language string
	Title    string
}
