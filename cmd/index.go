package cmd

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"somegit.dev/vikingowl/wowtui/internal/config"
	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/provider"
)

var errNoIndex = errors.New("no index configured; set one with: wowtui index use --path <file> --public-key <key>")

func newIndexCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Inspect and configure the signed addon index",
	}
	cmd.AddCommand(newIndexStatusCmd(), newIndexSearchCmd(), newIndexUseCmd())
	return cmd
}

func newIndexStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the configured index and whether it verifies and loads",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			if !cfg.Index.Configured() {
				fmt.Fprintln(cmd.OutOrStdout(), errNoIndex)
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			if cfg.Index.Path == "" {
				fmt.Fprintln(w, "path\tnot set")
			} else {
				fmt.Fprintf(w, "path\t%s\n", cfg.Index.Path)
				fmt.Fprintf(w, "signature\t%s\n", signaturePath(cfg.Index.Path))
			}
			switch pub, err := indexPublicKey(cfg.Index); {
			case err == nil:
				fmt.Fprintf(w, "public key\t%s\n", keyFingerprint(pub))
			case cfg.Index.PublicKey == "":
				fmt.Fprintln(w, "public key\tnot set")
			default:
				fmt.Fprintln(w, "public key\tinvalid")
			}
			idx, loadErr := loadIndex(cfg.Index)
			if loadErr != nil {
				fmt.Fprintln(w, "status\tnot loaded")
			} else {
				fmt.Fprintln(w, "status\tloaded")
				fmt.Fprintf(w, "entries\t%d\n", len(idx.Entries()))
				fmt.Fprintf(w, "generated\t%s\n", idx.GeneratedAt().UTC().Format(time.RFC3339))
			}
			if err := w.Flush(); err != nil {
				return err
			}
			return loadErr
		},
	}
}

func newIndexSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search index entries by id or name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			if !cfg.Index.Configured() {
				return errNoIndex
			}
			idx, err := loadIndex(cfg.Index)
			if err != nil {
				return err
			}
			entries := idx.Search(args[0])
			if len(entries) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "no index entries match %q\n", args[0])
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tSOURCE\tFOLDERS\tGAME TYPES\tREVIEWED\tFUNDING")
			for _, entry := range entries {
				source := entry.Source
				if ref, err := provider.ParseReference(entry.Source); err == nil {
					source = provider.FormatReference(ref)
				}
				gameTypes := make([]string, 0, len(entry.GameTypes))
				for _, gameType := range entry.GameTypes {
					gameTypes = append(gameTypes, string(gameType))
				}
				links := make([]string, 0, len(entry.Funding))
				for _, funding := range entry.Funding {
					link, err := funding.URL()
					if err != nil {
						return fmt.Errorf("index entry %s: %w", entry.ID, err)
					}
					links = append(links, link)
				}
				reviewed := "no"
				if entry.Reviewed {
					reviewed = "yes"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", entry.ID, entry.Name, source,
					strings.Join(entry.Folders, ","), strings.Join(gameTypes, ","), reviewed, orDash(strings.Join(links, ",")))
			}
			return w.Flush()
		},
	}
}

func newIndexUseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use --path <file> --public-key <key>",
		Short: "Verify a signed index file and save it in the config",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, _ := cmd.Flags().GetString("path")
			key, _ := cmd.Flags().GetString("public-key")
			abs, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			pub, err := index.DecodePublicKey(key)
			if err != nil {
				return fmt.Errorf("index not saved: %w", err)
			}
			settings := config.Index{Path: abs, PublicKey: index.EncodePublicKey(pub)}
			idx, err := loadIndex(settings)
			if err != nil {
				return fmt.Errorf("index not saved: %w", err)
			}
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			cfg.Index = settings
			if err := config.Save(configPath(cmd), cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "index set: %s (%d entries, public key %s)\n", abs, len(idx.Entries()), keyFingerprint(pub))
			return nil
		},
	}
	cmd.Flags().String("path", "", "index JSON file; its signature is the same path with .sig appended")
	cmd.Flags().String("public-key", "", "base64 ed25519 public key that signs the index")
	cmd.MarkFlagRequired("path")
	cmd.MarkFlagRequired("public-key")
	return cmd
}

func signaturePath(path string) string {
	return path + ".sig"
}

func indexPublicKey(settings config.Index) (ed25519.PublicKey, error) {
	if settings.PublicKey == "" {
		return nil, errors.New("index.public_key is not set")
	}
	return index.DecodePublicKey(settings.PublicKey)
}

// loadIndex reads the configured index and its signature and returns it only
// after index.Load verified and validated it.
func loadIndex(settings config.Index) (*index.Index, error) {
	if settings.Path == "" {
		return nil, errors.New("index.path is not set")
	}
	pub, err := indexPublicKey(settings)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(settings.Path)
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}
	sig, err := os.ReadFile(signaturePath(settings.Path))
	if err != nil {
		return nil, fmt.Errorf("read index signature: %w", err)
	}
	return index.Load(data, sig, pub)
}

// keyFingerprint returns the first 16 hex characters of the SHA-256 of the
// raw public key bytes.
func keyFingerprint(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return "sha256:" + hex.EncodeToString(sum[:])[:16]
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
