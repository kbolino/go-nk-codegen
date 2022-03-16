package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/iancoleman/strcase"
)

func main() {
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func run() error {
	patterns, err := parsePatterns(*flagPatterns)
	if err != nil {
		return fmt.Errorf("parsing patterns in file '%s': %w", *flagPatterns, err)
	}
	done, err := parseDone(*flagDone)
	if err != nil {
		return fmt.Errorf("parsing done signatures in file '%s': %w", *flagDone, err)
	}
	typeMap, err := parseTypeMap(*flagTypemap)
	if err != nil {
		return fmt.Errorf("parsing typemap in file '%s': %w", *flagTypemap, err)
	}
	funcs, err := parseCFuncs(*flagHeader, func(name string) bool {
		skip := true
		for _, pattern := range patterns {
			if match := pattern.Regexp.FindString(name); match != name {
				continue
			}
			if pattern.Negate {
				debugf("skipping function %s because of negated pattern %s", name, pattern.Regexp)
				skip = true
			} else {
				debugf("including function %s because of pattern %s", name, pattern.Regexp)
				skip = false
			}
		}
		return skip
	})
	if err != nil {
		return fmt.Errorf("parsing C functions in file '%s': %w", *flagHeader, err)
	}
	fmt.Println("package", *flagPackage)
	fmt.Println()
	fmt.Println("// GENERATED CODE -- DO NOT EDIT")
	fmt.Println()
	fmt.Println(`// #include "nk.h"`)
	fmt.Println("// #include <stdlib.h>")
	fmt.Println(`import "C"`)
	fmt.Println()
	fmt.Println(`import "unsafe"`)
	for _, f := range funcs {
		if err := printFunc(typeMap, done, &f, "", false); err != nil {
			return fmt.Errorf("printing definition of function %s: %w", f.Name, err)
		}
	}
	return nil
}

func printFunc(typeMap map[string]TypeConv, done map[string]struct{}, f *FunctionDecl, doc string, stringToBytes bool) error {
	nakedName := strings.TrimPrefix(f.Name, "nk_")
	goFuncName := strcase.ToCamel(nakedName)
	if stringToBytes {
		goFuncName = goFuncName + "Bytes"
	}
	method := true
	goParamOffset := 1
	if len(f.Params) == 0 || f.Params[0].Type != "struct nk_context *" {
		method = false
		debugf("marking function %s as not a method because its first parameter is not typed 'struct nk_context *'",
			f.Name)
		goParamOffset = 0
	}
	goParamTypes := make([]string, len(f.Params)-goParamOffset)
	goParams := make([]string, len(f.Params)-goParamOffset)
	cParams := make([]string, len(f.Params))
	goNameCounts := make(map[string]int)
	hasCStringParams := false
	var preamble strings.Builder
	for i := goParamOffset; i < len(f.Params); i++ {
		// convert type
		cParamIndex := i
		goParamIndex := i - goParamOffset
		cParam := f.Params[cParamIndex]
		goType, cgoType, err := convertType(typeMap, cParam.Type, ConvertTypeDefault)
		if err != nil {
			return fmt.Errorf("converting type '%s' of parameter %d: %w", cParam.Type, i, err)
		} else if goType == "" {
			return fmt.Errorf("no type mapped for parameter %d", i)
		}
		// infer and validate parameter name
		goName := strcase.ToLowerCamel(cParam.Name)
		if goName == "" {
			semanticType := goType
			for strings.HasPrefix(semanticType, "*") {
				semanticType = strings.TrimPrefix(semanticType, "*")
			}
			goName = strcase.ToLowerCamel(semanticType)
		}
		switch goName {
		case "string":
			goName = "s"
		case "int8", "int16", "int32", "int64", "uint8", "uint16", "uint32", "uint64", "int", "uint":
			goName = "n"
		case "bool":
			goName = "b"
		case "float32", "float64":
			goName = "x"
		case "len":
			goName = "length"
		case "cap":
			goName = "capacity"
		case "copy":
			goName = "cpy"
		case "make":
			goName = "mk"
		case "new":
			goName = "nw"
		}
		goNameCounts[goName]++
		if nameCount := goNameCounts[goName]; nameCount > 1 {
			goName = fmt.Sprintf("%s%d", goName, nameCount)
		}
		// check for CStrings
		if cgoType == "C.CString" {
			if stringToBytes {
				goType = "[]byte"
				cParams[cParamIndex] = fmt.Sprintf("(*C.char)(unsafe.Pointer(&%s[0]))", goName)
			} else {
				rawName := fmt.Sprintf("raw%s", strcase.ToCamel(goName))
				fmt.Fprintf(&preamble, "\t%s := C.CString(%s)\n", rawName, goName)
				fmt.Fprintf(&preamble, "\tdefer C.free(unsafe.Pointer(%s))\n", rawName)
				hasCStringParams = true
				cParams[cParamIndex] = rawName
			}
			// skip over the next param in the normal loop
			i++
			nextCParamIndex := i
			nextGoParamIndex := i - goParamOffset
			// ensure it is an int
			if nextCParamIndex >= len(f.Params) || f.Params[nextCParamIndex].Type != "int" {
				return fmt.Errorf("string parameter %d is not followed by length param", i)
			}
			// synthesize a C parameter set to the string length
			cParams[nextCParamIndex] = fmt.Sprintf("C.int(len(%s))", goName)
			// put a sentinel value in for the Go parameter
			goParams[nextGoParamIndex] = "__DELETED__"
		} else if len(cgoType) == 0 {
			cParams[cParamIndex] = goName
		} else if goType == "Handle" {
			cParams[cParamIndex] = fmt.Sprintf("%s.raw()", goName)
		} else {
			var paramFormat string
			if strings.HasPrefix(cgoType, "*C.struct_") {
				paramFormat = "(%s)(unsafe.Pointer(%s))"
			} else if strings.HasPrefix(cgoType, "C.struct_") {
				paramFormat = "*(*%s)(unsafe.Pointer(&%s))"
			} else {
				paramFormat = "(%s)(%s)"
			}
			cParams[cParamIndex] = fmt.Sprintf(paramFormat, cgoType, goName)
		}
		goParamTypes[goParamIndex] = goType
		goParams[goParamIndex] = fmt.Sprintf("%s %s", goName, goType)
	}
	for i, p := range goParams {
		switch p {
		// delete parameters which aren't needed after CString handling
		case "__DELETED__":
			goParams = append(goParams[:i], goParams[i+1:]...)
			goParamTypes = append(goParamTypes[:i], goParamTypes[i+1:]...)
		case "":
			return fmt.Errorf("parameter %d assigned no name", i)
		}
	}
	if method {
		cParams[0] = "ctx.raw()"
	}
	retType, _, err := convertType(typeMap, f.Return, ConvertTypeDefault)
	if err != nil {
		return fmt.Errorf("converting type '%s' of return: %w", f.Return, err)
	}
	anonMethodReceiver := ""
	if method {
		anonMethodReceiver = "(*Context) "
	}
	paramTypeList := strings.Join(goParamTypes, ", ")
	signature := ""
	if retType == "" {
		signature = fmt.Sprintf("func %s%s(%s)", anonMethodReceiver, goFuncName, paramTypeList)
	} else {
		signature = fmt.Sprintf("func %s%s(%s) %s", anonMethodReceiver, goFuncName, paramTypeList, retType)
	}
	if _, ok := done[signature]; ok {
		debugf("skipping function %s because it matches done signature '%s'", goFuncName, signature)
		return nil
	}
	namedMethodReceiver := ""
	if method {
		namedMethodReceiver = "(ctx *Context) "
	}
	paramList := strings.Join(goParams, ", ")
	castList := strings.Join(cParams, ", ")
	fmt.Println()
	if doc == "" {
		doc = fmt.Sprintf("%s calls %s.", goFuncName, f.Name)
	}
	fmt.Println("//", doc)
	if retType == "" {
		fmt.Printf("func %s%s(%s) {\n", namedMethodReceiver, goFuncName, paramList)
		fmt.Print(preamble.String())
		fmt.Printf("\tC.%s(%s)\n", f.Name, castList)
	} else {
		fmt.Printf("func %s%s(%s) %s {\n", namedMethodReceiver, goFuncName, paramList, retType)
		fmt.Print(preamble.String())
		if retType[0] >= 'a' && retType[0] <= 'z' {
			fmt.Printf("\treturn (%s)(C.%s(%s))\n", retType, f.Name, castList)
		} else {
			fmt.Printf("\t_retval := C.%s(%s)\n", f.Name, castList)
			if strings.HasPrefix(retType, "*") {
				return fmt.Errorf("pointer return")
			} else {
				fmt.Printf("\treturn *(*%s)(unsafe.Pointer(&_retval))\n", retType)
			}
		}
	}
	fmt.Println("}")
	if !stringToBytes && hasCStringParams {
		bytesDoc := fmt.Sprintf("%sBytes is like %s except that it does not copy string parameters.",
			goFuncName, goFuncName)
		printFunc(typeMap, done, f, bytesDoc, true)
	}
	return nil
}
