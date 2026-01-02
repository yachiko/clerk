package util

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"
)

// OutputFormat represents the output format
type OutputFormat string

const (
	OutputPlain OutputFormat = "plain"
	OutputJSON  OutputFormat = "json"
)

// Formatter handles output formatting
type Formatter struct {
	format OutputFormat
	writer io.Writer
}

// NewFormatter creates a new output formatter
func NewFormatter(format string, writer io.Writer) *Formatter {
	f := OutputFormat(strings.ToLower(format))
	if f != OutputJSON {
		f = OutputPlain
	}
	return &Formatter{
		format: f,
		writer: writer,
	}
}

// Print outputs data in the configured format
func (f *Formatter) Print(data any) error {
	if f.format == OutputJSON {
		return f.printJSON(data)
	}
	return f.printPlain(data)
}

func (f *Formatter) printJSON(data any) error {
	encoder := json.NewEncoder(f.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func (f *Formatter) printPlain(data any) error {
	_, err := fmt.Fprintln(f.writer, data)
	return err
}

// PrintSuccess prints a success message
func (f *Formatter) PrintSuccess(format string, args ...any) {
	if f.format == OutputJSON {
		return
	}
	color.Green(format, args...)
}

// PrintError prints an error message
func (f *Formatter) PrintError(format string, args ...any) {
	if f.format == OutputJSON {
		return
	}
	color.Red(format, args...)
}

// PrintWarning prints a warning message
func (f *Formatter) PrintWarning(format string, args ...any) {
	if f.format == OutputJSON {
		return
	}
	color.Yellow(format, args...)
}

// PrintInfo prints an info message
func (f *Formatter) PrintInfo(format string, args ...any) {
	if f.format == OutputJSON {
		return
	}
	color.Cyan(format, args...)
}

// MaskValue masks a secret value
func MaskValue(value string) string {
	if len(value) <= 8 {
		return strings.Repeat("*", len(value))
	}
	return value[:2] + strings.Repeat("*", len(value)-4) + value[len(value)-2:]
}

// MaskValueFull returns a fully masked value
func MaskValueFull(value string) string {
	return strings.Repeat("*", len(value))
}
