package remotefs

import (
	"context"
	"fmt"
)

const (
	DefaultWalkPageSize   = 100
	DefaultWalkMaxDepth   = 64
	DefaultWalkMaxEntries = 100_000
)

type WalkOptions struct {
	PageSize   int
	MaxDepth   int
	MaxEntries int
	// Filter controls whether an entry is passed to visit. It never prunes a
	// directory subtree; use Prune for traversal control.
	Filter func(entry Entry) bool
	// Prune prevents traversal below matching directories. The directory entry
	// itself is still passed to visit when Filter accepts it.
	Prune func(directory Entry) bool
}

type WalkFunc func(ctx context.Context, entry Entry) error

type walkDirectory struct {
	ref   Reference
	depth int
}

// Walk visits descendants of root in breadth-first order. It does not call
// visit for root itself. MaxDepth counts descendants, so MaxDepth=1 visits only
// direct children. Pagination cursors remain opaque to Walk; it only detects
// repeated cursors returned while listing the same directory.
func Walk(ctx context.Context, fs FileSystem, root Reference, opts WalkOptions, visit WalkFunc) error {
	if fs == nil {
		return ErrUnsupported
	}
	if visit == nil {
		return ErrUnsupported
	}
	if ctx == nil {
		return ErrNilContext
	}
	if opts.PageSize < 0 || opts.MaxDepth < 0 || opts.MaxEntries < 0 {
		return ErrInvalidOption
	}
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = DefaultWalkPageSize
	}
	maxDepth := opts.MaxDepth
	if maxDepth <= 0 {
		maxDepth = DefaultWalkMaxDepth
	}
	maxEntries := opts.MaxEntries
	if maxEntries <= 0 {
		maxEntries = DefaultWalkMaxEntries
	}

	queue := []walkDirectory{{ref: root, depth: 0}}
	visited := map[string]struct{}{referenceKey(root): {}}
	seenEntries := 0
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		current := queue[0]
		queue = queue[1:]
		cursor := ""
		seenCursors := map[string]struct{}{cursor: {}}
		for {
			page, err := fs.List(ctx, current.ref, ListOptions{Cursor: cursor, PageSize: pageSize})
			if err != nil {
				return err
			}
			for _, entry := range page.Entries {
				if err := ctx.Err(); err != nil {
					return err
				}
				seenEntries++
				if seenEntries > maxEntries {
					return fmt.Errorf("%w: %d", ErrWalkLimitExceeded, maxEntries)
				}
				if opts.Filter == nil || opts.Filter(entry) {
					if err := visit(ctx, entry); err != nil {
						return err
					}
				}
				entryDepth := current.depth + 1
				pruned := entry.IsDir() && opts.Prune != nil && opts.Prune(entry)
				if entry.IsDir() && !pruned && entryDepth < maxDepth {
					ref := entry.Reference
					key := referenceKey(ref)
					if _, exists := visited[key]; !exists {
						visited[key] = struct{}{}
						queue = append(queue, walkDirectory{ref: ref, depth: entryDepth})
					}
				}
			}
			if page.NextCursor == "" {
				break
			}
			if _, exists := seenCursors[page.NextCursor]; exists {
				return WrapError("walk", fs.Driver(), ErrProtocol, "driver returned a cyclic pagination cursor")
			}
			cursor = page.NextCursor
			seenCursors[cursor] = struct{}{}
		}
	}
	return nil
}

func referenceKey(ref Reference) string {
	if ref.ID != "" {
		return "id:" + ref.ID
	}
	return "path:" + ref.Path
}
