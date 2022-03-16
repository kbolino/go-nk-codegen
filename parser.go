package main

import (
	"fmt"
	"sort"

	"modernc.org/cc/v3"
)

const Anonymous = "(anonymous)"

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
	anyMatch := false
	for _, pattern := range patterns {
		if match := pattern.Regexp.FindString(name); match == name {
			if pattern.Negate {
				include = false
				anyMatch = true
				debugf("excluding %s %s because of negated pattern '%s'", what, name, pattern.Regexp)
			} else {
				include = true
				anyMatch = true
				debugf("including %s %s because of pattern '%s'", what, name, pattern.Regexp)
			}
		}
	}
	if !anyMatch {
		debugf("excluding %s %s because no patterns matched it", what, name)
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
	var enums []EnumDecl
	var funcs []FunctionDecl
	var structs []StructDecl
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
			// no declarator, so not a function, could be enum or struct

			// declaration_specifiers
			//   : storage_class_specifier
			//   | storage_class_specifier declaration_specifiers
			//   | type_specifier
			//   | type_specifier declaration_specifiers
			//   | type_qualifier
			//   | type_qualifier declaration_specifiers
			//   ;
			for ds := decln.DeclarationSpecifiers; ds != nil; ds = ds.DeclarationSpecifiers {
				if ts := ds.TypeSpecifier; ts != nil && ts.Case == cc.TypeSpecifierEnum {
					enumDecl, err := p.parseEnum(decln)
					if err != nil {
						return ParseResult{}, fmt.Errorf("parsing enum at position %s: %w", tu.Position(), err)
					}
					if enumDecl.Name != "" {
						enums = append(enums, enumDecl)
					}
				} else if ts != nil && ts.Case == cc.TypeSpecifierStructOrUnion {
					structDecl, err := p.parseStruct(decln)
					if err != nil {
						return ParseResult{}, fmt.Errorf("parsing struct at position %s: %w", tu.Position(), err)
					}
					if structDecl.Name != "" {
						structs = append(structs, structDecl)
					}
				}
			}
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
			debugf("found function %s at %s", decl.Name(), decl.Position())
			if !p.matcher.MatchFunc(decl.Name().String()) {
				continue
			}
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
			debugf("ignoring non-function declaration %s at %s", decl.Name(), decl.Position())
			continue
		}
		returnType, err := returnTypeName(decln.DeclarationSpecifiers, decl.Pointer)
		if err != nil {
			return ParseResult{}, fmt.Errorf("cannot resolve return type for function %s: %w", decl.Name(), err)
		}
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
		Enums:   enums,
		Funcs:   funcs,
		Structs: structs,
	}, nil
}

func (p *Parser) parseEnum(decln *cc.Declaration) (EnumDecl, error) {
	// enum_specifier
	//   : ENUM '{' enumerator_list '}'
	//   | ENUM IDENTIFIER '{' enumerator_list '}'
	//   | ENUM IDENTIFIER
	//   ;
	for ds := decln.DeclarationSpecifiers; ds != nil; ds = ds.DeclarationSpecifiers {
		if ts := ds.TypeSpecifier; ts == nil {
			continue
		} else if es := ts.EnumSpecifier; es == nil {
			continue
		} else if es.Case != cc.EnumSpecifierDef {
			return EnumDecl{}, nil
		} else {
			name := es.Token2.String()
			if name == "" {
				name = Anonymous
			}
			debugf("found enum %s at %s", name, es.Position())
			if !p.matcher.MatchEnum(name) {
				return EnumDecl{}, nil
			}
			// enumerator_list
			//   : enumerator
			//   | enumerator_list ',' enumerator
			//   ;
			// enumerator
			//   : IDENTIFIER
			//   | IDENTIFIER '=' constant_expression
			//   ;
			var constants []string
			for el := es.EnumeratorList; el != nil; el = el.EnumeratorList {
				constants = append(constants, el.Enumerator.Token.String())
			}
			return EnumDecl{
				Name:      name,
				Constants: constants,
			}, nil
		}
	}
	return EnumDecl{}, nil
}

func (p *Parser) parseStruct(decln *cc.Declaration) (StructDecl, error) {
	// TODO
	return StructDecl{}, nil
}
