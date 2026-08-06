package provider

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"somegit.dev/vikingowl/wowtui/internal/wow"
)

type Git struct{}

func NewGit() *Git { return &Git{} }

func (g *Git) Resolve(ctx context.Context, ref Reference) (RemoteAddon, error) {
	if _, err := g.run(ctx, "ls-remote", "--heads", ref.ID); err != nil {
		return RemoteAddon{}, fmt.Errorf("git repository unavailable: %w", err)
	}
	return RemoteAddon{Provider: "git", ID: ref.ID, Name: gitRepoName(ref.ID), URL: ref.ID}, nil
}

func (g *Git) Latest(ctx context.Context, addon RemoteAddon, _ wow.GameFlavor, source string) (Release, error) {
	if source == "release" {
		return Release{}, fmt.Errorf("generic git sources do not provide releases")
	}
	if source == "" || source == "auto" || source == "tag" {
		if tags, err := g.tags(ctx, addon.ID); err == nil && len(tags) > 0 {
			tag := pickTag(tags)
			return Release{Kind: KindTag, Tag: tag, AssetName: gitRepoName(addon.ID) + "-" + tag + ".zip", DownloadURL: addon.ID}, nil
		} else if source == "tag" {
			return Release{}, fmt.Errorf("no git tags found")
		}
	}
	branch, commit, err := g.defaultBranch(ctx, addon.ID)
	if err != nil {
		return Release{}, err
	}
	return Release{Kind: KindBranch, Tag: branch, Commit: commit, AssetName: gitRepoName(addon.ID) + "-" + branch + ".zip", DownloadURL: addon.ID}, nil
}

func (g *Git) Download(ctx context.Context, release Release) (io.ReadCloser, error) {
	workDir, err := os.MkdirTemp("", "wowtui-git-")
	if err != nil {
		return nil, err
	}
	checkout := filepath.Join(workDir, "repo")
	ref := release.Tag
	if ref == "" {
		ref = "HEAD"
	}
	if _, err := g.run(ctx, "clone", "--depth", "1", "--branch", ref, release.DownloadURL, checkout); err != nil {
		os.RemoveAll(workDir)
		return nil, fmt.Errorf("git clone failed: %w", err)
	}
	archivePath := filepath.Join(workDir, "addon.zip")
	if err := zipDirectory(checkout, archivePath, gitRepoName(release.DownloadURL)+"-"+ref); err != nil {
		os.RemoveAll(workDir)
		return nil, err
	}
	file, err := os.Open(archivePath)
	if err != nil {
		os.RemoveAll(workDir)
		return nil, err
	}
	return &cleanupReader{File: file, cleanup: func() { os.RemoveAll(workDir) }}, nil
}

func (g *Git) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func (g *Git) tags(ctx context.Context, repo string) ([]string, error) {
	output, err := g.run(ctx, "ls-remote", "--tags", "--refs", repo)
	if err != nil {
		return nil, err
	}
	var tags []string
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.HasPrefix(fields[1], "refs/tags/") {
			tags = append(tags, strings.TrimPrefix(fields[1], "refs/tags/"))
		}
	}
	return tags, nil
}

func (g *Git) defaultBranch(ctx context.Context, repo string) (string, string, error) {
	output, err := g.run(ctx, "ls-remote", "--symref", repo, "HEAD")
	if err != nil {
		return "", "", err
	}
	branch := "main"
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "ref:" && strings.HasPrefix(fields[1], "refs/heads/") {
			branch = strings.TrimPrefix(fields[1], "refs/heads/")
		}
	}
	output, err = g.run(ctx, "ls-remote", repo, "refs/heads/"+branch)
	if err != nil {
		return "", "", err
	}
	fields := strings.Fields(string(output))
	if len(fields) == 0 {
		return "", "", fmt.Errorf("could not resolve git branch %s", branch)
	}
	return branch, fields[0], nil
}

func gitRepoName(raw string) string {
	base := filepath.Base(strings.TrimSuffix(raw, ".git"))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "git-addon"
	}
	return base
}

func zipDirectory(root, output, prefix string) error {
	file, err := os.Create(output)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := zip.NewWriter(file)
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." || strings.HasPrefix(rel, ".git") {
			return err
		}
		if info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Join(prefix, rel))
		entry, err := writer.Create(name)
		if err != nil {
			return err
		}
		_, err = entry.Write(data)
		return err
	})
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	return err
}

type cleanupReader struct {
	*os.File
	cleanup func()
}

func (r *cleanupReader) Close() error {
	err := r.File.Close()
	r.cleanup()
	return err
}
