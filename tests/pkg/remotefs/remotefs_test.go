package remotefs_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/huwenlong92/sdkit/pkg/remotefs"
)

type fakeFileSystem struct {
	list func(context.Context, remotefs.Reference, remotefs.ListOptions) (remotefs.ListPage, error)
}

func (f *fakeFileSystem) Driver() string { return "fake" }

func (f *fakeFileSystem) Stat(context.Context, remotefs.Reference) (remotefs.Entry, error) {
	return remotefs.Entry{}, remotefs.ErrNotFound
}

func (f *fakeFileSystem) List(ctx context.Context, ref remotefs.Reference, opts remotefs.ListOptions) (remotefs.ListPage, error) {
	if f.list == nil {
		return remotefs.ListPage{}, nil
	}
	return f.list(ctx, ref, opts)
}

func (f *fakeFileSystem) Download(context.Context, remotefs.DownloadRequest, remotefs.ProgressSink) (remotefs.DownloadResult, error) {
	return remotefs.DownloadResult{}, remotefs.ErrUnsupported
}

func (f *fakeFileSystem) Close() error { return nil }

func TestErrorSupportsErrorsIsAndKeepsSafeSummary(t *testing.T) {
	err := &remotefs.Error{
		Operation: "stat",
		Driver:    "fake",
		Err:       remotefs.ErrNotFound,
		Summary:   "remote path was not found",
	}
	if !errors.Is(err, remotefs.ErrNotFound) {
		t.Fatalf("errors.Is(%v, ErrNotFound) = false", err)
	}
	var typed *remotefs.Error
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As(%v, *Error) = false", err)
	}
	if got := err.Error(); strings.Contains(got, "secret-token") || !strings.Contains(got, "remote path was not found") {
		t.Fatalf("unexpected safe error text %q", got)
	}
}

func TestSafeSummaryPreservesUTF8AndRemovesTerminalControls(t *testing.T) {
	value := "\x1b[31m" + strings.Repeat("界", 300) + "\x1b[0m\x00\nsecret"
	summary := remotefs.SafeSummary(value)
	if !utf8.ValidString(summary) {
		t.Fatalf("summary is not valid UTF-8: %q", summary)
	}
	if strings.ContainsRune(summary, '\x1b') || strings.ContainsRune(summary, '\x00') {
		t.Fatalf("summary retained terminal controls: %q", summary)
	}
	if len(summary) > 515 {
		t.Fatalf("summary length = %d, want at most 515 bytes", len(summary))
	}
}

