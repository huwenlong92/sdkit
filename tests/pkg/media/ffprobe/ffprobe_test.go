package ffprobe_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/pkg/execx"
	"github.com/huwenlong92/sdkit/pkg/media"
	"github.com/huwenlong92/sdkit/pkg/media/ffprobe"
)

type ffprobeRunnerFunc func(context.Context, ffprobe.Command) (ffprobe.Result, error)

func (fn ffprobeRunnerFunc) RunOutput(ctx context.Context, command ffprobe.Command) (ffprobe.Result, error) {
	return fn(ctx, command)
}

func TestFFProbeNormalizesStreams(t *testing.T) {
	input := mediaInput(t)
	fixture := mediaFixture(t, "complex.json")
	var probeCommand ffprobe.Command
	runner := ffprobeRunnerFunc(func(_ context.Context, command ffprobe.Command) (ffprobe.Result, error) {
		if len(command.Args) == 1 && command.Args[0] == "-version" {
			return ffprobe.Result{Stdout: []byte("ffprobe version 8.0.1 Copyright\n")}, nil
		}
		probeCommand = command
		return ffprobe.Result{Stdout: fixture}, nil
	})
	driver, err := ffprobe.New(context.Background(), ffprobe.Config{BinaryPath: "/opt/ffprobe", MinVersion: "7.0", MaxVersion: "9.0"}, ffprobe.WithRunner(runner))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	info, err := driver.Probe(context.Background(), media.Input{Path: input}, media.ProbeOptions{})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if driver.Version() != "8.0.1" {
		t.Fatalf("Version() = %q", driver.Version())
	}
	if info.Container != "mov,mp4,m4a,3gp,3g2,mj2" || info.Duration != 12*time.Second+345*time.Millisecond || info.SizeBytes != 12345678 || info.BitRate != 8000000 {
		t.Fatalf("unexpected format info: %+v", info)
	}
	if len(info.VideoStreams) != 2 || len(info.AudioStreams) != 2 || len(info.SubtitleStreams) != 1 {
		t.Fatalf("unexpected stream counts: %+v", info)
	}
	video := info.VideoStreams[0]
	if video.Codec != "hevc" || video.Width != 3840 || video.Height != 2160 || video.FrameRateRatio != "24000/1001" || video.FrameRate < 23.97 || video.FrameRate > 23.98 || video.Rotation != -90 {
		t.Fatalf("unexpected video stream: %+v", video)
	}
	if !video.HighDynamicRange || video.HighDynamicRangeType != "pq" {
		t.Fatalf("unexpected HDR metadata: %+v", video)
	}
	if info.VideoStreams[1].FrameRate != 0 || info.VideoStreams[1].FrameRateRatio != "" {
		t.Fatalf("invalid frame rate should normalize to zero: %+v", info.VideoStreams[1])
	}
	if info.AudioStreams[1].Language != "zho" || info.AudioStreams[1].Channels != 6 {
		t.Fatalf("unexpected second audio stream: %+v", info.AudioStreams[1])
	}
	if info.SubtitleStreams[0].Title != "简体中文" {
		t.Fatalf("unexpected subtitle stream: %+v", info.SubtitleStreams[0])
	}
	if probeCommand.Name != "/opt/ffprobe" || !containsSequence(probeCommand.Args, "-print_format", "json") || !containsSequence(probeCommand.Args, "-i", input) {
		t.Fatalf("unexpected probe command: %+v", probeCommand)
	}
}

func TestFFProbeAcceptsAudioOnlyInput(t *testing.T) {
	driver := newFixtureProber(t, mediaFixture(t, "audio_only.json"))
	info, err := driver.Probe(context.Background(), media.Input{Path: mediaInput(t)}, media.ProbeOptions{})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if len(info.VideoStreams) != 0 || len(info.AudioStreams) != 1 || info.AudioStreams[0].Codec != "flac" {
		t.Fatalf("unexpected audio-only result: %+v", info)
	}
}

func TestFFProbeRejectsNonLocalAndUnavailableInputs(t *testing.T) {
	driver := newFixtureProber(t, mediaFixture(t, "audio_only.json"))
	_, err := driver.Probe(context.Background(), media.Input{Path: "https://example.test/video.mp4"}, media.ProbeOptions{})
	if !errors.Is(err, media.ErrInvalidInput) {
		t.Fatalf("remote input error = %v", err)
	}
	_, err = driver.Probe(context.Background(), media.Input{Path: filepath.Join(t.TempDir(), "missing.mp4")}, media.ProbeOptions{})
	if !errors.Is(err, media.ErrInputUnavailable) {
		t.Fatalf("missing input error = %v", err)
	}
}

