package sync

import "github.com/r-dson/abox/internal/exclusion"

// Options controls sync-out archive extraction behavior.
type Options struct {
	Matcher             *exclusion.Matcher
	RootName            string
	DeleteMissing       bool
	AllowUnsafeSymlinks bool
	Snapshot            *RootSnapshot
	ForceSync           bool
}
