package db

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// SQLite datetime format (from datetime('now'))
const SQLiteTimeFormat = "2006-01-02 15:04:05"

// JSONMap handles scanning and storing map[string]any as JSON text.
type JSONMap map[string]any

func (j *JSONMap) Scan(value any) error {
	if value == nil {
		*j = nil
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into JSONMap", value)
	}
	if len(data) == 0 {
		*j = nil
		return nil
	}
	return json.Unmarshal(data, j)
}

func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// JSONIntArray handles scanning and storing []int as JSON text.
type JSONIntArray []int

func (j *JSONIntArray) Scan(value any) error {
	if value == nil {
		*j = nil
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into JSONIntArray", value)
	}
	if len(data) == 0 {
		*j = nil
		return nil
	}
	return json.Unmarshal(data, j)
}

func (j JSONIntArray) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// JSONStringArray handles scanning and storing []string as JSON text.
type JSONStringArray []string

func (j *JSONStringArray) Scan(value any) error {
	if value == nil {
		*j = nil
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into JSONStringArray", value)
	}
	if len(data) == 0 {
		*j = nil
		return nil
	}
	return json.Unmarshal(data, j)
}

func (j JSONStringArray) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// NullTime handles scanning SQLite TEXT datetime columns.
type NullTime struct {
	Time  time.Time
	Valid bool
}

func (t *NullTime) Scan(value any) error {
	if value == nil {
		t.Valid = false
		return nil
	}
	var str string
	switch v := value.(type) {
	case []byte:
		str = string(v)
	case string:
		str = v
	default:
		return fmt.Errorf("cannot scan %T into NullTime", value)
	}
	if str == "" {
		t.Valid = false
		return nil
	}
	// Try multiple formats
	formats := []string{
		SQLiteTimeFormat,
		time.RFC3339,
		time.RFC3339Nano,
	}
	for _, format := range formats {
		if parsed, err := time.Parse(format, str); err == nil {
			t.Time = parsed
			t.Valid = true
			return nil
		}
	}
	return fmt.Errorf("cannot parse time %q", str)
}
