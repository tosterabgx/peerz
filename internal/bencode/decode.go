package bencode

import (
	"errors"
	"fmt"
	"strconv"
)

var ErrUnexpectedEOF = errors.New("bencode: unexpected end of input")

type decoder struct {
	data  []byte
	pos   int
	depth int
	info  []byte
}

func (d *decoder) peek() (byte, error) {
	if d.pos >= len(d.data) {
		return 0, ErrUnexpectedEOF
	}
	return d.data[d.pos], nil
}

func (d *decoder) value() (any, error) {
	c, err := d.peek()
	if err != nil {
		return nil, err
	}
	switch {
	case c == 'i':
		return d.integer()
	case c == 'l':
		return d.list()
	case c == 'd':
		return d.dict()
	case c >= '0' && c <= '9':
		return d.str()
	default:
		return nil, fmt.Errorf("bencode: unexpected byte %q at offset %d", c, d.pos)
	}
}

func (d *decoder) readUntil(delim byte) ([]byte, error) {
	start := d.pos
	for d.pos < len(d.data) {
		if d.data[d.pos] == delim {
			s := d.data[start:d.pos]
			d.pos++
			return s, nil
		}
		d.pos++
	}
	return nil, ErrUnexpectedEOF
}

func (d *decoder) integer() (int64, error) {
	d.pos++
	raw, err := d.readUntil('e')
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(string(raw), 10, 64)
}

func (d *decoder) str() (string, error) {
	raw, err := d.readUntil(':')
	if err != nil {
		return "", err
	}
	n, err := strconv.Atoi(string(raw))
	if err != nil || n < 0 {
		return "", fmt.Errorf("bencode: bad string length %q", raw)
	}
	if d.pos+n > len(d.data) {
		return "", ErrUnexpectedEOF
	}
	if n > len(d.data)-d.pos {
		return "", ErrUnexpectedEOF
	}
	s := string(d.data[d.pos : d.pos+n])
	d.pos += n
	return s, nil
}

func (d *decoder) list() ([]any, error) {
	d.pos++
	d.depth++
	defer func() { d.depth-- }()

	var out []any
	for {
		c, err := d.peek()
		if err != nil {
			return nil, err
		}
		if c == 'e' {
			d.pos++
			return out, nil
		}
		v, err := d.value()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
}

func (d *decoder) dict() (map[string]any, error) {
	d.pos++
	d.depth++
	defer func() { d.depth-- }()

	out := make(map[string]any)
	for {
		c, err := d.peek()
		if err != nil {
			return nil, err
		}
		if c == 'e' {
			d.pos++
			return out, nil
		}
		key, err := d.str()
		if err != nil {
			return nil, err
		}
		start := d.pos
		v, err := d.value()
		if err != nil {
			return nil, err
		}
		if d.depth == 1 && key == "info" {
			d.info = d.data[start:d.pos]
		}
		out[key] = v
	}
}

func Decode(data []byte) (any, []byte, error) {
	d := &decoder{data: data}
	v, err := d.value()
	if err != nil {
		return nil, nil, err
	}
	if d.pos != len(d.data) {
		return nil, nil, fmt.Errorf("bencode: trailing data at offset %d", d.pos)
	}
	return v, d.info, nil
}
