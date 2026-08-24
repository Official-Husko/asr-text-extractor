package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Official-Husko/asr-text-extractor/pkg/asura"
)

func newSoundCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sound",
		Short: "Work with ASTS streamsounds manifests and RSCF audio archives",
	}
	cmd.AddCommand(newSoundUnpackCmd())
	return cmd
}

func newSoundUnpackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unpack <file> [output-dir]",
		Short: "Extract every embedded WAV asset from a streamsounds (ASTS) or .sounds (RSCF) file",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			outDir := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			if len(args) == 2 {
				outDir = args[1]
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if !asura.CheckMagic(data) || len(data) < 12 {
				return fmt.Errorf("%s: %w", path, asura.ErrBadMagic)
			}

			var entries []asura.AudioEntry
			switch string(data[8:12]) {
			case "ASTS":
				f, err := asura.ParseASTS(data)
				if err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
				fmt.Fprintf(os.Stderr, "Version: %d  Entries: %d\n", f.Version, len(f.Entries))
				for _, e := range f.Entries {
					entries = append(entries, asura.AudioEntry{Path: e.Path, Data: e.Data})
				}
			case "RSCF":
				f, err := asura.ParseRSCF(data)
				if err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
				fmt.Fprintf(os.Stderr, "Entries: %d\n", len(f.AudioEntries))
				entries = f.AudioEntries
			default:
				return fmt.Errorf("%s: expected an ASTS or RSCF chunk, found %q", path, data[8:12])
			}

			for _, e := range entries {
				dest := filepath.Join(outDir, assetRelPath(e.Path))
				if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(dest, e.Data, 0o644); err != nil {
					return err
				}
				fmt.Fprintln(os.Stderr, "wrote", dest)
			}
			return nil
		},
	}
}

// assetRelPath converts a game asset path (backslash-separated, as stored in the manifest)
// into a safe, OS-appropriate relative path: separators are normalized and any "." or ".."
// component is dropped so a crafted path can't escape outDir.
func assetRelPath(assetPath string) string {
	parts := strings.Split(strings.ReplaceAll(assetPath, "\\", "/"), "/")
	clean := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			continue
		}
		clean = append(clean, p)
	}
	if len(clean) == 0 {
		return "unnamed"
	}
	return filepath.Join(clean...)
}
