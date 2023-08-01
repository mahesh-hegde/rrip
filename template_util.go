package main

import (
	"encoding/json"
	"strings"
	"text/template"
)

func createTemplate(name string, tm string) *template.Template {
	tmpl, err := template.New(name).Parse(tm)
	check(err, "cannot parse template:", tm)
	return tmpl
}

func formatTemplate(tm *template.Template, value any) string {
	var sb strings.Builder
	err := tm.Execute(&sb, value)
	check(err, "Cannot apply template to value:", value)
	return sb.String()
}

// Returns value as json with indentation
func marshallIndent(value any) string {
	b, err := json.MarshalIndent(value, "", "  ")
	check(err)
	return string(b)
}
