package rdf

import (
	"fmt"
	"io"
	"runtime"
)

// TripleDecoder parses RDF documents (serializations of an RDF graph).
//
// For streaming parsing, use the Decode() method to decode a single Triple
// at a time. Or, if you want to read the whole document in one go, use DecodeAll().
type TripleDecoder interface {
	// Decode should parse a RDF document and return the next valid triple.
	// It should return io.EOF when the whole document is parsed.
	Decode() (Triple, error)

	// DecodeAll should parse the entire RDF document and return all valid
	// triples, or an error.
	DecodeAll() ([]Triple, error)

	// SetBase sets the base IRI which will be used for resolving relative IRIs.
	// For formats that doesn't allow relative IRIs (N-Triples), this is a no-op.
	// TODO strip #fragment in implementations? - check w3.org spec.
	SetBase(IRI)
}

// NewTripleDecoder returns a new TripleDecoder capable of parsing triples
// from the given io.Reader in the given serialization format.
func NewTripleDecoder(r io.Reader, f Format) TripleDecoder {
	switch f {
	case FormatNT:
		return newNTDecoder(r)
	case FormatRDFXML:
		return newRDFXMLDecoder(r)
	case FormatTTL:
		return newTTLDecoder(r)
	default:
		panic(fmt.Errorf("Decoder for serialization format %s not implemented", f))
	}
}

// QuadDecoder parses RDF quads in one of the following formats:
// N-Quads.
//
// For streaming parsing, use the Decode() method to decode a single Quad
// at a time. Or, if you want to read the whole source in one go, DecodeAll().
type QuadDecoder struct {
	l      *lexer
	format Format

	DefaultGraph Context  // default graph
	tokens       [3]token // 3 token lookahead
	peekCount    int      // number of tokens peeked at (position in tokens lookahead array)
}

// NewQuadDecoder returns a new QuadDecoder capable of parsing quads
// from the given io.Reader in the given serialization format.
func NewQuadDecoder(r io.Reader, f Format) *QuadDecoder {
	return &QuadDecoder{
		l:            newLineLexer(r),
		format:       f,
		DefaultGraph: Blank{id: "_:defaultGraph"},
	}
}

// Decode returns the next valid Quad, or an error
func (d *QuadDecoder) Decode() (Quad, error) {
	return d.parseNQ()
}

// DecodeAll decodes and returns all Quads from source, or an error
func (d *QuadDecoder) DecodeAll() ([]Quad, error) {
	var qs []Quad
	for q, err := d.Decode(); err != io.EOF; q, err = d.Decode() {
		if err != nil {
			return nil, err
		}
		qs = append(qs, q)
	}
	return qs, nil
}

// next returns the next token.
func (d *QuadDecoder) next() token {
	if d.peekCount > 0 {
		d.peekCount--
	} else {
		d.tokens[0] = d.l.nextToken()
	}

	return d.tokens[d.peekCount]
}

// peek returns but does not consume the next token.
func (d *QuadDecoder) peek() token {
	if d.peekCount > 0 {
		return d.tokens[d.peekCount-1]
	}
	d.peekCount = 1
	d.tokens[0] = d.l.nextToken()
	return d.tokens[0]
}

// recover catches non-runtime panics and binds the panic error
// to the given error pointer.
func (d *QuadDecoder) recover(errp *error) {
	e := recover()
	if e != nil {
		if _, ok := e.(runtime.Error); ok {
			// Don't recover from runtime errors.
			panic(e)
		}
		//d.stop() something to clean up?
		*errp = e.(error)
	}
	return
}

// expect1As consumes the next token and guarantees that it has the expected type.
func (d *QuadDecoder) expect1As(context string, expected tokenType) token {
	t := d.next()
	if t.typ != expected {
		if t.typ == tokenError {
			d.errorf("%d:%d: syntax error: %s", t.line, t.col, t.text)
		} else {
			d.unexpected(t, context)
		}
	}
	return t
}

// expectAs consumes the next token and guarantees that it has the one of the expected types.
func (d *QuadDecoder) expectAs(context string, expected ...tokenType) token {
	t := d.next()
	for _, e := range expected {
		if t.typ == e {
			return t
		}
	}
	if t.typ == tokenError {
		d.errorf("%d:%d: syntax error: %v", t.line, t.col, t.text)
	} else {
		d.unexpected(t, context)
	}
	return t
}

// errorf formats the error and terminates parsing.
func (d *QuadDecoder) errorf(format string, args ...interface{}) {
	format = fmt.Sprintf("%s", format)
	panic(fmt.Errorf(format, args...))
}

// unexpected complains about the given token and terminates parsing.
func (d *QuadDecoder) unexpected(t token, context string) {
	d.errorf("%d:%d unexpected %v as %s", t.line, t.col, t.typ, context)
}
