package main

import "flag"

var (
	flagCPP   = flag.String("cpp", "cpp", "path to the C preprocessor")
	flagDebug = flag.Bool("debug", false, "enable debug logging")
	flagDone  = flag.String("done", "done.txt", "path to file containing names of Go function signatures that have "+
		"already been migrated; empty lines ignored, comment lines start with #; no parameter names, newlines, "+
		"trailing commas, or return names")
	flagHeader   = flag.String("header", "nk.h", "path to nk.h header")
	flagInclude  = flag.String("include", "", "append to include path")
	flagPatterns = flag.String("patterns", "patterns.txt", "path to file containing regexps to match against C "+
		"function names, one per line; empty lines ignored, comment lines start with #, and negated lines with !; "+
		"patterns implicitly start with ^ and end with $, use .* to unanchor them")
	flagTypemap = flag.String("typemap", "typemap.csv", "path to file containing type mappings from C to Go and cgo; "+
		"one mapping per line; CSV format 'ctype,gotype,cgotype'; empty lines ignored, comment lines start with #")
)
