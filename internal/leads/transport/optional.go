package transport

import (
	"encoding/json"

	"github.com/google/uuid"
)

type OptionalUUID struct {
	Value *uuid.UUID
	Set   bool
}

func (o OptionalUUID) IsZero() bool {
	return !o.Set
}

func (o *OptionalUUID) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}

	var raw string
	if err := json.Unmarshal(data, &raw); err == nil {
		if raw == "" {
			o.Value = nil
			return nil
		}

		parsed, err := uuid.Parse(raw)
		if err != nil {
			return err
		}

		o.Value = &parsed
		return nil
	}

	var parsed uuid.UUID
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}

	o.Value = &parsed
	return nil
}
