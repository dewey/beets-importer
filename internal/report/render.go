package report

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
)

//go:embed template.html
var pageTemplate string

// Render writes the report as one self-contained HTML page.
func Render(w io.Writer, d Data) error {
	payload, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("encode report data: %w", err)
	}
	t, err := template.New("report").Parse(pageTemplate)
	if err != nil {
		return fmt.Errorf("parse report template: %w", err)
	}
	return t.Execute(w, struct {
		Data template.JS
		Date string
	}{template.JS(payload), d.Generated})
}
