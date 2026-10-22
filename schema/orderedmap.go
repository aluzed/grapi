package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// OrderedMap is a JSON object that keeps its key order: attribute order in a
// schema.json drives the admin layouts, so a plain map would lose information.
type OrderedMap[V any] struct {
	keys   []string
	values map[string]V
}

// Len returns the number of entries.
func (m *OrderedMap[V]) Len() int { return len(m.keys) }

// Keys returns the keys in insertion order.
func (m *OrderedMap[V]) Keys() []string { return append([]string(nil), m.keys...) }

// Get returns the value for key.
func (m *OrderedMap[V]) Get(key string) (V, bool) {
	v, ok := m.values[key]
	return v, ok
}

// Set inserts or replaces key; a new key goes last.
func (m *OrderedMap[V]) Set(key string, v V) {
	if m.values == nil {
		m.values = map[string]V{}
	}
	if _, ok := m.values[key]; !ok {
		m.keys = append(m.keys, key)
	}
	m.values[key] = v
}

// Delete removes key if present.
func (m *OrderedMap[V]) Delete(key string) {
	if _, ok := m.values[key]; !ok {
		return
	}
	delete(m.values, key)
	for i, k := range m.keys {
		if k == key {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			break
		}
	}
}

// All iterates in key order.
func (m *OrderedMap[V]) All() func(yield func(string, V) bool) {
	return func(yield func(string, V) bool) {
		for _, k := range m.keys {
			if !yield(k, m.values[k]) {
				return
			}
		}
	}
}

// UnmarshalJSON decodes an object token by token to keep the key order.
func (m *OrderedMap[V]) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("expected a JSON object, got %v", tok)
	}
	*m = OrderedMap[V]{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := tok.(string)
		if !ok {
			return fmt.Errorf("expected an object key, got %v", tok)
		}
		var v V
		if err := dec.Decode(&v); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		m.Set(key, v)
	}
	_, err = dec.Token()
	return err
}

// MarshalJSON encodes the object in key order.
func (m OrderedMap[V]) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range m.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		vb, err := json.Marshal(m.values[k])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
