package output

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/PaesslerAG/jsonpath"
	"gopkg.in/yaml.v3"
)

type Options struct {
	Format    string
	NoHeaders bool
	Template  string
	JSONPath  string
	TTY       bool
}

func Render(w io.Writer, data any, opts Options) error {
	format := strings.TrimSpace(opts.Format)
	if format == "" {
		if opts.TTY {
			format = "table"
		} else {
			format = "json"
		}
	}

	switch format {
	case "table", "wide":
		return renderTable(w, data, opts.NoHeaders)
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	case "yaml":
		enc, err := yaml.Marshal(data)
		if err != nil {
			return err
		}
		_, err = w.Write(enc)
		return err
	case "csv":
		return renderCSV(w, data)
	case "go-template":
		return renderTemplate(w, data, opts.Template)
	case "jsonpath":
		return renderJSONPath(w, data, opts.JSONPath)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func renderTable(w io.Writer, data any, noHeaders bool) error {
	headers, rows, err := rowsFromData(data)
	if err != nil {
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if !noHeaders {
		_, _ = fmt.Fprintln(tw, strings.Join(headers, "\t"))
	}
	for _, row := range rows {
		cells := make([]string, 0, len(headers))
		for _, h := range headers {
			cells = append(cells, row[h])
		}
		_, _ = fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}

	return tw.Flush()
}

func renderCSV(w io.Writer, data any) error {
	headers, rows, err := rowsFromData(data)
	if err != nil {
		return err
	}

	cw := csv.NewWriter(w)
	if err := cw.Write(headers); err != nil {
		return err
	}
	for _, row := range rows {
		record := make([]string, 0, len(headers))
		for _, h := range headers {
			record = append(record, row[h])
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func renderTemplate(w io.Writer, data any, tpl string) error {
	if strings.TrimSpace(tpl) == "" {
		return fmt.Errorf("template is required for go-template output")
	}
	t, err := template.New("out").Parse(tpl)
	if err != nil {
		return err
	}
	return t.Execute(w, data)
}

func renderJSONPath(w io.Writer, data any, expr string) error {
	if strings.TrimSpace(expr) == "" {
		return fmt.Errorf("jsonpath is required for jsonpath output")
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}

	var in any
	if err := json.Unmarshal(raw, &in); err != nil {
		return err
	}

	out, err := jsonpath.Get(expr, in)
	if err != nil {
		return err
	}

	switch v := out.(type) {
	case string:
		_, err = io.WriteString(w, v)
		return err
	default:
		enc := json.NewEncoder(w)
		return enc.Encode(v)
	}
}

func rowsFromData(data any) ([]string, []map[string]string, error) {
	v := reflect.ValueOf(data)
	if !v.IsValid() {
		return nil, nil, fmt.Errorf("invalid data")
	}

	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, nil, fmt.Errorf("nil data")
		}
		v = v.Elem()
	}

	if v.Kind() == reflect.Slice {
		if v.Len() == 0 {
			return []string{}, []map[string]string{}, nil
		}
		headers := map[string]struct{}{}
		rows := make([]map[string]string, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			row, err := toStringMap(v.Index(i))
			if err != nil {
				return nil, nil, err
			}
			for k := range row {
				headers[k] = struct{}{}
			}
			rows = append(rows, row)
		}
		headerList := mapKeys(headers)
		normalized := make([]map[string]string, 0, len(rows))
		for _, r := range rows {
			normalized = append(normalized, normalizeRow(r, headerList))
		}
		return headerList, normalized, nil
	}

	row, err := toStringMap(v)
	if err != nil {
		return nil, nil, err
	}
	headers := mapKeys(mapFromStringMap(row))
	return headers, []map[string]string{normalizeRow(row, headers)}, nil
}

func toStringMap(v reflect.Value) (map[string]string, error) {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, fmt.Errorf("nil row")
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Map:
		out := map[string]string{}
		iter := v.MapRange()
		for iter.Next() {
			k := fmt.Sprint(iter.Key().Interface())
			out[k] = stringify(iter.Value().Interface())
		}
		return out, nil
	case reflect.Struct:
		out := map[string]string{}
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}
			name := field.Tag.Get("json")
			if name == "" {
				name = strings.ToLower(field.Name)
			} else {
				name = strings.Split(name, ",")[0]
			}
			if name == "" || name == "-" {
				continue
			}
			out[name] = stringify(v.Field(i).Interface())
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported data type %s", v.Kind())
	}
}

func stringify(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case fmt.Stringer:
		return val.String()
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprint(val)
		}
		if len(b) == 0 {
			return ""
		}
		if b[0] == '"' {
			var s string
			if unmarshalErr := json.Unmarshal(b, &s); unmarshalErr == nil {
				return s
			}
		}
		return string(b)
	}
}

func mapKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func normalizeRow(in map[string]string, headers []string) map[string]string {
	out := make(map[string]string, len(headers))
	for _, h := range headers {
		out[h] = in[h]
	}
	return out
}

func mapFromStringMap(in map[string]string) map[string]struct{} {
	out := map[string]struct{}{}
	for k := range in {
		out[k] = struct{}{}
	}
	return out
}

func RenderToString(data any, opts Options) (string, error) {
	var buf bytes.Buffer
	if err := Render(&buf, data, opts); err != nil {
		return "", err
	}
	return buf.String(), nil
}
