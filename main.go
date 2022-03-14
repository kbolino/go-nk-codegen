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
	for _, f := range funcs {
		nakedName := strings.TrimPrefix(f.Name, "nk_")
		goFuncName := strcase.ToCamel(nakedName)
		method := true
		if len(f.Params) == 0 || f.Params[0].Type != "struct nk_context *" {
			method = false
			debugf("marking function %s as not a method because its first parameter is not typed 'struct nk_context *'",
				f.Name)
		}
		goParamTypes := make([]string, len(f.Params)-1)
		goParams := make([]string, len(f.Params)-1)
		cParams := make([]string, len(f.Params))
		for i := 1; i < len(f.Params); i++ {
			cParam := f.Params[i]
			goType, cgoType, err := convertType(typeMap, cParam.Type, ConvertTypeDefault)
			if err != nil {
				return fmt.Errorf("converting type '%s' of parameter %d of function %s: %w", cParam.Type, i, f.Name, err)
			}
			goName := strcase.ToLowerCamel(cParam.Name)
			if goName == "" {
				goName = fmt.Sprintf("param%d", i)
			}
			goParamTypes[i-1] = goType
			goParams[i-1] = fmt.Sprintf("%s %s", goName, goType)
			cParams[i] = fmt.Sprintf("(%s)(%s)", cgoType, goName)
		}
		if method {
			cParams[0] = "ctx.raw()"
		} else {
			cParams = cParams[1:]
		}
		retType, _, err := convertType(typeMap, f.Return, ConvertTypeDefault)
		if err != nil {
			return fmt.Errorf("converting type '%s' of return for function %s: %w", f.Return, f.Name, err)
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
			debugf("skipping method %s because it matches done signature '%s'", goFuncName, signature)
			continue
		}
		namedMethodReceiver := ""
		if method {
			namedMethodReceiver = "(ctx *Context) "
		}
		paramList := strings.Join(goParams, ", ")
		castList := strings.Join(cParams, ", ")
		fmt.Println()
		fmt.Printf("// %s calls %s.\n", goFuncName, f.Name)
		if retType == "" {
			fmt.Printf("func %s%s(%s) {\n", namedMethodReceiver, goFuncName, paramList)
			fmt.Printf("\tC.%s(%s)\n", f.Name, castList)
		} else {
			fmt.Printf("func %s%s(%s) %s {\n", namedMethodReceiver, goFuncName, paramList, retType)
			fmt.Printf("\t(%s)(C.%s(%s))\n", retType, f.Name, castList)
		}
		fmt.Println("}")
	}
	return nil
}
