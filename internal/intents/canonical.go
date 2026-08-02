package intents

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// canonicalJSON implements the RFC 8785 subset used by the closed intent
// envelope: objects, arrays, UTF-8 strings, booleans, null, and integers.
// Maps, floats, interfaces, and unexported fields are rejected so new material
// types cannot silently weaken the digest contract.
func canonicalJSON(value any) ([]byte, error) {
	var builder strings.Builder
	if err := appendCanonical(&builder, reflect.ValueOf(value)); err != nil {
		return nil, err
	}
	return []byte(builder.String()), nil
}

func appendCanonical(builder *strings.Builder, value reflect.Value) error {
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			builder.WriteString("null")
			return nil
		}
		return appendCanonical(builder, value.Elem())
	}
	if value.Type() == reflect.TypeOf(time.Time{}) {
		appendJSONString(builder, value.Interface().(time.Time).UTC().Format(time.RFC3339Nano))
		return nil
	}
	switch value.Kind() {
	case reflect.Struct:
		type field struct {
			name  string
			value reflect.Value
		}
		fields := make([]field, 0, value.NumField())
		typeInfo := value.Type()
		for index := 0; index < value.NumField(); index++ {
			definition := typeInfo.Field(index)
			if definition.PkgPath != "" {
				return fmt.Errorf("canonical material contains unexported field %s", definition.Name)
			}
			tag := definition.Tag.Get("json")
			parts := strings.Split(tag, ",")
			name := parts[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = definition.Name
			}
			omitEmpty := len(parts) > 1 && parts[1] == "omitempty"
			fieldValue := value.Field(index)
			if omitEmpty && fieldValue.Kind() == reflect.Pointer && fieldValue.IsNil() {
				continue
			}
			fields = append(fields, field{name, fieldValue})
		}
		sort.Slice(fields, func(left, right int) bool { return fields[left].name < fields[right].name })
		builder.WriteByte('{')
		for index, item := range fields {
			if index > 0 {
				builder.WriteByte(',')
			}
			appendJSONString(builder, item.name)
			builder.WriteByte(':')
			if err := appendCanonical(builder, item.value); err != nil {
				return err
			}
		}
		builder.WriteByte('}')
	case reflect.Slice, reflect.Array:
		builder.WriteByte('[')
		for index := 0; index < value.Len(); index++ {
			if index > 0 {
				builder.WriteByte(',')
			}
			if err := appendCanonical(builder, value.Index(index)); err != nil {
				return err
			}
		}
		builder.WriteByte(']')
	case reflect.String:
		if !utf8.ValidString(value.String()) {
			return fmt.Errorf("canonical material contains invalid UTF-8")
		}
		appendJSONString(builder, value.String())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		builder.WriteString(strconv.FormatUint(value.Uint(), 10))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		builder.WriteString(strconv.FormatInt(value.Int(), 10))
	case reflect.Bool:
		builder.WriteString(strconv.FormatBool(value.Bool()))
	default:
		return fmt.Errorf("unsupported canonical material type %s", value.Kind())
	}
	return nil
}

func appendJSONString(builder *strings.Builder, value string) {
	const hexadecimal = "0123456789abcdef"
	builder.WriteByte('"')
	for _, character := range value {
		switch character {
		case '"', '\\':
			builder.WriteByte('\\')
			builder.WriteRune(character)
		case '\b':
			builder.WriteString("\\b")
		case '\t':
			builder.WriteString("\\t")
		case '\n':
			builder.WriteString("\\n")
		case '\f':
			builder.WriteString("\\f")
		case '\r':
			builder.WriteString("\\r")
		default:
			if character < 0x20 {
				builder.WriteString("\\u00")
				builder.WriteByte(hexadecimal[byte(character)>>4])
				builder.WriteByte(hexadecimal[byte(character)&0x0f])
			} else {
				builder.WriteRune(character)
			}
		}
	}
	builder.WriteByte('"')
}
