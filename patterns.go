package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"regexp"
)

type Pattern struct {
	Regexp *regexp.Regexp
	Negate bool
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
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		line = bytes.TrimSpace(line)
		negate := false
		if len(line) == 0 || line[0] == '#' {
			continue
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
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}
	return patterns, nil
}
