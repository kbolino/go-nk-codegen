package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"regexp"
)

const (
	attrsPrefix = "#attrs: "
	maxAttrs    = 32
)

var attrsPrefixBytes = []byte(attrsPrefix)

type Pattern struct {
	Regexp *regexp.Regexp
	Negate bool
	Attrs  map[string]string
}

func parsePatterns(fileName string) ([]Pattern, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	lineNum := 0
	var patterns []Pattern
	var attrs map[string]string
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		line = bytes.TrimSpace(line)
		negate := false
		if len(line) == 0 {
			continue
		} else if line[0] == '#' {
			if !bytes.HasPrefix(line, attrsPrefixBytes) {
				continue
			}
			line = bytes.TrimSpace(bytes.TrimPrefix(line, attrsPrefixBytes))
			if len(line) == 0 {
				attrs = nil
				continue
			}
			attrStrs := bytes.SplitN(line, []byte{','}, maxAttrs+1)
			if len(attrStrs) > maxAttrs {
				return nil, fmt.Errorf("too many attributes specified on line %d", lineNum)
			}
			attrs = make(map[string]string, len(attrStrs))
			for _, attrStr := range attrStrs {
				parts := bytes.SplitN(attrStr, []byte{'='}, 2)
				key := string(bytes.TrimSpace(parts[0]))
				value := ""
				if len(parts) > 1 {
					value = string(bytes.TrimSpace(parts[1]))
				}
				attrs[key] = value
			}
		} else if line[0] == '!' {
			negate = true
			line = bytes.TrimSpace(line[1:])
		}
		re, err := regexp.Compile(string(line))
		if err != nil {
			return nil, fmt.Errorf("compiling line %d: %w", lineNum, err)
		}
		patterns = append(patterns, Pattern{
			Regexp: re,
			Negate: negate,
			Attrs:  attrs,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}
	return patterns, nil
}
