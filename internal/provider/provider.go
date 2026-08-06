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

type Release struct {
	ID          int64
	Tag         string
	AssetName   string
	DownloadURL string
	Size        int64
}

type Provider interface {
	Resolve(ctx context.Context, ref Reference) (RemoteAddon, error)
	LatestRelease(ctx context.Context, addon RemoteAddon, flavor wow.GameFlavor) (Release, error)
	Download(ctx context.Context, release Release) (io.ReadCloser, error)
}
