package components

import (
	"fmt"
	"math"
	"reflect"
	"time"
)

type FieldType string

const (
	FieldString   FieldType = "string"
	FieldInteger  FieldType = "integer"
	FieldBoolean  FieldType = "boolean"
	FieldDuration FieldType = "duration"
	FieldObject   FieldType = "object"
)

type FieldSchema struct {
	Type     FieldType `json:"type"`
	Required bool      `json:"required,omitempty"`
	Secret   bool      `json:"secret,omitempty"`
}

type Schema struct {
	Fields map[string]FieldSchema `json:"fields"`
}

func (s Schema) ValidateDeclaration() error {
	for name, field := range s.Fields {
		if name == "" {
			return fmt.Errorf("field name is required")
		}
		switch field.Type {
		case FieldString, FieldInteger, FieldBoolean, FieldDuration, FieldObject:
		default:
			return fmt.Errorf("field %q has unsupported type %q", name, field.Type)
		}
	}
	return nil
}

func (s Schema) Validate(configuration Configuration) error {
	if err := s.ValidateDeclaration(); err != nil {
		return err
	}
	for name, field := range s.Fields {
		value, present := configuration[name]
		if !present {
			if field.Required {
				return fmt.Errorf("required field %q is missing", name)
			}
			continue
		}
		if !matchesType(value, field.Type) {
			return fmt.Errorf("field %q has invalid type for %s", name, field.Type)
		}
	}
	for name := range configuration {
		if _, known := s.Fields[name]; !known {
			return fmt.Errorf("unknown configuration field %q", name)
		}
	}
	return nil
}

func matchesType(value any, fieldType FieldType) bool {
	switch fieldType {
	case FieldString:
		_, ok := value.(string)
		return ok
	case FieldBoolean:
		_, ok := value.(bool)
		return ok
	case FieldInteger:
		return isInteger(value)
	case FieldDuration:
		if duration, ok := value.(time.Duration); ok {
			return duration >= 0
		}
		text, ok := value.(string)
		if !ok {
			return false
		}
		_, err := time.ParseDuration(text)
		return err == nil
	case FieldObject:
		kind := reflect.ValueOf(value).Kind()
		return kind == reflect.Map || kind == reflect.Struct
	default:
		return false
	}
}

func isInteger(value any) bool {
	switch number := value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case float32:
		return !math.IsNaN(float64(number)) && math.Trunc(float64(number)) == float64(number)
	case float64:
		return !math.IsNaN(number) && math.Trunc(number) == number
	default:
		return false
	}
}
