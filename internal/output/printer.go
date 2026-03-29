package output

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
)

type Format string

const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
)

func ParseFormat(value string) (Format, error) {
	format := Format(value)
	switch format {
	case FormatTable, FormatJSON:
		return format, nil
	default:
		return "", fmt.Errorf("unsupported output format %q", value)
	}
}

type Printer struct {
	writer io.Writer
	format Format
}

func New(writer io.Writer, format Format) *Printer {
	return &Printer{
		writer: writer,
		format: format,
	}
}

func (p *Printer) PrintJSON(value any) error {
	encoder := json.NewEncoder(p.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func (p *Printer) PrintTable(headers []string, rows [][]string) error {
	if p.format != FormatTable {
		return fmt.Errorf("table output is not supported for format %q", p.format)
	}

	writer := tabwriter.NewWriter(p.writer, 0, 8, 2, ' ', 0)
	if err := writeRow(writer, headers); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writeRow(writer, row); err != nil {
			return err
		}
	}

	return writer.Flush()
}

func writeRow(writer io.Writer, columns []string) error {
	for index, column := range columns {
		if index > 0 {
			if _, err := io.WriteString(writer, "\t"); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(writer, column); err != nil {
			return err
		}
	}
	_, err := io.WriteString(writer, "\n")
	return err
}
