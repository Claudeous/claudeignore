package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// jsonObject is a JSON object that survives a decode/encode round trip without
// losing keys or reordering them. Values are kept as raw JSON, so anything
// claudeignore does not own — sandbox.network, permissions, env, custom keys —
// is written back byte for byte, exactly where the user put it.
type jsonObject struct {
	keys   []string
	values map[string]json.RawMessage
}

func newJSONObject() *jsonObject {
	return &jsonObject{values: make(map[string]json.RawMessage)}
}

// parseJSONObject decodes a JSON object, recording the order of its keys.
func parseJSONObject(data []byte) (*jsonObject, error) {
	dec := json.NewDecoder(bytes.NewReader(data))

	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("expected a JSON object, got %v", tok)
	}

	o := newJSONObject()
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("expected an object key, got %v", keyTok)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		o.set(key, raw)
	}
	if _, err := dec.Token(); err != nil { // closing brace
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("unexpected trailing data after the JSON object")
	}

	return o, nil
}

// set stores a raw value, keeping the position of a key that already exists
// and appending a new one at the end.
func (o *jsonObject) set(key string, raw json.RawMessage) {
	if o.values == nil {
		o.values = make(map[string]json.RawMessage)
	}
	if _, exists := o.values[key]; !exists {
		o.keys = append(o.keys, key)
	}
	o.values[key] = raw
}

func (o *jsonObject) get(key string) (json.RawMessage, bool) {
	if o == nil {
		return nil, false
	}
	raw, ok := o.values[key]
	return raw, ok
}

// object returns the value at key decoded as a nested object. It reports false
// when the key is missing or does not hold a JSON object.
func (o *jsonObject) object(key string) (*jsonObject, bool) {
	raw, ok := o.get(key)
	if !ok {
		return nil, false
	}
	child, err := parseJSONObject(raw)
	if err != nil {
		return nil, false
	}
	return child, true
}

// getPath walks a nested key path and returns the raw value at its end.
func (o *jsonObject) getPath(keys ...string) (json.RawMessage, bool) {
	if len(keys) == 0 {
		return nil, false
	}
	cur := o
	for _, key := range keys[:len(keys)-1] {
		child, ok := cur.object(key)
		if !ok {
			return nil, false
		}
		cur = child
	}
	return cur.get(keys[len(keys)-1])
}

// setPath writes a raw value at a nested key path, creating the intermediate
// objects it needs. Sibling keys are preserved at every level; an intermediate
// value that exists but is not an object is replaced.
func (o *jsonObject) setPath(raw json.RawMessage, keys ...string) error {
	if len(keys) == 0 {
		return errors.New("empty key path")
	}
	if len(keys) == 1 {
		o.set(keys[0], raw)
		return nil
	}

	child, ok := o.object(keys[0])
	if !ok {
		child = newJSONObject()
	}
	if err := child.setPath(raw, keys[1:]...); err != nil {
		return err
	}
	encoded, err := child.MarshalJSON()
	if err != nil {
		return err
	}
	o.set(keys[0], encoded)
	return nil
}

// MarshalJSON renders the object in key order. Raw values are copied verbatim,
// never re-encoded, so user content is not re-escaped along the way.
func (o *jsonObject) MarshalJSON() ([]byte, error) {
	if o == nil {
		return []byte("{}"), nil
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := encodeJSON(k)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(o.values[k])
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// encodeJSON marshals a value without HTML escaping, so paths and commands
// containing &, < or > stay readable in the settings file.
func encodeJSON(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// formatJSON renders raw JSON with the two-space indentation Claude Code uses.
func formatJSON(raw []byte) ([]byte, error) {
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, compact.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
