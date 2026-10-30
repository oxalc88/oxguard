package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Read exact Git blobs, not git archive: export-ignore/export-subst attributes
// must not hide or transform baseline source. No checkout, hooks or source runs.
func baselinePath(name string) (string, bool, error) {
	clean := filepath.FromSlash(name)
	if name == "" || filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" || strings.Contains(name, "\\") || clean == ".." || strings.HasPrefix(filepath.Clean(clean), ".."+string(filepath.Separator)) {
		return "", false, fmt.Errorf("unsafe baseline path")
	}
	for _, part := range strings.Split(filepath.ToSlash(clean), "/") {
		if part == ".." || part == "." || part == "" {
			return "", false, fmt.Errorf("unsafe baseline path")
		}
		if part == "node_modules" || part == ".git" || part == "venv" || part == ".venv" || part == "__pycache__" {
			return clean, false, nil
		}
	}
	if filepath.Ext(clean) == ".py" || filepath.Ext(clean) == ".pyi" || filepath.Base(clean) == "pyproject.toml" || filepath.Base(clean) == "setup.cfg" {
		return clean, true, nil
	}
	return clean, false, nil
}

type baselineBlob struct{ name, sha string }

const (
	baselineListingLimit = 4 * 1024 * 1024
	baselineFileLimit    = 20000
	baselineBlobLimit    = 8 * 1024 * 1024
	baselineTotalLimit   = 128 * 1024 * 1024
)

type baselineBudget struct{ total int64 }

func (b *baselineBudget) add(size int64) error {
	if size < 0 || size > baselineBlobLimit {
		return fmt.Errorf("baseline file exceeds analysis budget")
	}
	if b.total > baselineTotalLimit-size {
		return fmt.Errorf("baseline source exceeds 128 MiB")
	}
	b.total += size
	return nil
}

func parseBaselineTree(data []byte, prefix string) ([]baselineBlob, error) {
	if len(data) > baselineListingLimit {
		return nil, fmt.Errorf("baseline tree exceeds analysis budget")
	}
	blobs := []baselineBlob{}
	for _, entry := range bytes.Split(data, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		fields := bytes.SplitN(entry, []byte{'\t'}, 2)
		if len(fields) != 2 {
			return nil, fmt.Errorf("invalid Git tree record")
		}
		meta := strings.Fields(string(fields[0]))
		if len(meta) != 3 {
			return nil, fmt.Errorf("invalid Git tree metadata")
		}
		if (meta[0] != "100644" && meta[0] != "100755") || meta[1] != "blob" {
			continue
		} // no symlinks/submodules
		name := string(fields[1])
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		name = strings.TrimPrefix(name, prefix)
		clean, selected, err := baselinePath(name)
		if err != nil {
			return nil, err
		}
		if !selected {
			continue
		}
		if len(blobs) >= baselineFileLimit {
			return nil, fmt.Errorf("baseline source exceeds 20000 files")
		}
		blobs = append(blobs, baselineBlob{clean, meta[2]})
	}
	return blobs, nil
}

func baselineRoot(ctx context.Context, root, sha string) (string, error) {
	prefixCommand := exec.CommandContext(ctx, "git", "rev-parse", "--show-prefix")
	prefixCommand.Dir = root
	prefixBytes, err := prefixCommand.Output()
	if err != nil {
		return "", err
	}
	prefix := strings.TrimSuffix(strings.TrimSuffix(string(prefixBytes), "\n"), "\r")
	listing := exec.CommandContext(ctx, "git", "ls-tree", "-r", "--full-tree", "-z", sha)
	listing.Dir = root
	stdout, err := listing.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err = listing.Start(); err != nil {
		return "", err
	}
	data, readErr := io.ReadAll(io.LimitReader(stdout, baselineListingLimit+1))
	if readErr != nil || len(data) > baselineListingLimit {
		listing.Process.Kill()
	}
	waitErr := listing.Wait()
	if readErr != nil {
		return "", readErr
	}
	if len(data) > baselineListingLimit {
		return "", fmt.Errorf("baseline tree exceeds analysis budget")
	}
	if waitErr != nil {
		return "", waitErr
	}
	blobs, err := parseBaselineTree(data, prefix)
	if err != nil {
		return "", err
	}
	var requests strings.Builder
	for _, blob := range blobs {
		requests.WriteString(blob.sha + "\n")
	}

	temporary, err := os.MkdirTemp("", "pyguard-baseline-")
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(temporary)
		}
	}()
	cmd := exec.CommandContext(ctx, "git", "cat-file", "--batch")
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(requests.String())
	stream, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err = cmd.Start(); err != nil {
		return "", err
	}
	copyErr := copyBaselineBlobs(bufio.NewReader(stream), temporary, blobs)
	if copyErr != nil {
		cmd.Process.Kill()
	}
	waitErr = cmd.Wait()
	if copyErr != nil {
		return "", copyErr
	}
	if waitErr != nil {
		return "", fmt.Errorf("git cat-file: %w", waitErr)
	}

	ok = true
	return temporary, nil
}

func copyBaselineBlobs(reader *bufio.Reader, root string, blobs []baselineBlob) error {
	var budget baselineBudget
	for _, blob := range blobs {
		if _, selected, err := baselinePath(filepath.ToSlash(blob.name)); err != nil || !selected {
			return fmt.Errorf("unsafe baseline blob path")
		}
		header, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		fields := strings.Fields(header)
		if len(fields) != 3 || fields[0] != blob.sha || fields[1] != "blob" {
			return fmt.Errorf("invalid Git blob response")
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid baseline blob size")
		}
		if err := budget.add(size); err != nil {
			return err
		}

		destination := filepath.Join(root, blob.name)
		if err = os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			return err
		}
		file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, copyErr := io.CopyN(file, reader, size)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		separator, err := reader.ReadByte()
		if err != nil || separator != '\n' {
			return fmt.Errorf("invalid Git blob separator")
		}
	}
	return nil
}