func TestWalkPaginatesAndHonorsFilterAndLimits(t *testing.T) {
	fs := &fakeFileSystem{}
	fs.list = func(ctx context.Context, dir remotefs.Reference, opts remotefs.ListOptions) (remotefs.ListPage, error) {
		if err := ctx.Err(); err != nil {
			return remotefs.ListPage{}, err
		}
		switch dir.Path + ":" + opts.Cursor {
		case "/:":
			return remotefs.ListPage{Entries: []remotefs.Entry{
				{Name: "a.txt", Reference: remotefs.Reference{Path: "/a.txt"}, Type: remotefs.EntryFile},
				{Name: "nested", Reference: remotefs.Reference{Path: "/nested"}, Type: remotefs.EntryDirectory},
			}, NextCursor: "2"}, nil
		case "/:2":
			return remotefs.ListPage{Entries: []remotefs.Entry{
				{Name: "skip.log", Reference: remotefs.Reference{Path: "/skip.log"}, Type: remotefs.EntryFile},
			}}, nil
		case "/nested:":
			return remotefs.ListPage{Entries: []remotefs.Entry{
				{Name: "b.txt", Reference: remotefs.Reference{Path: "/nested/b.txt"}, Type: remotefs.EntryFile},
			}}, nil
		default:
			return remotefs.ListPage{}, errors.New("unexpected list request")
		}
	}

	var paths []string
	err := remotefs.Walk(context.Background(), fs, remotefs.Reference{Path: "/"}, remotefs.WalkOptions{
		PageSize:   2,
		MaxDepth:   2,
		MaxEntries: 4,
		Filter: func(entry remotefs.Entry) bool {
			return !strings.HasSuffix(entry.Name, ".log")
		},
	}, func(_ context.Context, entry remotefs.Entry) error {
		paths = append(paths, entry.Reference.Path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	want := []string{"/a.txt", "/nested", "/nested/b.txt"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
}

func TestWalkStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fs := &fakeFileSystem{list: func(ctx context.Context, _ remotefs.Reference, _ remotefs.ListOptions) (remotefs.ListPage, error) {
		cancel()
		<-ctx.Done()
		return remotefs.ListPage{}, ctx.Err()
	}}
	err := remotefs.Walk(ctx, fs, remotefs.Reference{Path: "/"}, remotefs.WalkOptions{}, func(context.Context, remotefs.Entry) error {
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("walk error = %v, want context.Canceled", err)
	}
}

func TestWalkRejectsNilContextAndCursorCycles(t *testing.T) {
	fs := &fakeFileSystem{}
	if err := remotefs.Walk(nil, fs, remotefs.Reference{Path: "/"}, remotefs.WalkOptions{}, func(context.Context, remotefs.Entry) error {
		return nil
	}); !errors.Is(err, remotefs.ErrNilContext) {
		t.Fatalf("nil context error = %v", err)
	}

	fs.list = func(_ context.Context, _ remotefs.Reference, opts remotefs.ListOptions) (remotefs.ListPage, error) {
		switch opts.Cursor {
		case "":
			return remotefs.ListPage{NextCursor: "A"}, nil
		case "A":
			return remotefs.ListPage{NextCursor: "B"}, nil
		case "B":
			return remotefs.ListPage{NextCursor: "A"}, nil
		default:
			return remotefs.ListPage{}, errors.New("unexpected cursor")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := remotefs.Walk(ctx, fs, remotefs.Reference{Path: "/"}, remotefs.WalkOptions{}, func(context.Context, remotefs.Entry) error {
		return nil
	})
	if !errors.Is(err, remotefs.ErrProtocol) {
		t.Fatalf("cursor cycle error = %v", err)
	}
}

func TestWalkDoesNotVisitEntriesBeyondMaximumDepth(t *testing.T) {
	fs := &fakeFileSystem{list: func(_ context.Context, dir remotefs.Reference, _ remotefs.ListOptions) (remotefs.ListPage, error) {
		switch dir.Path {
		case "/":
			return remotefs.ListPage{Entries: []remotefs.Entry{{Name: "one", Reference: remotefs.Reference{Path: "/one"}, Type: remotefs.EntryDirectory}}}, nil
		case "/one":
			return remotefs.ListPage{Entries: []remotefs.Entry{{Name: "two.txt", Reference: remotefs.Reference{Path: "/one/two.txt"}, Type: remotefs.EntryFile}}}, nil
		default:
			return remotefs.ListPage{}, errors.New("walk exceeded maximum depth")
		}
	}}
	var paths []string
	err := remotefs.Walk(context.Background(), fs, remotefs.Reference{Path: "/"}, remotefs.WalkOptions{MaxDepth: 1}, func(_ context.Context, entry remotefs.Entry) error {
		paths = append(paths, entry.Reference.Path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if strings.Join(paths, ",") != "/one" {
		t.Fatalf("paths = %v, want only depth-one entry", paths)
	}
}

func TestWalkFilterDoesNotPruneButPruneStopsDirectoryTraversal(t *testing.T) {
	listCalls := make(map[string]int)
	fs := &fakeFileSystem{list: func(_ context.Context, dir remotefs.Reference, _ remotefs.ListOptions) (remotefs.ListPage, error) {
		listCalls[dir.Path]++
		switch dir.Path {
		case "/":
			return remotefs.ListPage{Entries: []remotefs.Entry{
				{Name: "filtered", Reference: remotefs.Reference{ID: "dir-filtered", Path: "/filtered"}, Type: remotefs.EntryDirectory},
				{Name: "pruned", Reference: remotefs.Reference{ID: "dir-pruned", Path: "/pruned"}, Type: remotefs.EntryDirectory},
			}}, nil
		case "/filtered":
			return remotefs.ListPage{Entries: []remotefs.Entry{{Name: "kept.txt", Reference: remotefs.Reference{Path: "/filtered/kept.txt"}, Type: remotefs.EntryFile}}}, nil
		case "/pruned":
			return remotefs.ListPage{}, errors.New("pruned directory was listed")
		default:
			return remotefs.ListPage{}, nil
		}
	}}
	var paths []string
	err := remotefs.Walk(context.Background(), fs, remotefs.Reference{Path: "/"}, remotefs.WalkOptions{
		Filter: func(entry remotefs.Entry) bool { return entry.Name != "filtered" },
		Prune:  func(entry remotefs.Entry) bool { return entry.Name == "pruned" },
	}, func(_ context.Context, entry remotefs.Entry) error {
		paths = append(paths, entry.Reference.Path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(paths, ",") != "/pruned,/filtered/kept.txt" {
		t.Fatalf("visited paths = %v", paths)
	}
	if listCalls["/filtered"] != 1 || listCalls["/pruned"] != 0 {
		t.Fatalf("list calls = %v", listCalls)
	}
}

func TestWalkStopsDirectoryIdentityCycles(t *testing.T) {
	listCalls := make(map[string]int)
	fs := &fakeFileSystem{list: func(_ context.Context, dir remotefs.Reference, _ remotefs.ListOptions) (remotefs.ListPage, error) {
		listCalls[dir.Path]++
		switch dir.Path {
		case "/":
			return remotefs.ListPage{Entries: []remotefs.Entry{{
				Name: "a", Reference: remotefs.Reference{ID: "shared-directory", Path: "/a"}, Type: remotefs.EntryDirectory,
			}}}, nil
		case "/a":
			return remotefs.ListPage{Entries: []remotefs.Entry{{
				Name: "back-to-a", Reference: remotefs.Reference{ID: "shared-directory", Path: "/a/loop"}, Type: remotefs.EntryDirectory,
			}}}, nil
		default:
			return remotefs.ListPage{}, errors.New("directory identity cycle was traversed")
		}
	}}
	if err := remotefs.Walk(context.Background(), fs, remotefs.Reference{Path: "/"}, remotefs.WalkOptions{}, func(context.Context, remotefs.Entry) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if listCalls["/"] != 1 || listCalls["/a"] != 1 || len(listCalls) != 2 {
		t.Fatalf("list calls = %v", listCalls)
	}
}

func TestWalkStopsAtMaximumEntryCount(t *testing.T) {
	fs := &fakeFileSystem{list: func(_ context.Context, _ remotefs.Reference, _ remotefs.ListOptions) (remotefs.ListPage, error) {
		return remotefs.ListPage{Entries: []remotefs.Entry{
			{Name: "one", Reference: remotefs.Reference{Path: "/one"}, Type: remotefs.EntryFile},
			{Name: "two", Reference: remotefs.Reference{Path: "/two"}, Type: remotefs.EntryFile},
		}}, nil
	}}
	visited := 0
	err := remotefs.Walk(context.Background(), fs, remotefs.Reference{Path: "/"}, remotefs.WalkOptions{MaxEntries: 1}, func(context.Context, remotefs.Entry) error {
		visited++
		return nil
	})
	if !errors.Is(err, remotefs.ErrWalkLimitExceeded) {
		t.Fatalf("Walk() error = %v, want ErrWalkLimitExceeded", err)
	}
	if visited != 1 {
		t.Fatalf("visited entries = %d, want 1", visited)
	}
}

func TestProgressSinkErrorIsReturned(t *testing.T) {
	want := errors.New("stop progress")
	var calls atomic.Int32
	err := remotefs.EmitProgress(context.Background(), remotefs.ProgressSinkFunc(func(context.Context, remotefs.Progress) error {
		calls.Add(1)
		return want
	}), remotefs.Progress{Phase: remotefs.ProgressTransferring, TransferredBytes: 1})
	if !errors.Is(err, want) {
		t.Fatalf("EmitProgress error = %v, want %v", err, want)
	}
	if calls.Load() != 1 {
		t.Fatalf("progress calls = %d, want 1", calls.Load())
	}
}

func TestProgressRejectsInvalidValuesAndNilContext(t *testing.T) {
	invalid := []remotefs.Progress{
		{Phase: remotefs.ProgressTransferring, TransferredBytes: -1},
		{Phase: remotefs.ProgressTransferring, TransferredBytes: 2, TotalBytes: 1},
		{Phase: remotefs.ProgressTransferring, BytesPerSecond: -1},
		{Phase: "unknown"},
	}
	for _, progress := range invalid {
		if err := remotefs.EmitProgress(context.Background(), nil, progress); !errors.Is(err, remotefs.ErrInvalidArgument) {
			t.Fatalf("EmitProgress(%+v) error = %v", progress, err)
		}
	}
	if err := remotefs.EmitProgress(nil, nil, remotefs.Progress{Phase: remotefs.ProgressPreparing}); !errors.Is(err, remotefs.ErrNilContext) {
		t.Fatalf("nil context error = %v", err)
	}
}
