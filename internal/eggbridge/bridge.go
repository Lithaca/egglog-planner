package eggbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Options struct {
	Executable string
	WorkDir    string
	Timeout    time.Duration
	MaxBytes   int64
	MaxNodes   int
}

type Node struct {
	Op       string   `json:"op"`
	Children []string `json:"children"`
	EClass   string   `json:"eclass"`
	Subsumed bool     `json:"subsumed"`
}

type Graph struct {
	Nodes map[string]Node `json:"nodes"`
}

type cappedOutput struct {
	mu       sync.Mutex
	limit    int64
	buf      bytes.Buffer
	exceeded bool
}

func newCappedOutput(limit int64) *cappedOutput { return &cappedOutput{limit: limit} }
func (c *cappedOutput) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := len(p)
	remaining := c.limit - int64(c.buf.Len())
	if remaining <= 0 {
		if n > 0 {
			c.exceeded = true
		}
		return n, nil
	}
	if int64(n) > remaining {
		c.exceeded = true
		p = p[:int(remaining)]
	}
	_, _ = c.buf.Write(p)
	return n, nil
}
func (c *cappedOutput) String() string { c.mu.Lock(); defer c.mu.Unlock(); return c.buf.String() }
func (c *cappedOutput) Exceeded() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.exceeded }

func ParseGraph(data []byte, maxNodes int) (Graph, error) {
	var g Graph
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&g); err != nil {
		return g, fmt.Errorf("egglog: malformed graph: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return g, fmt.Errorf("egglog: trailing or malformed graph data")
	}
	if maxNodes <= 0 {
		maxNodes = 2000
	}
	if len(g.Nodes) == 0 || len(g.Nodes) > maxNodes {
		return g, fmt.Errorf("egglog: graph node count %d outside 1..%d", len(g.Nodes), maxNodes)
	}
	for id, n := range g.Nodes {
		if id == "" || n.Op == "" || n.EClass == "" {
			return g, fmt.Errorf("egglog: malformed node %q", id)
		}
		for _, child := range n.Children {
			if _, ok := g.Nodes[child]; !ok {
				return g, fmt.Errorf("egglog: missing child %q", child)
			}
		}
	}
	return g, nil
}

func Run(ctx context.Context, o Options, program string) (Graph, error) {
	if err := ctx.Err(); err != nil {
		return Graph{}, fmt.Errorf("egglog: %w", err)
	}
	if o.Executable == "" {
		return Graph{}, fmt.Errorf("egglog: executable required")
	}
	if o.WorkDir == "" {
		return Graph{}, fmt.Errorf("egglog: work directory required")
	}
	if o.Timeout <= 0 {
		o.Timeout = 5 * time.Second
	}
	if o.MaxBytes <= 0 {
		o.MaxBytes = 4 << 20
	}
	versionCtx, versionCancel := context.WithTimeout(ctx, o.Timeout)
	defer versionCancel()
	versionOutput := newCappedOutput(4096)
	versionCmd := exec.CommandContext(versionCtx, o.Executable, "--version")
	versionCmd.Stdout = versionOutput
	versionCmd.Stderr = versionOutput
	err := versionCmd.Run()
	if versionCtx.Err() != nil {
		return Graph{}, fmt.Errorf("egglog: %w", versionCtx.Err())
	}
	if versionOutput.Exceeded() {
		return Graph{}, fmt.Errorf("egglog: version output exceeds 4096 bytes")
	}
	if err != nil {
		return Graph{}, fmt.Errorf("egglog: version check: %w: %s", err, strings.TrimSpace(versionOutput.String()))
	}
	if !strings.HasPrefix(strings.TrimSpace(versionOutput.String()), "egglog 0.4.0") {
		return Graph{}, fmt.Errorf("egglog: require 0.4.0, got %q", strings.TrimSpace(versionOutput.String()))
	}
	dir, err := os.MkdirTemp(o.WorkDir, "egglog-")
	if err != nil {
		return Graph{}, fmt.Errorf("egglog: create work directory: %w", err)
	}
	defer os.RemoveAll(dir)
	source := filepath.Join(dir, "query.egg")
	if err := os.WriteFile(source, []byte(program), 0600); err != nil {
		return Graph{}, fmt.Errorf("egglog: write program: %w", err)
	}
	childCtx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	cmd := exec.CommandContext(childCtx, o.Executable, "--to-json", source)
	output := newCappedOutput(o.MaxBytes)
	cmd.Stdout = output
	cmd.Stderr = output
	err = cmd.Run()
	if childCtx.Err() != nil {
		return Graph{}, fmt.Errorf("egglog: %w", childCtx.Err())
	}
	if output.Exceeded() {
		return Graph{}, fmt.Errorf("egglog: output exceeds %d bytes", o.MaxBytes)
	}
	if err != nil {
		return Graph{}, fmt.Errorf("egglog: %w: %s", err, strings.TrimSpace(output.String()))
	}
	path := filepath.Join(dir, "query.json")
	stat, err := os.Stat(path)
	if err != nil {
		return Graph{}, fmt.Errorf("egglog: missing graph: %w", err)
	}
	if stat.Size() > o.MaxBytes {
		return Graph{}, fmt.Errorf("egglog: graph exceeds %d bytes", o.MaxBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Graph{}, fmt.Errorf("egglog: read graph: %w", err)
	}
	return ParseGraph(data, o.MaxNodes)
}
