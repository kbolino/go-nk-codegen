package main

import (
	"fmt"
	"sort"

	"modernc.org/cc/v3"
)

type EnumDecl struct {
	Name      string
	Constants []string
}

type FunctionDecl struct {
	Name   string
	Return string
	Params []FunctionParam
}

type FunctionParam struct {
	Name string
	Type string
}

type StructDecl struct {
	Name    string
	Members []StructMember
}

type StructMember struct {
	Name string
	Type string
}

type Matcher interface {
	MatchEnum(name string) bool
	MatchFunc(name string) bool
	MatchStruct(name string) bool
}

type patternMatcher struct {
	enumPatterns   []Pattern
	funcPatterns   []Pattern
	structPatterns []Pattern
}

func (m *patternMatcher) MatchEnum(name string) bool {
	return m.match("enum", name, m.enumPatterns)
}

func (m *patternMatcher) MatchFunc(name string) bool {
	return m.match("function", name, m.funcPatterns)
}

func (m *patternMatcher) MatchStruct(name string) bool {
	return m.match("struct", name, m.structPatterns)
}

func (m *patternMatcher) match(what, name string, patterns []Pattern) bool {
	if len(patterns) == 0 {
		return false
	}
	include := false
	for _, pattern := range patterns {
		if match := pattern.Regexp.FindString(name); match == name {
			if pattern.Negate {
				include = false
				debugf("excluding %s %s because of negated pattern '%s'", what, name, pattern.Regexp)
			} else {
				include = true
				debugf("including %s %s because of pattern '%s'", what, name, pattern.Regexp)
			}
		}
	}
	return include
}

func NewPatternMatcher(enumPatterns, funcPatterns, structPatterns []Pattern) Matcher {
	return &patternMatcher{
		enumPatterns:   enumPatterns,
		funcPatterns:   funcPatterns,
		structPatterns: structPatterns,
	}
}

type Parser struct {
	matcher Matcher
}

func NewParser(matcher Matcher) *Parser {
	return &Parser{
		matcher: matcher,
	}
}

type ParseResult struct {
	Enums   []EnumDecl
	Funcs   []FunctionDecl
	Structs []StructDecl
}

func (p *Parser) Parse(fileName string) (ParseResult, error) {
	debug("determing host configuration from C preprocessor")
	predefined, includePaths, sysIncludePaths, err := cc.HostConfig(*flagCPP)
	if err != nil {
		return ParseResult{}, fmt.Errorf("obtaining host configuration: %w", err)
	}
	debugf("predefined = %s", predefined)
	debugf("includePaths = %v", includePaths)
	debugf("sysIncludePaths = %v", sysIncludePaths)
	if *flagInclude != "" {
		debugf("appending %s to includePaths", *flagInclude)
		includePaths = append(includePaths, *flagInclude)
	}
	sources := []cc.Source{
		{Name: "__predefined__", Value: predefined},
		{Name: fileName},
	}
	debugf("parsing file %s", fileName)
	cfg := &cc.Config{}
	ast, err := cc.Parse(cfg, includePaths, sysIncludePaths, sources)
	if err != nil {
		return ParseResult{}, fmt.Errorf("parsing sources: %w", err)
	}
	var funcs []FunctionDecl
	// translation_unit
	//   : external_declaration
	//   | translation_unit external_declaration
	//   ;
	for tu := ast.TranslationUnit; tu != nil; tu = tu.TranslationUnit {
		// external_declaration
		//   : function_definition
		//   | declaration
		//   ;
		decln := tu.ExternalDeclaration.Declaration
		if decln == nil {
			// function definition, not mere declaration
			continue
		}
		// declaration
		//   : declaration_specifiers ';'
		//   | declaration_specifiers init_declarator_list ';'
		//   ;
		idl := decln.InitDeclaratorList
		if idl == nil {
			// no declarator
			continue
		}
		// init_declarator_list
		//   : init_declarator
		//   | init_declarator_list ',' init_declarator
		//   ;
		if idl.InitDeclaratorList != nil {
			// multiple declarators
			continue
		}
		// init_declarator
		//   : declarator
		//   | declarator '=' initializer
		//   ;
		idecl := idl.InitDeclarator
		if idecl.Initializer != nil {
			// has initializer
			continue
		}
		// declarator
		//   : pointer direct_declarator
		//   | direct_declarator
		//   ;
		decl := idecl.Declarator
		if !p.matcher.MatchFunc(decl.Name().String()) {
			continue
		}
		// direct_declarator
		//   : IDENTIFIER
		//   | '(' declarator ')'
		//   | direct_declarator '[' constant_expression ']'
		//   | direct_declarator '[' ']'
		//   | direct_declarator '(' parameter_type_list ')'
		//   | direct_declarator '(' identifier_list ')'
		//   | direct_declarator '(' ')'
		//   ;
		var params []FunctionParam
		ddecl := decl.DirectDeclarator
		switch ddecl.Case {
		case cc.DirectDeclaratorFuncParam:
			// parameter_type_list
			//   : parameter_list
			//   | parameter_list ',' ELLIPSIS
			//   ;
			switch ddecl.ParameterTypeList.Case {
			case cc.ParameterTypeListList:
				params, err = makeFuncParams(ddecl.ParameterTypeList.ParameterList)
				if err != nil {
					return ParseResult{}, fmt.Errorf("cannot resolve parameters for function %s: %w", decl.Name(), err)
				}
			case cc.ParameterTypeListVar:
				return ParseResult{}, fmt.Errorf("function %s requires varargs support", decl.Name())
			}
		default:
			debugf("ignoring non-function declaration %s", decl.Name())
			continue
		}
		returnType, err := returnTypeName(decln.DeclarationSpecifiers, decl.Pointer)
		if err != nil {
			return ParseResult{}, fmt.Errorf("cannot resolve return type for function %s: %w", decl.Name(), err)
		}
		debugf("found function %s", decl.Name())
		funcs = append(funcs, FunctionDecl{
			Name:   decl.Name().String(),
			Return: returnType,
			Params: params,
		})
	}
	sort.Slice(funcs, func(i, j int) bool {
		return funcs[i].Name < funcs[j].Name
	})
	return ParseResult{
		Funcs: funcs,
	}, nil
}
