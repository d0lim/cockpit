package sqlport

import (
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
)

var readVerbs = map[string]bool{
	"select": true, "with": true, "show": true, "explain": true,
	"describe": true, "desc": true, "table": true, "values": true,
}

type statementError struct {
	denied bool
	reason string
}

type scanner struct {
	src       string
	driver    contract.DBDriver
	backslash bool
	words     []string
	end       int
	err       *statementError
}

func checkStatement(driver contract.DBDriver, stmt string) (string, *statementError) {
	var kept string
	for _, backslash := range []bool{false, true} {
		s := &scanner{src: stmt, driver: driver, backslash: backslash, end: len(stmt)}
		s.run()
		if s.err != nil {
			return "", s.err
		}
		if err := s.verdict(); err != nil {
			return "", err
		}
		if !backslash {
			kept = stmt[:s.end]
		} else if s.end != len(kept) {
			return "", &statementError{denied: true, reason: "the statement ends at a different place depending on backslash escapes"}
		}
	}
	return kept, nil
}

func (s *scanner) verdict() *statementError {
	if len(s.words) == 0 {
		return &statementError{reason: "the statement is empty"}
	}
	if !readVerbs[s.words[0]] {
		return &statementError{denied: true, reason: "only read statements are accepted (select, with, show, explain, describe, table, values)"}
	}
	if s.driver == contract.DriverMySQL {
		for i := 0; i+1 < len(s.words); i++ {
			if s.words[i] == "into" && (s.words[i+1] == "outfile" || s.words[i+1] == "dumpfile") {
				return &statementError{denied: true, reason: "SELECT ... INTO OUTFILE and INTO DUMPFILE are not accepted"}
			}
		}
	}
	return nil
}

func (s *scanner) run() {
	i, ended := 0, false
	for i < len(s.src) && s.err == nil {
		c := s.src[i]
		next := i + 1
		switch {
		case isSpace(c):
		case c == ';':
			if !ended {
				s.end = i
				ended = true
			}
		case s.lineComment(i):
			next = s.skipLine(i)
		case strings.HasPrefix(s.src[i:], "/*"):
			next = s.skipBlock(i)
		case ended:
			s.err = &statementError{denied: true, reason: "only one statement is accepted"}
		case c == '\'' || c == '"' || c == '`':
			next = s.skipQuoted(i, c)
		case c == '$' && s.driver == contract.DriverPostgres:
			next = s.skipDollar(i)
		case isWordByte(c):
			next = i
			for next < len(s.src) && isWordByte(s.src[next]) {
				next++
			}
			s.words = append(s.words, strings.ToLower(s.src[i:next]))
		}
		i = next
	}
}

func (s *scanner) lineComment(i int) bool {
	if s.driver == contract.DriverMySQL && s.src[i] == '#' {
		return true
	}
	if !strings.HasPrefix(s.src[i:], "--") {
		return false
	}
	if s.driver == contract.DriverPostgres {
		return true
	}
	return i+2 == len(s.src) || isSpace(s.src[i+2])
}

func (s *scanner) skipLine(i int) int {
	for i < len(s.src) && s.src[i] != '\n' {
		i++
	}
	return i
}

func (s *scanner) skipBlock(i int) int {
	if s.driver == contract.DriverMySQL && strings.HasPrefix(s.src[i:], "/*!") {
		s.err = &statementError{denied: true, reason: "executable comments (/*! ... */) are not accepted"}
		return len(s.src)
	}
	depth := 0
	for j := i; j+1 < len(s.src); j++ {
		switch {
		case s.src[j] == '/' && s.src[j+1] == '*':
			if depth == 0 || s.driver == contract.DriverPostgres {
				depth++
			}
			j++
		case s.src[j] == '*' && s.src[j+1] == '/':
			depth--
			j++
			if depth == 0 {
				return j + 1
			}
		}
	}
	s.err = &statementError{reason: "a block comment is not closed"}
	return len(s.src)
}

func (s *scanner) skipQuoted(i int, q byte) int {
	escapes := s.backslash && (s.driver == contract.DriverMySQL && q != '`' || s.driver == contract.DriverPostgres && q == '\'')
	for j := i + 1; j < len(s.src); j++ {
		switch {
		case escapes && s.src[j] == '\\':
			j++
		case s.src[j] == q && j+1 < len(s.src) && s.src[j+1] == q:
			j++
		case s.src[j] == q:
			return j + 1
		}
	}
	s.err = &statementError{reason: "a quoted string or identifier is not closed"}
	return len(s.src)
}

func (s *scanner) skipDollar(i int) int {
	j := i + 1
	if j < len(s.src) && isDigit(s.src[j]) {
		for j < len(s.src) && isDigit(s.src[j]) {
			j++
		}
		return j
	}
	for j < len(s.src) && isWordByte(s.src[j]) && s.src[j] != '$' {
		j++
	}
	if j >= len(s.src) || s.src[j] != '$' {
		return i + 1
	}
	tag := s.src[i : j+1]
	if k := strings.Index(s.src[j+1:], tag); k >= 0 {
		return j + 1 + k + len(tag)
	}
	s.err = &statementError{reason: "a dollar-quoted string is not closed"}
	return len(s.src)
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isWordByte(c byte) bool {
	return c == '_' || c == '$' || isDigit(c) || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}
