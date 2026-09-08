package domain

import (
	"context"
	"fmt"
	"reflect"
	"strings"
)

// ContactFileField is the shared contract for exports, templates and imports.
type ContactFileField struct {
	Key      string `json:"key"`
	Header   string `json:"header"`
	Type     string `json:"type"`
	ReadOnly bool   `json:"read_only"`
}

type ContactFileRepository interface {
	Fields(context.Context, string) ([]ContactFileField, error)
	LoadProfiles(context.Context, string, []*Contact) error
}

func BaseContactFileFields(labels map[string]string) []ContactFileField {
	// Preserve the established export order; newly added scalar Contact fields
	// are appended automatically from their JSON contract.
	order := []string{"email", "external_id", "first_name", "last_name", "full_name", "phone", "timezone", "language", "country", "address_line_1", "address_line_2", "state", "postcode", "job_title"}
	for _, kind := range []string{"string", "number", "datetime", "json"} {
		for index := 1; index <= 5; index++ {
			order = append(order, fmt.Sprintf("custom_%s_%d", kind, index))
		}
	}
	order = append(order, "created_at", "updated_at")
	known := map[string]ContactFileField{}
	contactType := reflect.TypeOf(Contact{})
	for index := 0; index < contactType.NumField(); index++ {
		field := contactType.Field(index)
		key := strings.Split(field.Tag.Get("json"), ",")[0]
		if key == "" || key == "-" || key == "profile" || key == "email_hmac" || field.Type.Kind() == reflect.Slice {
			continue
		}
		kind := "string"
		switch field.Type {
		case reflect.TypeOf((*NullableFloat64)(nil)):
			kind = "number"
		case reflect.TypeOf((*NullableTime)(nil)):
			kind = "datetime"
		case reflect.TypeOf((*NullableJSON)(nil)):
			kind = "json"
		}
		header := key
		if strings.HasPrefix(key, "custom_") {
			parts := strings.Split(key, "_")
			header = "Custom " + strings.ToUpper(parts[1][:1]) + parts[1][1:] + " " + parts[2]
			if label := strings.TrimSpace(labels[key]); label != "" {
				header = label
			}
			header += " (" + key + ")"
		}
		known[key] = ContactFileField{Key: key, Header: header, Type: kind, ReadOnly: key == "created_at" || key == "updated_at"}
	}
	fields := make([]ContactFileField, 0, len(known))
	for _, key := range order {
		if field, ok := known[key]; ok {
			fields = append(fields, field)
			delete(known, key)
		}
	}
	for index := 0; index < contactType.NumField(); index++ {
		key := strings.Split(contactType.Field(index).Tag.Get("json"), ",")[0]
		if field, ok := known[key]; ok {
			fields = append(fields, field)
		}
	}
	return fields
}

// Keep every generated header unique, including collisions with technical keys.
func UniqueContactFileHeaders(fields []ContactFileField) []ContactFileField {
	counts := map[string]int{}
	keys := map[string]string{}
	for _, field := range fields {
		counts[strings.ToLower(field.Header)]++
		keys[strings.ToLower(field.Key)] = field.Key
	}
	used := map[string]bool{}
	for index := range fields {
		field := &fields[index]
		lower := strings.ToLower(field.Header)
		if counts[lower] > 1 || (keys[lower] != "" && keys[lower] != field.Key) {
			field.Header = field.Header + " (" + field.Key + ")"
		}
		for used[strings.ToLower(field.Header)] {
			field.Header += " (" + field.Key + ")"
		}
		used[strings.ToLower(field.Header)] = true
	}
	return fields
}
