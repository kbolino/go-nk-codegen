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
	enumPatterns, err := parsePatterns(*flagEnums)
	if err != nil {
		return fmt.Errorf("parsing enum patterns in file '%s': %w", *flagEnums, err)
	}
	funcPatterns, err := parsePatterns(*flagFuncs)
	if err != nil {
		return fmt.Errorf("parsing function patterns in file '%s': %w", *flagFuncs, err)
	}
	typeMap, err := parseTypeMap(*flagTypemap)
	if err != nil {
		return fmt.Errorf("parsing typemap in file '%s': %w", *flagTypemap, err)
	}
	parser := NewParser(NewPatternMatcher(enumPatterns, funcPatterns, nil))
	result, err := parser.Parse(*flagHeader)
	if err != nil {
		return fmt.Errorf("parsing C functions in file '%s': %w", *flagHeader, err)
	}
	fmt.Println("package", *flagPackage)
	fmt.Println()
	fmt.Println("// GENERATED CODE -- DO NOT EDIT")
	fmt.Println()
	fmt.Println(`// #include "nk.h"`)
	fmt.Println(`import "C"`)
	fmt.Println()
	fmt.Println(`import "unsafe"`)
	for _, e := range result.Enums {
		if err := printEnum(e); err != nil {
			return fmt.Errorf("printing definition of enum %s: %w", e.Name, err)
		}
	}
	for _, f := range result.Funcs {
		if err := printFunc(typeMap, f, ""); err != nil {
			return fmt.Errorf("printing definition of function %s: %w", f.Name, err)
		}
	}
	return nil
}

func printEnum(e EnumDecl) error {
	typeName := strcase.ToCamel(strings.TrimPrefix(e.Name, "nk_"))
	untyped := false
	if _, ok := e.Attrs[AttrUntyped]; ok {
		untyped = true
	}
	var names []string
	var maxNameLen int
	for _, con := range e.Constants {
		name := strcase.ToCamel(strings.ToLower(strings.TrimPrefix(con, "NK_")))
		// fix some common acronyms
		name = strings.ReplaceAll(name, "Uv", "UV")
		name = strings.ReplaceAll(name, "Rgba", "RGBA")
		name = strings.ReplaceAll(name, "Rgb", "RGB")
		names = append(names, name)
		if len(name) > maxNameLen {
			maxNameLen = len(name)
		}
	}
	if !untyped {
		fmt.Println()
		fmt.Printf("// %s is equivalent to enum %s.\n", typeName, e.Name)
		fmt.Printf("type %s int32\n", typeName)
	}
	fmt.Println()
	if untyped {
		fmt.Printf("// constants for enum %s:\n", e.Name)
	}
	fmt.Println("const (")
	for i, name := range names {
		if untyped {
			fmt.Printf("\t%*s = C.%s\n", -maxNameLen, name, e.Constants[i])
		} else {
			fmt.Printf("\t%*s %s = C.%s\n", -maxNameLen, name, typeName, e.Constants[i])
		}
	}
	fmt.Println(")")
	return nil
}

func printFunc(typeMap map[string]TypeConv, f FunctionDecl, doc string) error {
	nakedName := strings.TrimPrefix(f.Name, "nk_")
	goFuncName := strcase.ToCamel(nakedName)
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
			end := goName[len(goName)-1]
			if '0' <= end && end <= '9' {
				goName = fmt.Sprintf("%s_%d", goName, nameCount)
			} else {
				goName = fmt.Sprintf("%s%d", goName, nameCount)
			}
		}
		// check for CStrings
		if cgoType == "C.CString" {
			rawName := fmt.Sprintf("raw%s", strcase.ToCamel(goName))
			fmt.Fprintf(&preamble, "\t%s := cStringPool.Get(%s)\n", rawName, goName)
			fmt.Fprintf(&preamble, "\tdefer cStringPool.Release(%s)\n", rawName)
			cParams[cParamIndex] = rawName
			if _, ok := f.Attrs["nostrlen"]; !ok {
				// skip over the next param in the normal loop
				i++
				nextCParamIndex := i
				nextGoParamIndex := i - goParamOffset
				// ensure it is an int
				if nextCParamIndex >= len(f.Params) || f.Params[nextCParamIndex].Type != "int" {
					return fmt.Errorf("string parameter %d is not followed by length param (set attr nostrlen to override)", i)
				}
				// synthesize a C parameter set to the string length
				cParams[nextCParamIndex] = fmt.Sprintf("C.int(len(%s))", goName)
				// put a sentinel value in for the Go parameter
				goParams[nextGoParamIndex] = "__DELETED__"
			}
		} else if len(cgoType) == 0 {
			cParams[cParamIndex] = goName
		} else if goType == "Handle" {
			cParams[cParamIndex] = fmt.Sprintf("%s.raw()", goName)
		} else {
			var paramFormat string
			_, hasAttrUnsafePtr := f.Attrs[AttrUnsafePtr]
			if strings.HasPrefix(cgoType, "*C.struct_") || strings.HasPrefix(cgoType, "*") && hasAttrUnsafePtr {
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
	return nil
}
