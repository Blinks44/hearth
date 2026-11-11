package ops

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// jsonMember is a key of a JSON object and where it and its value are in
// the text.
type jsonMember struct {
	key                     string
	keyStart, valStart, end int
}

// jsonObject reads the object that starts at text[start], a '{': its
// members and the index just after its '}'.
func jsonObject(text string, start int) ([]jsonMember, int, error) {
	dec := json.NewDecoder(strings.NewReader(text[start:]))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, 0, errors.New("not a JSON object")
	}
	var out []jsonMember
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, 0, fmt.Errorf("read JSON: %w", err)
		}
		key, _ := tok.(string)
		quoted, _ := json.Marshal(key)
		keyEnd := start + int(dec.InputOffset())
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, 0, fmt.Errorf("read JSON: %w", err)
		}
		end := start + int(dec.InputOffset())
		out = append(out, jsonMember{key: key, keyStart: keyEnd - len(quoted), valStart: end - len(v), end: end})
	}
	if _, err := dec.Token(); err != nil {
		return nil, 0, fmt.Errorf("read JSON: %w", err)
	}
	return out, start + int(dec.InputOffset()), nil
}

// lineIndent is the blanks that start the line at, which i is in.
func lineIndent(text string, i int) string {
	line := text[strings.LastIndex(text[:i], "\n")+1:]
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// indentJSON is raw laid out over lines, the lines after the first
// starting with indent.
func indentJSON(raw json.RawMessage, indent string) (string, error) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, indent, "  "); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// JSONSet returns text, a JSON object, with the value at path set to raw:
// in place where it is, a new member at the end of its object where it is
// not, the objects on the way made where they are missing. The rest of the
// text stays as it is.
func JSONSet(text string, path []string, raw json.RawMessage) (string, error) {
	start := strings.Index(text, "{")
	if start < 0 {
		return "", errors.New("not a JSON object")
	}
	for i, key := range path {
		members, end, err := jsonObject(text, start)
		if err != nil {
			return "", err
		}
		var m *jsonMember
		for j := range members {
			if members[j].key == key {
				m = &members[j]
			}
		}
		if m != nil && i < len(path)-1 {
			if text[m.valStart] != '{' {
				return "", fmt.Errorf("%s is not an object", key)
			}
			start = m.valStart
			continue
		}
		// What to put: raw, in the objects of the rest of the path.
		value := raw
		for k := len(path) - 1; k > i; k-- {
			name, _ := json.Marshal(path[k])
			value = json.RawMessage(`{` + string(name) + `:` + string(value) + `}`)
		}
		if m != nil {
			v, err := indentJSON(value, lineIndent(text, m.keyStart))
			if err != nil {
				return "", err
			}
			return text[:m.valStart] + v + text[m.end:], nil
		}
		name, _ := json.Marshal(key)
		if len(members) == 0 {
			outer := lineIndent(text, start)
			v, err := indentJSON(value, outer+"  ")
			if err != nil {
				return "", err
			}
			return text[:start] + "{\n" + outer + "  " + string(name) + ": " + v + "\n" + outer + "}" + text[end:], nil
		}
		last := members[len(members)-1]
		indent := lineIndent(text, last.keyStart)
		v, err := indentJSON(value, indent)
		if err != nil {
			return "", err
		}
		return text[:last.end] + ",\n" + indent + string(name) + ": " + v + text[last.end:], nil
	}
	return "", errors.New("empty path")
}

// JSONDelete returns text, a JSON object, without the member at path,
// and with it the comma and line it took; text as it is when there is
// none.
func JSONDelete(text string, path []string) (string, error) {
	start := strings.Index(text, "{")
	if start < 0 {
		return "", errors.New("not a JSON object")
	}
	for i, key := range path {
		members, end, err := jsonObject(text, start)
		if err != nil {
			return "", err
		}
		j := -1
		for k := range members {
			if members[k].key == key {
				j = k
			}
		}
		if j < 0 {
			return text, nil
		}
		m := members[j]
		if i < len(path)-1 {
			if text[m.valStart] != '{' {
				return text, nil
			}
			start = m.valStart
			continue
		}
		switch {
		case len(members) == 1:
			return text[:start] + "{}" + text[end:], nil
		case j < len(members)-1:
			return text[:m.keyStart] + text[members[j+1].keyStart:], nil
		default:
			return text[:members[j-1].end] + text[m.end:], nil
		}
	}
	return text, nil
}
