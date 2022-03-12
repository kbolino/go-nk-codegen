package main

import (
	"fmt"
	"os"
)

func debug(args ...interface{}) {
	if *flagDebug {
		fmt.Fprint(os.Stderr, "DEBUG: ")
		fmt.Fprintln(os.Stderr, args...)
	}
}

func debugf(format string, args ...interface{}) {
	if *flagDebug {
		fmt.Fprintf(os.Stderr, "DEBUG: %s\n", fmt.Sprintf(format, args...))
	}
}
