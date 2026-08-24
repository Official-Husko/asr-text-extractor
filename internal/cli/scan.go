package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Official-Husko/asr-text-extractor/pkg/asura"
)

// treeNode is one line of a scan report: a display name, and any children nested under it —
// either real subdirectories/files, or (for a recognized Asura file) synthetic nodes naming
// its own internal entries. See writeTree.
type treeNode struct {
	name     string
	children []*treeNode
}

func newScanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scan <folder> [output-file]",
		Short: "Walk a folder and list every recognized Asura file's own contents by name (no data extracted)",
		Long: `scan walks a folder recursively and writes a text tree of what it finds: real
subdirectories/files as usual, but for every file that's a recognized Asura chunk type (HTXT
text, DLLN voice, ASTS sound manifest, RSCF texture archive, or an AsuraZbb-compressed level
package), also listing the names of every entry inside it — sub-file paths, texture/mesh
paths, string-table hashes, voice command names, and so on.

No entry data is read out or written anywhere; only names. This is meant for browsing a whole
game installation's structure (e.g. to see what's actually inside its many .pc/.pc_textures/
.asr_* files) without doing a full unpack of everything, which for a real game install would
take far more time and disk space than a name-only listing needs.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			folder := args[0]
			info, err := os.Stat(folder)
			if err != nil {
				return err
			}
			if !info.IsDir() {
				return fmt.Errorf("%s: not a directory (scan takes a folder, not a single file)", folder)
			}

			base := filepath.Base(filepath.Clean(folder))
			out := strings.TrimSuffix(base, filepath.Ext(base)) + ".txt"
			if len(args) == 2 {
				out = args[1]
			}

			root := &treeNode{name: base}
			if err := scanInto(root, folder); err != nil {
				return err
			}

			f, err := os.Create(out)
			if err != nil {
				return err
			}
			err = writeTree(f, root)
			closeErr := f.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
			fmt.Fprintln(os.Stderr, "wrote", out)
			return nil
		},
	}
}

// scanInto reads dir's entries and appends one child node per entry to node: subdirectories
// recurse, files are sniffed via scanFile. A subdirectory or file that can't be read (bad
// permissions, a broken symlink, ...) gets a single inline note instead of aborting the whole
// walk — an install with hundreds of thousands of files is exactly the situation where one bad
// entry shouldn't lose everything else.
func scanInto(node *treeNode, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		full := filepath.Join(dir, e.Name())
		if e.IsDir() {
			child := &treeNode{name: e.Name()}
			node.children = append(node.children, child)
			if err := scanInto(child, full); err != nil {
				child.children = append(child.children, &treeNode{name: fmt.Sprintf("(error reading directory: %v)", err)})
			}
			continue
		}
		child, err := scanFile(full, e.Name())
		if err != nil {
			child = &treeNode{name: fmt.Sprintf("%s (error reading file: %v)", e.Name(), err)}
		}
		node.children = append(node.children, child)
	}
	return nil
}

// scanFile builds a tree node for a single file: its own name, plus, if it's a recognized
// Asura container, a child node per entry it contains. Only ever returns an error for a
// genuine I/O failure opening/reading the file — an unrecognized file, or one whose magic
// matches but whose content fails to parse, is still a valid (if childless, or
// error-annotated) node, not a scan failure.
func scanFile(path, name string) (*treeNode, error) {
	node := &treeNode{name: name}

	prefix, err := readFilePrefix(path, 12)
	if err != nil {
		return nil, err
	}

	switch {
	case asura.CheckZbbMagic(prefix):
		scanPackage(node, path)
	case asura.CheckMagic(prefix):
		scanAsuraContainer(node, path, prefix)
	}
	return node, nil
}

// readFilePrefix reads up to n bytes from the start of path — just enough to check for the
// Asura/AsuraZbb magic — without reading the rest of a potentially huge, irrelevant file
// (game installs mix in many-gigabyte video/executable files alongside the asset archives
// this tool actually understands).
func readFilePrefix(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n)
	read, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, err
	}
	return buf[:read], nil
}

// scanPackage reads and parses path as an AsuraZbb-compressed level package, listing its
// manifest sub-files, embedded textures, and embedded meshes by name. The full decompressed
// buffer (up to several hundred MB for a real level) is local to this call and goes out of
// scope once it returns, so scanning many large packages in one run stays bounded to roughly
// the size of whichever single package is currently being read, not the whole install.
func scanPackage(node *treeNode, path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		node.children = append(node.children, &treeNode{name: fmt.Sprintf("(error reading file: %v)", err)})
		return
	}
	pkg, err := asura.ParsePackage(raw)
	if err != nil {
		node.children = append(node.children, &treeNode{name: fmt.Sprintf("(AsuraZbb package, failed to parse: %v)", err)})
		return
	}

	if len(pkg.Entries) > 0 {
		sub := &treeNode{name: fmt.Sprintf("files (%d)", len(pkg.Entries))}
		for _, e := range pkg.Entries {
			sub.children = append(sub.children, &treeNode{name: e.Path})
		}
		node.children = append(node.children, sub)
	}
	if len(pkg.Textures) > 0 {
		sub := &treeNode{name: fmt.Sprintf("textures (%d)", len(pkg.Textures))}
		for _, t := range pkg.Textures {
			sub.children = append(sub.children, &treeNode{name: t.Path})
		}
		node.children = append(node.children, sub)
	}
	if len(pkg.Meshes) > 0 {
		sub := &treeNode{name: fmt.Sprintf("meshes (%d)", len(pkg.Meshes))}
		for _, m := range pkg.Meshes {
			label := m.Path
			if m.Skeleton != nil {
				label += " [skeleton: " + m.Skeleton.Name + "]"
			}
			sub.children = append(sub.children, &treeNode{name: label})
		}
		node.children = append(node.children, sub)
	}
	if len(pkg.Audio) > 0 {
		sub := &treeNode{name: fmt.Sprintf("audio (%d)", len(pkg.Audio))}
		for _, a := range pkg.Audio {
			sub.children = append(sub.children, &treeNode{name: a.Path})
		}
		node.children = append(node.children, sub)
	}
}

// scanAsuraContainer dispatches a plain (non-AsuraZbb) "Asura   "-signed file to the right
// parser by peeking at the chunk tag immediately after the 8-byte magic: HTXT/ASTS/RSCF are
// always that very first tag, but DLLN voice entries are scattered through otherwise-unknown
// binary data (see UnpackVoice), so anything that doesn't match one of the other three falls
// through to a DLLN scan instead of being assumed unrecognized outright.
func scanAsuraContainer(node *treeNode, path string, prefix []byte) {
	raw, err := os.ReadFile(path)
	if err != nil {
		node.children = append(node.children, &treeNode{name: fmt.Sprintf("(error reading file: %v)", err)})
		return
	}

	tag := ""
	if len(prefix) >= 12 {
		tag = string(prefix[8:12])
	}
	switch tag {
	case "HTXT":
		scanHTXT(node, raw)
	case "ASTS":
		scanASTS(node, raw)
	case "RSCF":
		scanRSCF(node, raw)
	default:
		scanVoice(node, raw)
	}
}

func scanHTXT(node *treeNode, raw []byte) {
	f, err := asura.ParseHTXT(raw)
	if err != nil {
		node.children = append(node.children, &treeNode{name: fmt.Sprintf("(HTXT, failed to parse: %v)", err)})
		return
	}
	sub := &treeNode{name: fmt.Sprintf("strings (%d)", len(f.Entries))}
	for i, e := range f.Entries {
		label := fmt.Sprint(e.Hash)
		if i < len(f.SymbolNames) && f.SymbolNames[i] != "" {
			label += " (" + f.SymbolNames[i] + ")"
		}
		sub.children = append(sub.children, &treeNode{name: label})
	}
	node.children = append(node.children, sub)
}

func scanASTS(node *treeNode, raw []byte) {
	f, err := asura.ParseASTS(raw)
	if err != nil {
		node.children = append(node.children, &treeNode{name: fmt.Sprintf("(ASTS, failed to parse: %v)", err)})
		return
	}
	sub := &treeNode{name: fmt.Sprintf("sounds (%d)", len(f.Entries))}
	for _, e := range f.Entries {
		sub.children = append(sub.children, &treeNode{name: e.Path})
	}
	node.children = append(node.children, sub)
}

func scanRSCF(node *treeNode, raw []byte) {
	f, err := asura.ParseRSCF(raw)
	if err != nil {
		node.children = append(node.children, &treeNode{name: fmt.Sprintf("(RSCF, failed to parse: %v)", err)})
		return
	}
	if len(f.Entries) > 0 {
		sub := &treeNode{name: fmt.Sprintf("textures (%d)", len(f.Entries))}
		for _, e := range f.Entries {
			sub.children = append(sub.children, &treeNode{name: e.Path})
		}
		node.children = append(node.children, sub)
	}
	if len(f.AudioEntries) > 0 {
		sub := &treeNode{name: fmt.Sprintf("audio (%d)", len(f.AudioEntries))}
		for _, e := range f.AudioEntries {
			sub.children = append(sub.children, &treeNode{name: e.Path})
		}
		node.children = append(node.children, sub)
	}
}

// scanVoice tries interpreting raw as a DLLN voice file. Finding zero entries isn't an error
// here — it just means this particular plain-magic Asura file isn't HTXT/ASTS/RSCF/DLLN, i.e.
// a chunk type this tool doesn't understand yet — so the file is left with no children rather
// than a misleading "(voice, failed to parse)" note.
func scanVoice(node *treeNode, raw []byte) {
	records, err := asura.UnpackVoice(raw)
	if err != nil || len(records) == 0 {
		return
	}
	sub := &treeNode{name: fmt.Sprintf("voice lines (%d)", len(records))}
	for _, r := range records {
		sub.children = append(sub.children, &treeNode{name: r.Command})
	}
	node.children = append(node.children, sub)
}

// writeTree renders root as an ASCII tree matching the classic "tree" command's own connector
// style ("├── ", "└── ", "│   ").
func writeTree(w io.Writer, root *treeNode) error {
	buf := bufio.NewWriter(w)
	if _, err := fmt.Fprintln(buf, root.name); err != nil {
		return err
	}
	if err := writeTreeChildren(buf, root.children, ""); err != nil {
		return err
	}
	return buf.Flush()
}

func writeTreeChildren(w *bufio.Writer, children []*treeNode, prefix string) error {
	for i, c := range children {
		connector, childPrefix := "├── ", prefix+"│   "
		if i == len(children)-1 {
			connector, childPrefix = "└── ", prefix+"    "
		}
		if _, err := fmt.Fprintln(w, prefix+connector+c.name); err != nil {
			return err
		}
		if err := writeTreeChildren(w, c.children, childPrefix); err != nil {
			return err
		}
	}
	return nil
}
