package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

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

// scanner bounds how many CPU-bound decompress/parse jobs (see scanPackage, scanAsuraContainer)
// run at once, across the *whole* recursive directory walk — not one pool per directory, which
// would let sibling subdirectories oversubscribe available cores by each spinning up their own
// full-sized pool. Sized to GOMAXPROCS: a real CPU profile of a game-install scan showed over
// 90% of total CPU time inside zlib/flate decompressing AsuraZbb-wrapped packages
// (`pkg/asura.DecompressZbb`), a fully CPU-bound, per-file-independent cost that benefits
// directly from running on every available core instead of one at a time.
//
// **Deliberately does not parallelize the file *reads* themselves** — an earlier version of
// this change did (dispatching each file's `os.ReadFile` onto the pool alongside its
// decompression), and measured almost no real speedup on a real 20GB install (`user` CPU time
// came out within ~7% of wall-clock time — the signature of a genuinely I/O-bound run, not a
// parallel CPU-bound one) despite the same change showing an exactly-as-predicted ~1.7x speedup
// on a smaller, already-page-cached folder used to validate the parallel logic itself. The real
// game install here lives on a rotational hard disk (`lsblk` confirms it), not an SSD — spinning
// media has essentially one read head, so several goroutines all issuing large, effectively
// random-offset reads at once (mixing prefix peeks with 100s-of-MB whole-file reads across many
// different files) causes seek thrashing that can erase or even reverse a parallel CPU win, not
// just fail to help. So `scanInto` keeps every file's `os.ReadFile` sequential — matching, as
// closely as directory-listing order allows, the single-stream access pattern spinning disks
// actually perform well at — and only ever hands *already-in-memory* bytes to this pool for the
// CPU-only decompress/parse step, which has no further disk access to contend over.
type scanner struct {
	sem chan struct{}
	wg  sync.WaitGroup
}

func newScanner() *scanner {
	return &scanner{sem: make(chan struct{}, runtime.GOMAXPROCS(0))}
}

// submit blocks the caller until a pool slot is free, then runs fn on a pooled goroutine. This
// is a deliberate choice over a non-blocking dispatch: scanInto's sequential file-reading loop
// calls submit right after reading a file's full bytes into memory, so blocking here is what
// caps how many files' worth of raw (still-compressed) bytes can be buffered in memory at once
// to roughly GOMAXPROCS — without this backpressure, a walk that discovers recognized files
// faster than the pool can decompress them would accumulate unbounded memory. Each fn only ever
// touches state private to its own call (a single treeNode nothing else reads until wait
// returns), so no locking is needed beyond the semaphore controlling how many run at once.
func (s *scanner) submit(fn func()) {
	s.sem <- struct{}{}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() { <-s.sem }()
		fn()
	}()
}

// wait blocks until every submitted job has finished — called once, after the whole directory
// tree has been walked, so the tree is fully and stably populated before it's written out.
func (s *scanner) wait() {
	s.wg.Wait()
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
			sc := newScanner()
			if err := scanInto(sc, root, folder); err != nil {
				return err
			}
			sc.wait()

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
// recurse, and every file is fully read on *this* goroutine, sequentially, in directory-listing
// order — see the scanner doc comment for why file reads specifically are kept off the worker
// pool despite CPU-bound decompression being dispatched there. Only a recognized file's own
// declared size worth of I/O ever happens: an unrecognized file is dropped after a 12-byte
// magic check (readFilePrefix), never fully read.
//
// The child node for each file is created and appended here, in listing order, before its
// (already-read) bytes are handed to sc for the CPU-bound part — sc.wait() (called once, after
// the whole tree has been walked) is what guarantees every node is fully populated by the time
// the tree is written out, and because each file's job only ever touches its own
// already-positioned node, the final tree's shape and ordering come out identical to a fully
// sequential walk regardless of the order those background jobs happen to finish in. A
// subdirectory or file that can't be read (bad permissions, a broken symlink, ...) gets a single
// inline note instead of aborting the whole walk — an install with hundreds of thousands of
// files is exactly the situation where one bad entry shouldn't lose everything else.
func scanInto(sc *scanner, node *treeNode, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		full := filepath.Join(dir, e.Name())
		if e.IsDir() {
			child := &treeNode{name: e.Name()}
			node.children = append(node.children, child)
			if err := scanInto(sc, child, full); err != nil {
				child.children = append(child.children, &treeNode{name: fmt.Sprintf("(error reading directory: %v)", err)})
			}
			continue
		}

		child := &treeNode{name: e.Name()}
		node.children = append(node.children, child)

		prefix, err := readFilePrefix(full, 12)
		if err != nil {
			child.name = fmt.Sprintf("%s (error reading file: %v)", e.Name(), err)
			continue
		}
		isZbb := asura.CheckZbbMagic(prefix)
		if !isZbb && !asura.CheckMagic(prefix) {
			continue // not a recognized container — leave the node childless, no further I/O
		}

		raw, err := os.ReadFile(full)
		if err != nil {
			child.name = fmt.Sprintf("%s (error reading file: %v)", e.Name(), err)
			continue
		}
		sc.submit(func() {
			if isZbb {
				scanPackage(child, raw)
			} else {
				scanAsuraContainer(child, raw, prefix)
			}
		})
	}
	return nil
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

// scanPackage decompresses and parses raw as an AsuraZbb-compressed level package (already
// fully read from disk by the caller — see scanInto), listing its manifest sub-files, embedded
// textures, and embedded meshes by name. The full decompressed buffer (up to several hundred MB
// for a real level) is local to this call and goes out of scope once it returns, so running many
// of these concurrently (see the scanner doc comment) stays bounded to roughly
// GOMAXPROCS-many packages' worth of decompressed memory at once, not the whole install's.
func scanPackage(node *treeNode, raw []byte) {
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

// scanAsuraContainer dispatches a plain (non-AsuraZbb) "Asura   "-signed file (raw, already
// fully read from disk by the caller — see scanInto) to the right parser by peeking at the
// chunk tag immediately after the 8-byte magic: HTXT/ASTS/RSCF are always that very first tag,
// but DLLN voice entries are scattered through otherwise-unknown binary data (see UnpackVoice),
// so anything that doesn't match one of the other three falls through to a DLLN scan instead of
// being assumed unrecognized outright.
func scanAsuraContainer(node *treeNode, raw []byte, prefix []byte) {
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
