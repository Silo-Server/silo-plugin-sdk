// Command hello-scan-source is a minimal scan_source.v1 plugin for Silo
// Autoscan. Another tool appends changed paths to a change-log file, one per
// line; each poll returns the lines added since the previous poll.
//
// The marker is the byte offset just past the last complete line reported, so
// each poll reads only what was appended. The host stores it
// verbatim and sends it back on the next poll, so the plugin keeps no state of
// its own.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	goruntime "runtime"
	"strconv"
	"strings"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	publicmanifest "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
	sdkruntime "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtime"
)

//go:embed manifest.json
var manifestJSON []byte

// changesFileKey is the source_config key declared in manifest.json's
// config_form. The operator fills it in when adding the source.
const changesFileKey = "changes_file"

type runtimeServer struct {
	pluginv1.UnimplementedRuntimeServer
	manifest *pluginv1.PluginManifest
}

func (s *runtimeServer) GetManifest(context.Context, *pluginv1.GetManifestRequest) (*pluginv1.GetManifestResponse, error) {
	return &pluginv1.GetManifestResponse{Manifest: s.manifest}, nil
}

type scanSourceServer struct {
	pluginv1.UnimplementedScanSourceServer
}

func (scanSourceServer) PollChanges(_ context.Context, req *pluginv1.PollChangesRequest) (*pluginv1.PollChangesResponse, error) {
	path := strings.TrimSpace(req.GetSourceConfig()[changesFileKey])
	if path == "" {
		// The host records this message on the source and in the Autoscan
		// activity log, and keeps the marker, so write it for the operator.
		return nil, fmt.Errorf("%s is not configured", changesFileKey)
	}
	// The marker is the byte offset just past the last complete line already
	// reported, so each poll reads only what was appended since.
	marker := req.GetMarker()
	size, err := fileSize(path)
	if errors.Is(err, os.ErrNotExist) {
		if marker == "" || marker == "0" {
			// Nothing has been written yet, so a source can be created before
			// the other tool first writes. Read from the start once it does.
			return &pluginv1.PollChangesResponse{NextMarker: "0"}, nil
		}
		// The log held lines at an earlier poll. Returning "0" now would
		// replay all of them once the file is back, so fail instead: the host
		// keeps the marker and records this message on the source.
		return nil, fmt.Errorf("change log %s not found", path)
	}
	if err != nil {
		return nil, err
	}
	if marker == "" {
		// First poll for the source: start from now and do not replay what
		// the file already holds.
		return resyncToEnd(path)
	}
	offset, err := strconv.ParseInt(marker, 10, 64)
	if err != nil || offset < 0 {
		// Not a marker this plugin wrote. Resynchronize to the end instead of
		// replaying the whole file.
		return resyncToEnd(path)
	}
	if offset > size {
		// The log was truncated since the last poll, so every line it holds
		// now was written after that. Read it from the start.
		offset = 0
	} else if offset > 0 {
		// Every marker this plugin writes ends just past a newline. Any other
		// byte there means the log was truncated and has since grown past the
		// marker, so read it from the start too.
		atLineStart, err := followsNewline(path, offset)
		if err != nil {
			return nil, err
		}
		if !atLineStart {
			offset = 0
		}
	}

	lines, next, err := readAppended(path, offset)
	if err != nil {
		return nil, err
	}
	// Always return a non-empty marker. An empty next_marker is stored as "no
	// marker", which makes the next poll look like a first poll again.
	resp := &pluginv1.PollChangesResponse{NextMarker: strconv.FormatInt(next, 10)}
	for _, line := range lines {
		resp.Changes = append(resp.Changes, changeFor(line))
	}
	return resp, nil
}

// changeFor turns one change-log line into a structured change. A trailing
// slash marks a directory, which is reported as a subtree. Anything else is
// reported as a file; the host widens a video file to a scan of its directory
// and scans an existing directory as a subtree.
func changeFor(line string) *pluginv1.ScanSourceChange {
	if strings.HasSuffix(line, "/") {
		return &pluginv1.ScanSourceChange{
			SourcePath: strings.TrimRight(line, "/"),
			Scope:      pluginv1.ScanSourceChangeScope_SCAN_SOURCE_CHANGE_SCOPE_SUBTREE,
		}
	}
	return &pluginv1.ScanSourceChange{
		SourcePath: line,
		Scope:      pluginv1.ScanSourceChangeScope_SCAN_SOURCE_CHANGE_SCOPE_FILE,
	}
}

