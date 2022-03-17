package main

import "flag"

var (
	flagCPP   = flag.String("cpp", "cpp", "path to the C preprocessor")
	flagDebug = flag.Bool("debug", false, "enable debug logging")
	flagEnums = flag.String("enums", "enums.txt", "path to file containing regexps to match againsg C enums; same "+
		"syntax as -funcs")
	flagFuncs = flag.String("funcs", "funcs.txt", "path to file containing regexps to match against C function "+
		"names, one per line; empty lines ignored, comment lines start with #, and negated lines with !; patterns "+
		"must match entire function name")
	flagHeader  = flag.String("header", "nk.h", "path to nk.h header")
	flagInclude = flag.String("include", "", "append to include path")
	flagPackage = flag.String("package", "nk", "package name; short name, not full path")
	flagTypemap = flag.String("typemap", "typemap.csv", "path to file containing type mappings from C to Go and cgo; "+
		"one mapping per line; CSV format 'ctype,gotype,cgotype'; empty lines ignored, comment lines start with #")
)
