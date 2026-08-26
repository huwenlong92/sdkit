package ffprobe_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/pkg/media"
	"github.com/huwenlong92/sdkit/pkg/media/ffprobe"
)

func TestFFProbeIntegration(t *testing.T) {
	if os.Getenv("SDKIT_FFPROBE_INTEGRATION") != "1" {
		t.Skip("set SDKIT_FFPROBE_INTEGRATION=1 to run the local ffprobe integration test")
	}
	input := os.Getenv("SDKIT_FFPROBE_INPUT")
	if input == "" {
		t.Skip("set SDKIT_FFPROBE_INPUT to a local media file")
	}
	binary, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	driver, err := ffprobe.New(ctx, ffprobe.Config{BinaryPath: binary})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	info, err := driver.Probe(ctx, media.Input{Path: input}, media.ProbeOptions{})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if info.Container == "" && len(info.VideoStreams)+len(info.AudioStreams)+len(info.SubtitleStreams) == 0 {
		t.Fatalf("Probe() returned empty metadata: %+v", info)
	}
}
