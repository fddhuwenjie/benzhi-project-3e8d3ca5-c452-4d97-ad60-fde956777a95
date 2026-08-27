package evidence

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func CanonicalJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err = json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err = enc.Encode(decoded); err != nil {
		return nil, fmt.Errorf("canonical encode: %w", err)
	}
	return bytes.TrimSpace(b.Bytes()), nil
}
