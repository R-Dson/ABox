package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// pullProgress parses Docker image pull JSON stream and displays
// a single-line aggregate percentage on the terminal.
type pullProgress struct {
	w      io.Writer
	prefix string
	buf    []byte
	layers map[string]struct{ current, total int64 }
}

func newPullProgress(w io.Writer, image string) *pullProgress {
	p := &pullProgress{
		w:      w,
		prefix: fmt.Sprintf("Pulling %s...", image),
		layers: make(map[string]struct{ current, total int64 }),
	}
	fmt.Fprint(w, p.prefix)
	return p
}

func (p *pullProgress) Write(data []byte) (int, error) {
	p.buf = append(p.buf, data...)
	for {
		i := bytes.IndexByte(p.buf, '\n')
		if i < 0 {
			break
		}
		line := p.buf[:i]
		p.buf = p.buf[i+1:]
		p.processLine(line)
	}
	return len(data), nil
}

func (p *pullProgress) processLine(line []byte) {
	var msg struct {
		Status         string `json:"status"`
		ProgressDetail struct {
			Current int64 `json:"current"`
			Total   int64 `json:"total"`
		} `json:"progressDetail"`
		ID string `json:"id"`
	}
	if err := json.Unmarshal(line, &msg); err != nil {
		return
	}
	if msg.ID == "" || msg.ProgressDetail.Total == 0 {
		return
	}

	layer := p.layers[msg.ID]
	layer.current = msg.ProgressDetail.Current
	layer.total = msg.ProgressDetail.Total
	p.layers[msg.ID] = layer

	p.render()
}

func (p *pullProgress) render() {
	var totalBytes, pulledBytes int64
	for _, l := range p.layers {
		totalBytes += l.total
		pulledBytes += l.current
	}
	if totalBytes == 0 {
		return
	}
	pct := pulledBytes * 100 / totalBytes
	fmt.Fprintf(p.w, "\r%s %3d%%", p.prefix, pct)
}

func (p *pullProgress) finish() {
	fmt.Fprint(p.w, "\n")
}
