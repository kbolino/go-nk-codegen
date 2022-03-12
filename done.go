package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
)

func parseDone(fileName string) (map[string]struct{}, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	lineNum := 0
	done := make(map[string]struct{})
	for scanner.Scan() {
		lineNum++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		done[string(line)] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}
	return done, nil
}