// fileSize returns the change log's size. The error wraps os.ErrNotExist when
// the file is missing.
func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat change log: %w", err)
	}
	return info.Size(), nil
}

// followsNewline reports whether the byte just before offset is a newline.
func followsNewline(path string, offset int64) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("open change log: %w", err)
	}
	defer func() { _ = f.Close() }() // read-only; a close error cannot lose data
	var b [1]byte
	if _, err := f.ReadAt(b[:], offset-1); err != nil {
		return false, fmt.Errorf("read change log: %w", err)
	}
	return b[0] == '\n', nil
}

// resyncToEnd answers a poll with no changes and a marker at the end of the
// last complete line, so the next poll reads only what is appended after now.
func resyncToEnd(path string) (*pluginv1.PollChangesResponse, error) {
	end, err := completeLinesEnd(path)
	if err != nil {
		return nil, err
	}
	return &pluginv1.PollChangesResponse{NextMarker: strconv.FormatInt(end, 10)}, nil
}

// readAppended returns the non-blank complete lines written after offset and
// the offset just past the last of them. A line still being written (no
// trailing newline yet) is left for the next poll.
func readAppended(path string, offset int64) ([]string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, offset, fmt.Errorf("open change log: %w", err)
	}
	defer func() { _ = f.Close() }() // read-only; a close error cannot lose data
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, fmt.Errorf("seek change log: %w", err)
	}

	var lines []string
	next := offset
	reader := bufio.NewReader(f)
	for {
		raw, err := reader.ReadString('\n')
		if err == io.EOF {
			return lines, next, nil // raw, if any, is an unfinished line
		}
		if err != nil {
			return nil, offset, fmt.Errorf("read change log: %w", err)
		}
		next += int64(len(raw))
		if line := strings.TrimSpace(raw); line != "" {
			lines = append(lines, line)
		}
	}
}

// completeLinesEnd returns the offset just past the last complete line: where
// reading resumes when a poll starts from now. It reads in blocks and keeps no
// lines, so a large log costs one pass and a fixed buffer.
func completeLinesEnd(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open change log: %w", err)
	}
	defer func() { _ = f.Close() }() // read-only; a close error cannot lose data

	var end, pos int64
	buf := make([]byte, 64*1024)
	for {
		n, err := f.Read(buf)
		if i := bytes.LastIndexByte(buf[:n], '\n'); i >= 0 {
			end = pos + int64(i) + 1
		}
		pos += int64(n)
		if err == io.EOF {
			return end, nil
		}
		if err != nil {
			return 0, fmt.Errorf("read change log: %w", err)
		}
	}
}

func main() {
	manifest, err := loadManifest()
	if err != nil {
		panic(err)
	}

	sdkruntime.Serve(sdkruntime.ServeConfig{
		Servers: sdkruntime.CapabilityServers{
			Runtime:    &runtimeServer{manifest: manifest},
			ScanSource: scanSourceServer{},
		},
	})
}

func loadManifest() (*pluginv1.PluginManifest, error) {
	manifest, err := publicmanifest.Load(manifestJSON)
	if err != nil {
		return nil, fmt.Errorf("load embedded manifest: %w", err)
	}

	executablePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable path: %w", err)
	}
	binaryData, err := os.ReadFile(executablePath)
	if err != nil {
		return nil, fmt.Errorf("read executable %q: %w", executablePath, err)
	}
	checksum := sha256.Sum256(binaryData)
	manifest.Checksum = hex.EncodeToString(checksum[:])
	if len(manifest.GetSupportedPlatforms()) == 0 {
		manifest.SupportedPlatforms = []*pluginv1.SupportedPlatform{
			{Os: goruntime.GOOS, Arch: goruntime.GOARCH},
		}
	}

	return manifest, nil
}