func TestFFProbeHonorsAlreadyCanceledContext(t *testing.T) {
	driver := newFixtureProber(t, mediaFixture(t, "audio_only.json"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := driver.Probe(ctx, media.Input{Path: mediaInput(t)}, media.ProbeOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled probe error = %v", err)
	}
}

func TestFFProbeClassifiesRunnerFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want error
	}{
		{name: "output limit", err: execx.ErrOutputLimitExceeded, want: media.ErrOutputLimit},
		{name: "probe failed", err: errors.New("secret path /private/video.mp4"), want: media.ErrProbeFailed},
		{name: "canceled", err: context.Canceled, want: context.Canceled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := mediaInput(t)
			runner := versionThen(ffprobeRunnerFunc(func(_ context.Context, _ ffprobe.Command) (ffprobe.Result, error) {
				return ffprobe.Result{}, test.err
			}))
			driver, err := ffprobe.New(context.Background(), ffprobe.Config{}, ffprobe.WithRunner(runner))
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			_, err = driver.Probe(context.Background(), media.Input{Path: input}, media.ProbeOptions{})
			if !errors.Is(err, test.want) {
				t.Fatalf("Probe() error = %v, want %v", err, test.want)
			}
			if strings.Contains(err.Error(), "/private/video.mp4") {
				t.Fatalf("error exposes runner details: %v", err)
			}
		})
	}
}

func TestFFProbeClassifiesBinaryAndVersionErrors(t *testing.T) {
	_, err := ffprobe.New(context.Background(), ffprobe.Config{}, ffprobe.WithRunner(ffprobeRunnerFunc(func(_ context.Context, _ ffprobe.Command) (ffprobe.Result, error) {
		return ffprobe.Result{}, exec.ErrNotFound
	})))
	if !errors.Is(err, media.ErrBinaryUnavailable) {
		t.Fatalf("missing binary error = %v", err)
	}
	_, err = ffprobe.New(context.Background(), ffprobe.Config{MinVersion: "9.0"}, ffprobe.WithRunner(ffprobeRunnerFunc(func(_ context.Context, _ ffprobe.Command) (ffprobe.Result, error) {
		return ffprobe.Result{Stdout: []byte("ffprobe version 8.0.1 Copyright\n")}, nil
	})))
	if !errors.Is(err, media.ErrVersionUnsupported) {
		t.Fatalf("unsupported version error = %v", err)
	}
}

func TestFFProbeAcceptsSnapshotVersionPrefix(t *testing.T) {
	driver, err := ffprobe.New(context.Background(), ffprobe.Config{
		MinVersion: "9.0.1",
		MaxVersion: "9.0.1",
	}, ffprobe.WithRunner(ffprobeRunnerFunc(func(_ context.Context, _ ffprobe.Command) (ffprobe.Result, error) {
		return ffprobe.Result{Stdout: []byte("ffprobe version n9.0.1-6-g9d4ca21220-20260825 Copyright\n")}, nil
	})))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if driver.Version() != "n9.0.1-6-g9d4ca21220-20260825" {
		t.Fatalf("Version() = %q", driver.Version())
	}
}

func TestFFProbeRejectsInvalidOutput(t *testing.T) {
	driver := newFixtureProber(t, []byte("not-json"))
	_, err := driver.Probe(context.Background(), media.Input{Path: mediaInput(t)}, media.ProbeOptions{})
	if !errors.Is(err, media.ErrOutputInvalid) {
		t.Fatalf("invalid output error = %v", err)
	}
	var typed *media.Error
	if !errors.As(err, &typed) {
		t.Fatalf("invalid output should use *media.Error: %T", err)
	}
}

func TestFFProbeUsesPerCallOutputLimit(t *testing.T) {
	input := mediaInput(t)
	var got int64
	runner := versionThen(ffprobeRunnerFunc(func(_ context.Context, command ffprobe.Command) (ffprobe.Result, error) {
		got = command.OutputLimit
		return ffprobe.Result{Stdout: mediaFixture(t, "audio_only.json")}, nil
	}))
	driver, err := ffprobe.New(context.Background(), ffprobe.Config{OutputLimit: 1024}, ffprobe.WithRunner(runner))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = driver.Probe(context.Background(), media.Input{Path: input}, media.ProbeOptions{OutputLimit: 2048})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if got != 2048 {
		t.Fatalf("output limit = %d", got)
	}
}

func newFixtureProber(t *testing.T, fixture []byte) *ffprobe.Driver {
	t.Helper()
	runner := versionThen(ffprobeRunnerFunc(func(_ context.Context, _ ffprobe.Command) (ffprobe.Result, error) {
		return ffprobe.Result{Stdout: fixture}, nil
	}))
	driver, err := ffprobe.New(context.Background(), ffprobe.Config{}, ffprobe.WithRunner(runner))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return driver
}

func versionThen(next ffprobeRunnerFunc) ffprobeRunnerFunc {
	return func(ctx context.Context, command ffprobe.Command) (ffprobe.Result, error) {
		if len(command.Args) == 1 && command.Args[0] == "-version" {
			return ffprobe.Result{Stdout: []byte("ffprobe version 8.0.1 Copyright\n")}, nil
		}
		return next(ctx, command)
	}
}

func mediaInput(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.mp4")
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	return path
}

func mediaFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

func containsSequence(values []string, first string, second string) bool {
	for i := 0; i+1 < len(values); i++ {
		if values[i] == first && values[i+1] == second {
			return true
		}
	}
	return false
}
