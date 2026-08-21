package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// defaultCSVPath mirrors the original tool's Path.GetFileNameWithoutExtension(path) + ".csv".
func defaultCSVPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base)) + ".csv"
}

// backupPath returns path with "_back" appended, matching the original tool's in-place
// override backup naming.
func backupPath(path string) string {
	return path + "_back"
}

// backupIfInPlace copies src to its backup path when no explicit output path was given
// (i.e. the override would otherwise overwrite src in place), unless a backup already
// exists. Matches the original tool's auto-backup behavior.
func backupIfInPlace(src string, explicitOutput bool) error {
	if explicitOutput {
		return nil
	}
	dst := backupPath(src)
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "backupFile:", dst)
	return nil
}
