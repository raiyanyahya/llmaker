package dockerrt

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// decodePullStream parses the JSON event stream Docker emits during an image
// pull and forwards short, human-readable progress lines to onEvent.
func decodePullStream(r io.Reader, onEvent func(string)) error {
	dec := json.NewDecoder(r)
	for {
		var m struct {
			Status   string `json:"status"`
			ID       string `json:"id"`
			Progress string `json:"progress"`
			Error    string `json:"error"`
		}
		if err := dec.Decode(&m); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if m.Error != "" {
			return errors.New(m.Error)
		}
		if onEvent == nil {
			continue
		}
		line := m.Status
		if m.Progress != "" {
			line = m.Status + " " + m.Progress
		} else if m.ID != "" {
			line = m.ID + " " + m.Status
		}
		if s := strings.TrimSpace(line); s != "" {
			onEvent(s)
		}
	}
}
