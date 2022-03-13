package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/iancoleman/strcase"
)

type TypeConv struct {
	GoType  string
	CgoType string
}

type ConvertTypeOpts int32

const (
	ConvertTypeAutoPtr ConvertTypeOpts = 1 << iota
	ConvertTypeAutoStructEnum

	ConvertTypeDefault = ConvertTypeAutoPtr | ConvertTypeAutoStructEnum
)

func convertType(typeMap map[string]TypeConv, cType string, options ConvertTypeOpts) (goType string, cgoType string, err error) {
	debugf("converting type '%s'", cType)
	cType = strings.TrimPrefix(cType, "const ")
	if mapping, ok := typeMap[cType]; ok {
		return mapping.GoType, mapping.CgoType, nil
	}
	if options&ConvertTypeAutoPtr != 0 && strings.HasSuffix(cType, "*") {
		cType = strings.TrimSpace(strings.TrimSuffix(cType, "*"))
		rawGoType, rawCgoType, err := convertType(typeMap, cType, options)
		if err != nil {
			return "", "", fmt.Errorf("resolving type '%s': %w", cType, err)
		}
		return "*" + rawGoType, "*" + rawCgoType, nil
	}
	if options&ConvertTypeAutoStructEnum != 0 && strings.HasPrefix(cType, "struct ") {
		cType = strings.TrimPrefix(cType, "struct ")
		goType = strcase.ToCamel(strings.TrimPrefix(cType, "nk_"))
		cgoType = "C.struct_" + cType
		return goType, cgoType, nil
	} else if options&ConvertTypeAutoStructEnum != 0 && strings.HasPrefix(cType, "enum ") {
		cType = strings.TrimPrefix(cType, "enum ")
		goType = strcase.ToCamel(strings.TrimPrefix(cType, "nk_"))
		cgoType = "C.enum_" + cType
		return goType, cgoType, nil
	}
	return "", "", fmt.Errorf("unhandled C type '%s'", cType)
}

func parseTypeMap(fileName string) (map[string]TypeConv, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()
	typeMap := make(map[string]TypeConv)
	reader := csv.NewReader(file)
	reader.Comment = '#'
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = 3
	reader.ReuseRecord = true
	for {
		record, err := reader.Read()
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("CSV read error: %w", err)
		} else if len(record) == 0 {
			break
		}
		typeMap[record[0]] = TypeConv{
			GoType:  record[1],
			CgoType: record[2],
		}
	}
	return typeMap, nil
}
