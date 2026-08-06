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

type MatchHint struct {
	FolderName string
	Title      string
	Website    string
	SourceRepo string
	Wago       string
	Curse      string
}

type SearchProvider interface {
	Provider
	Search(ctx context.Context, query string, flavor wow.GameFlavor) ([]RemoteAddon, error)
}

type MatchProvider interface {
	Provider
	Match(ctx context.Context, hint MatchHint, flavor wow.GameFlavor) (RemoteAddon, bool, error)
}

type Authenticator interface {
	Authenticate(ctx context.Context) error
}

type ReleaseKind string

const (
	KindRelease ReleaseKind = "release"
	KindTag     ReleaseKind = "tag"
	KindBranch  ReleaseKind = "branch"
	KindDirect  ReleaseKind = "direct"
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
