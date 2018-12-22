// Package section is responsible for splitting a program into its different
// sections without attempting to parse the contents.
package section

import (
	"bytes"
	"fmt"
	"go/token"
	"strings"
	"unicode"

	"go.uber.org/multierr"
)

// Program is a single .patch file consisting of one or more changes.
type Program []*Change

// Change is a single change in a program.
type Change struct {
	// Position at which the first @ of the header occurs.
	HeaderPos token.Pos

	// Changes can optionally have a name.
	//
	// If any, it is specified between the first pair of @@s in the change.
	Name string

	// Metavariables section of the change.
	Meta Section

	// Position of the second "@@".
	AtPos token.Pos

	// Patch is the patch section of the change. This is the code after the
	// second @@.
	Patch Section
}

// Pos returns the position at which this change begins.
func (c *Change) Pos() token.Pos { return c.HeaderPos }

// End returns the position of the first character after this change.
func (c *Change) End() token.Pos {
	if len(c.Patch) > 0 {
		return c.Patch[len(c.Patch)-1].End()
	}

	// An emty change is effectively a no-op but that's not relevant here.
	// The End position for an empty change is when the second pair of "@@"s
	// ends.
	return c.AtPos + 2
}

// Section is a section of the change.
type Section []*Line

// Line is a single line from the patch.
type Line struct {
	// Position at which this line begins.
	StartPos token.Pos

	// Contents of the line.
	Text []byte
}

// Pos returns the position at which this line begins.
func (l *Line) Pos() token.Pos { return l.StartPos }

// End returns the position of the character just past this line.
func (l *Line) End() token.Pos { return l.StartPos + token.Pos(len(l.Text)) }

// Split splits a Program into sections.
func Split(fset *token.FileSet, filename string, content []byte) (Program, error) {
	file := fset.AddFile(filename, -1, len(content))
	file.SetLinesForContent(content)

	splitter := programSplitter{file: file, content: content}
	splitter.next() // read the first line

	return splitter.readProgram(), multierr.Combine(splitter.errors...)
}

type programSplitter struct {
	file    *token.File // file to feed newline information
	content []byte      // raw source

	text []byte    // contents of the current line
	pos  token.Pos // position at which text begins

	eof bool // whether we've reached EOF

	startOffset int // offset at the start of the current line
	offset      int // current position in content

	errors []error
}

// Posts an error message with positional information.
func (p *programSplitter) errf(off int, msg string, args ...interface{}) {
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}

	pos := p.file.Pos(off)
	p.errors = append(p.errors, fmt.Errorf("%v: %v", p.file.Position(pos), msg))
}

// Skips to the end of line. This may be a newline character or EOF.
func (p *programSplitter) skipUntilEOL() {
	for ; p.offset < len(p.content); p.offset++ {
		if p.content[p.offset] == '\n' {
			return
		}
	}
}

// Advances the scanner to the next non-comment line.
func (p *programSplitter) next() {
	for ; p.offset < len(p.content); p.offset++ {
		p.startOffset = p.offset
		p.skipUntilEOL()

		p.text = p.content[p.startOffset:p.offset]
		p.pos = p.file.Pos(p.startOffset)
		if !isComment(p.text) {
			p.offset++
			return
		}
	}

	// Reached EOF.
	p.pos = token.NoPos
	p.text = nil
	p.eof = true
	return
}

// Comments are supported only on their own lines.
func isComment(s []byte) bool {
	s = bytes.TrimLeftFunc(s, unicode.IsSpace)
	return len(s) > 0 && s[0] == '#'
}

func (p *programSplitter) readProgram() Program {
	var prog Program
	for !p.eof {
		prog = append(prog, p.readChange())
	}
	if len(prog) == 0 {
		p.errf(p.offset, "unexpected EOF, at least one change is required")
	}
	return prog
}

// Read and return a Change, or nil if EOF was reached.
func (p *programSplitter) readChange() *Change {
	// Can't use a struct literal here because readName and readMeta advance
	// p.pos between HeaderPos and AtPos.
	var c Change
	c.HeaderPos = p.pos
	c.Name = p.readName()
	c.Meta = p.readMeta()
	c.AtPos = p.pos
	c.Patch = p.readPatch()
	return &c
}

// Reads the name of a change.
func (p *programSplitter) readName() string {
	text := string(p.text)
	defer p.next()

	switch {
	case text == "@@":
		// unnamed
		return ""
	case len(text) > 2 && text[0] == '@' && text[len(text)-1] == '@':
		// named
		name := text[1:]          // leading @
		name = name[:len(name)-1] // trailing @
		return strings.TrimSpace(name)
		// TODO(abg): check if valid identifier
	default:
		p.errf(p.startOffset, `unexpected %q, expected "@@" or "@ change_name @"`, text)
		return ""
	}
}

// Reads the metavariables section of the change.
func (p *programSplitter) readMeta() Section {
	var s Section
	for ; !p.eof; p.next() {
		if len(p.text) == 2 && p.text[0] == '@' && p.text[1] == '@' {
			return s
		}
		s = append(s, &Line{StartPos: p.pos, Text: p.text})
	}

	p.errf(p.offset, `unexpected EOF, expected "@@"`)
	return nil
}

// Reads the patch section of a change, stopping when a new change is
// encountered or the end of the file is reached.
func (p *programSplitter) readPatch() Section {
	p.next() // skip past "@@" marking the end of metavariables section

	var s Section
	for ; !p.eof; p.next() {
		if len(p.text) > 0 && p.text[0] == '@' {
			// new change begins
			break
		}
		s = append(s, &Line{StartPos: p.pos, Text: p.text})
	}
	return s
}