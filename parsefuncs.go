package main

import (
	"fmt"
	"sort"

	"modernc.org/cc/v3"
)

type FunctionDecl struct {
	Name   string
	Return string
	Params []FunctionParam
}

type FunctionParam struct {
	Name string
	Type string
}

func parseCFuncs(fileName string) ([]FunctionDecl, error) {
	debug("determing host configuration from C preprocessor")
	predefined, includePaths, sysIncludePaths, err := cc.HostConfig(*flagCPP)
	if err != nil {
		return nil, fmt.Errorf("obtaining host configuration: %w", err)
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
		return nil, fmt.Errorf("parsing sources: %w", err)
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
					debugf("skipping function '%s' due to error making parameters: %s", decl.Name(), err)
					continue
				}
			case cc.ParameterTypeListVar:
				debugf("ignoring varargs function %s", decl.Name())
				continue
			}
		default:
			debugf("ignoring non-function declaration %s", decl.Name())
			continue
		}
		returnType, err := returnTypeName(decln.DeclarationSpecifiers, decl.Pointer)
		if err != nil {
			debugf("skipping function '%s' due to error computing return type: %s", decl.Name(), err)
			continue
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
	return funcs, nil
}
