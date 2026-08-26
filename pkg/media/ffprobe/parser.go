package ffprobe

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/pkg/media"
)

type probeDocument struct {
	Format  probeFormat   `json:"format"`
	Streams []probeStream `json:"streams"`
}

type probeFormat struct {
	FormatName     string `json:"format_name"`
	FormatLongName string `json:"format_long_name"`
	Duration       string `json:"duration"`
	Size           string `json:"size"`
	BitRate        string `json:"bit_rate"`
}

type probeStream struct {
	Index          int               `json:"index"`
	CodecName      string            `json:"codec_name"`
	CodecType      string            `json:"codec_type"`
	Profile        string            `json:"profile"`
	Width          int               `json:"width"`
	Height         int               `json:"height"`
	PixelFormat    string            `json:"pix_fmt"`
	AverageRate    string            `json:"avg_frame_rate"`
	RealRate       string            `json:"r_frame_rate"`
	SampleRate     string            `json:"sample_rate"`
	Channels       int               `json:"channels"`
	ChannelLayout  string            `json:"channel_layout"`
	ColorRange     string            `json:"color_range"`
	ColorSpace     string            `json:"color_space"`
	ColorTransfer  string            `json:"color_transfer"`
	ColorPrimaries string            `json:"color_primaries"`
	Tags           map[string]string `json:"tags"`
	SideData       []probeSideData   `json:"side_data_list"`
}

type probeSideData struct {
	Rotation *float64 `json:"rotation"`
}

func parseOutput(output []byte) (media.Info, error) {
	var document probeDocument
	if err := json.Unmarshal(output, &document); err != nil {
		return media.Info{}, err
	}
	if document.Format.FormatName == "" && len(document.Streams) == 0 {
		return media.Info{}, fmt.Errorf("metadata is empty")
	}
	info := media.Info{
		Container:     document.Format.FormatName,
		ContainerLong: document.Format.FormatLongName,
		SizeBytes:     parseInt64(document.Format.Size),
		BitRate:       parseInt64(document.Format.BitRate),
	}
	if seconds, err := strconv.ParseFloat(document.Format.Duration, 64); err == nil && seconds >= 0 && !math.IsInf(seconds, 0) && !math.IsNaN(seconds) {
		info.Duration = time.Duration(seconds * float64(time.Second))
	}
	for _, stream := range document.Streams {
		switch stream.CodecType {
		case "video":
			ratio := preferredFrameRate(stream.AverageRate, stream.RealRate)
			rate, _ := parseRatio(ratio)
			hdr, hdrType := highDynamicRange(stream.ColorTransfer, stream.ColorPrimaries)
			info.VideoStreams = append(info.VideoStreams, media.VideoStream{
				Index:                stream.Index,
				Codec:                stream.CodecName,
				Profile:              stream.Profile,
				Width:                stream.Width,
				Height:               stream.Height,
				PixelFormat:          stream.PixelFormat,
				FrameRate:            rate,
				FrameRateRatio:       ratio,
				Rotation:             rotation(stream),
				ColorRange:           stream.ColorRange,
				ColorSpace:           stream.ColorSpace,
				ColorTransfer:        stream.ColorTransfer,
				ColorPrimaries:       stream.ColorPrimaries,
				HighDynamicRange:     hdr,
				HighDynamicRangeType: hdrType,
				Language:             stream.Tags["language"],
			})
		case "audio":
			info.AudioStreams = append(info.AudioStreams, media.AudioStream{
				Index:         stream.Index,
				Codec:         stream.CodecName,
				Profile:       stream.Profile,
				SampleRate:    int(parseInt64(stream.SampleRate)),
				Channels:      stream.Channels,
				ChannelLayout: stream.ChannelLayout,
				Language:      stream.Tags["language"],
			})
		case "subtitle":
			info.SubtitleStreams = append(info.SubtitleStreams, media.SubtitleStream{
				Index:    stream.Index,
				Codec:    stream.CodecName,
				Language: stream.Tags["language"],
				Title:    stream.Tags["title"],
			})
		}
	}
	return info, nil
}

func parseInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed
}

func preferredFrameRate(average string, real string) string {
	if rate, valid := parseRatio(average); valid && rate > 0 {
		return average
	}
	if rate, valid := parseRatio(real); valid && rate > 0 {
		return real
	}
	return ""
}

func parseRatio(value string) (float64, bool) {
	parts := strings.Split(strings.TrimSpace(value), "/")
	if len(parts) != 2 {
		return 0, false
	}
	numerator, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, false
	}
	denominator, err := strconv.ParseFloat(parts[1], 64)
	if err != nil || denominator == 0 {
		return 0, false
	}
	rate := numerator / denominator
	if math.IsInf(rate, 0) || math.IsNaN(rate) {
		return 0, false
	}
	return rate, true
}

func rotation(stream probeStream) int {
	for _, sideData := range stream.SideData {
		if sideData.Rotation != nil {
			return int(math.Round(*sideData.Rotation))
		}
	}
	if value, err := strconv.ParseFloat(strings.TrimSpace(stream.Tags["rotate"]), 64); err == nil {
		return int(math.Round(value))
	}
	return 0
}

func highDynamicRange(transfer string, primaries string) (bool, string) {
	switch strings.ToLower(strings.TrimSpace(transfer)) {
	case "smpte2084":
		return true, "pq"
	case "arib-std-b67":
		return true, "hlg"
	}
	if strings.EqualFold(strings.TrimSpace(primaries), "bt2020") {
		return true, "unknown"
	}
	return false, ""
}
