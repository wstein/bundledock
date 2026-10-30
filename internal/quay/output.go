package quay

import (
	"encoding/json"
	"io"
)

// WriteJSON writes one JSON document with a "schema" field. value must
// marshal to a JSON object.
func WriteJSON(w io.Writer, schema string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return err
	}
	object["schema"] = schema
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(object)
}
