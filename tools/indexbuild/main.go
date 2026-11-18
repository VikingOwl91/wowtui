// Command indexbuild builds the signed wowtui addon index from the addons
// installed on a machine.
//
//	go run ./tools/indexbuild keygen   --out DIR
//	go run ./tools/indexbuild discover --registry registry.json --out candidates.json --cache DIR
//	go run ./tools/indexbuild build    --candidates candidates.json --key signing.key --out index.json
//	go run ./tools/indexbuild fingerprint --zip addon.zip --version TAG --out manifest.json
//
// keygen writes a new ed25519 signing key pair. discover seeds from the
// folders and TOC hints of a wowtui registry and finds each addon's GitHub
// repository through `gh api` (GET only, gh keeps the credentials; this
// program never reads a token). Every response is cached in --cache, so a
// rerun makes no repeated calls. build turns candidates.json into index.json
// and index.json.sig and verifies the pair with index.Load before it writes.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"

	"somegit.dev/vikingowl/wowtui/internal/index"
)

type deps struct {
	gh    GH
	clock Clock
}

const usageText = `usage: indexbuild <command> [flags]

commands:
  keygen    --out DIR
  discover  --registry FILE --out FILE --cache DIR [--review FILE] [--mirror-owners LIST]
  build     --candidates FILE --key FILE --out FILE
  fingerprint --zip FILE --version TAG --out FILE
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr, deps{gh: newExecGH(), clock: realClock{}}))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return 2
	}
	switch args[0] {
	case "fingerprint":
		return cmdFingerprint(args[1:], stdout, stderr)
	case "keygen":
		return cmdKeygen(args[1:], stdout, stderr)
	case "discover":
		return cmdDiscover(ctx, args[1:], stdout, stderr, d)
	case "build":
		return cmdBuild(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usageText)
		return 0
	}
	fmt.Fprintf(stderr, "indexbuild: unknown command %q\n\n%s", args[0], usageText)
	return 2
}

// parseFlags returns false after printing a usage error.
func parseFlags(fs *flag.FlagSet, args []string, stderr io.Writer, required map[string]*string) (ok bool, help bool) {
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return false, err == flag.ErrHelp
	}
	var missing []string
	for name, value := range required {
		if *value == "" {
			missing = append(missing, "--"+name)
		}
	}
	if len(missing) > 0 || fs.NArg() > 0 {
		if len(missing) > 0 {
			slices.Sort(missing)
			fmt.Fprintf(stderr, "indexbuild %s: required flag missing: %s\n", fs.Name(), strings.Join(missing, ", "))
		} else {
			fmt.Fprintf(stderr, "indexbuild %s: unexpected arguments: %s\n", fs.Name(), strings.Join(fs.Args(), " "))
		}
		fmt.Fprint(stderr, usageText)
		return false, false
	}
	return true, false
}

func cmdKeygen(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	out := fs.String("out", "", "directory for signing.key and signing.pub")
	if ok, help := parseFlags(fs, args, stderr, map[string]*string{"out": out}); !ok {
		return exitFor(help)
	}
	if err := keygen(*out); err != nil {
		fmt.Fprintf(stderr, "indexbuild keygen: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s (keep it secret) and %s\n", filepath.Join(*out, "signing.key"), filepath.Join(*out, "signing.pub"))
	return 0
}

func exitFor(help bool) int {
	if help {
		return 0
	}
	return 2
}

func cmdDiscover(ctx context.Context, args []string, stdout, stderr io.Writer, d deps) int {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	registryPath := fs.String("registry", "", "wowtui registry.json to seed from")
	out := fs.String("out", "", "candidates.json to write")
	cache := fs.String("cache", "", "directory for the response cache")
	review := fs.String("review", "", "review list to write (default: next to --out, .review.json)")
	mirrors := fs.String("mirror-owners", "hippuli", "comma separated GitHub accounts whose repositories are copies, empty for none")
	if ok, help := parseFlags(fs, args, stderr, map[string]*string{"registry": registryPath, "out": out, "cache": cache}); !ok {
		return exitFor(help)
	}
	if *review == "" {
		*review = strings.TrimSuffix(*out, ".json") + ".review.json"
	}
	reg, err := loadRegistryFile(*registryPath)
	if err != nil {
		fmt.Fprintf(stderr, "indexbuild discover: registry: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(*cache, 0o700); err != nil {
		fmt.Fprintf(stderr, "indexbuild discover: %v\n", err)
		return 1
	}
	logf := func(format string, args ...any) { fmt.Fprintf(stderr, format+"\n", args...) }
	client := NewClient(d.gh, ClientConfig{CacheDir: *cache, Clock: d.clock, Logf: logf})
	cfg := DiscoverConfig{Logf: logf}
	for _, owner := range strings.Split(*mirrors, ",") {
		if owner = strings.TrimSpace(owner); owner != "" {
			cfg.MirrorOwners = append(cfg.MirrorOwners, owner)
		}
	}
	cands, rev, err := Discover(ctx, client, reg, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "indexbuild discover: %v\n", err)
		return 1
	}
	if err := writeJSONFile(*out, cands); err != nil {
		fmt.Fprintf(stderr, "indexbuild discover: %v\n", err)
		return 1
	}
	if err := writeJSONFile(*review, rev); err != nil {
		fmt.Fprintf(stderr, "indexbuild discover: %v\n", err)
		return 1
	}
	stats := client.Stats()
	fmt.Fprintf(stdout, "%d packages, %d review items, %d of %d registry folders covered\nwrote %s and %s\ngh calls: %d live, %d from cache\n",
		len(cands.Packages), len(rev.Items), rev.Summary.CoveredFolders, rev.Summary.RegistryFolders, *out, *review, stats.Live, stats.Cached)
	return 0
}

func cmdBuild(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	candidates := fs.String("candidates", "", "candidates.json from discover")
	key := fs.String("key", "", "signing.key from keygen")
	out := fs.String("out", "", "index.json to write; the signature goes to <out>.sig")
	if ok, help := parseFlags(fs, args, stderr, map[string]*string{"candidates": candidates, "key": key, "out": out}); !ok {
		return exitFor(help)
	}
	sum, err := buildFiles(*candidates, *key, *out, index.Load)
	if err != nil {
		fmt.Fprintf(stderr, "indexbuild build: %v\n", err)
		return 1
	}
	for _, warning := range sum.Warnings {
		fmt.Fprintf(stderr, "warning: %s\n", warning)
	}
	fmt.Fprintf(stdout, "wrote %s (%d entries) and %s.sig, signature verified with index.Load\n", *out, sum.Entries, *out)
	return 0
}
