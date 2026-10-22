package schema

import (
	"bytes"
	"encoding/json"
)

// emptyKeys remembers keys that held an empty value ("", false, 0, null, [],
// {}): omitempty drops them, and a round trip must not.
type emptyKeys map[string]json.RawMessage

func collectEmpty(data []byte) emptyKeys {
	var all map[string]json.RawMessage
	if json.Unmarshal(data, &all) != nil {
		return nil
	}
	var out emptyKeys
	for k, v := range all {
		if isEmptyJSON(v) {
			if out == nil {
				out = emptyKeys{}
			}
			out[k] = v
		}
	}
	return out
}

func isEmptyJSON(v json.RawMessage) bool {
	switch string(bytes.TrimSpace(v)) {
	case `""`, "false", "0", "null", "[]", "{}":
		return true
	}
	t := bytes.TrimSpace(v)
	return (bytes.HasPrefix(t, []byte("[")) || bytes.HasPrefix(t, []byte("{"))) && len(bytes.Join(bytes.Fields(t), nil)) == 2
}

// restore adds back the empty keys missing from an encoded object.
func (e emptyKeys) restore(encoded []byte) ([]byte, error) {
	if len(e) == 0 {
		return encoded, nil
	}
	var present map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &present); err != nil {
		return nil, err
	}
	missing := map[string]json.RawMessage{}
	for k, v := range e {
		if _, ok := present[k]; !ok {
			missing[k] = v
		}
	}
	return withExtra(json.RawMessage(encoded), missing)
}
