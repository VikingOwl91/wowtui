package provider

import (
	"context"
	"io"

	"somegit.dev/vikingowl/wowtui/internal/wow"
)

type RemoteAddon struct {
	Provider string
	ID       string
	Name     string
	URL      string
}

type ReleaseKind string

const (
	KindRelease ReleaseKind = "release"
	KindTag     ReleaseKind = "tag"
	KindBranch  ReleaseKind = "branch"
)

type Release struct {
	Kind        ReleaseKind
	ID          int64
	Tag         string
	Commit      string
	AssetName   string
	DownloadURL string
	Size        int64
}

type Provider interface {
	Resolve(ctx context.Context, ref Reference) (RemoteAddon, error)
	Latest(ctx context.Context, addon RemoteAddon, flavor wow.GameFlavor, source string) (Release, error)
	Download(ctx context.Context, release Release) (io.ReadCloser, error)
}
