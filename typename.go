package main

import (
	"errors"
	"fmt"
	"strings"

	"modernc.org/cc/v3"
)

func makeFuncParams(paramList *cc.ParameterList) ([]FunctionParam, error) {
	var params []FunctionParam
	// parameter_list
	//   : parameter_declaration
	//   | parameter_list ',' parameter_declaration
	//   ;
	for pl := paramList; pl != nil; pl = pl.ParameterList {
		// parameter_declaration
		//   : declaration_specifiers declarator
		//   | declaration_specifiers abstract_declarator
		//   | declaration_specifiers
		//   ;
		pd := pl.ParameterDeclaration
		var paramType strings.Builder
		var name string
		writeDeclSpec(&paramType, pd.DeclarationSpecifiers)
		if decl := pd.Declarator; decl != nil {
			// declarator
			//   : pointer direct_declarator
			//   | direct_declarator
			//   ;
			if ptr := decl.Pointer; ptr != nil {
				writePointer(&paramType, ptr)
			}
			if dirDecl := decl.DirectDeclarator; dirDecl != nil {
				// direct_declarator
				//   : IDENTIFIER
				//   | '(' declarator ')'
				//   | direct_declarator '[' constant_expression ']'
				//   | direct_declarator '[' ']'
				//   | direct_declarator '(' parameter_type_list ')'
				//   | direct_declarator '(' identifier_list ')'
				//   | direct_declarator '(' ')'
				//   ;
				switch dirDecl.Case {
				case cc.DirectDeclaratorIdent:
					name = dirDecl.Name().String()
				default:
					return nil, errors.New("nested direct_declarator found")
				}
			}
		}
		if absDecl := pd.AbstractDeclarator; absDecl != nil {
			// abstract_declarator
			//   : pointer
			//   | direct_abstract_declarator
			//   | pointer direct_abstract_declarator
			//   ;
			if ptr := absDecl.Pointer; ptr != nil {
				writePointer(&paramType, ptr)
			}
			if dirAbsDecl := absDecl.DirectAbstractDeclarator; dirAbsDecl != nil {
				return nil, errors.New("direct_abstract_declarator found")
			}
		}
		params = append(params, FunctionParam{
			Name: name,
			Type: strings.TrimSpace(paramType.String()),
		})
	}
	return params, nil
}
func returnTypeName(declSpec *cc.DeclarationSpecifiers, pointer *cc.Pointer) (string, error) {
	var result strings.Builder
	if err := writeDeclSpec(&result, declSpec); err != nil {
		return "", err
	}
	if err := writePointer(&result, pointer); err != nil {
		return "", err
	}
	return strings.TrimSpace(result.String()), nil
}

func writeDeclSpec(dst *strings.Builder, declSpec *cc.DeclarationSpecifiers) error {
	// declaration_specifiers
	//   : storage_class_specifier
	//   | storage_class_specifier declaration_specifiers
	//   | type_specifier
	//   | type_specifier declaration_specifiers
	//   | type_qualifier
	//   | type_qualifier declaration_specifiers
	//   ;
	for ds := declSpec; ds != nil; ds = ds.DeclarationSpecifiers {
		// ignore storage_class_specifier
		if ts := ds.TypeSpecifier; ts != nil {
			if err := writeTypeSpec(dst, ts); err != nil {
				return err
			}
		}
		if tq := ds.TypeQualifier; tq != nil {
			if err := writeTypeQual(dst, tq); err != nil {
				return err
			}
		}
	}
	return nil

}

func writePointer(dst *strings.Builder, pointer *cc.Pointer) error {
	// pointer
	//   : '*'
	//   | '*' type_qualifier_list
	//   | '*' pointer
	//   | '*' type_qualifier_list pointer
	//   ;
	for p := pointer; p != nil; p = p.Pointer {
		dst.WriteRune('*')
		if tql := p.TypeQualifiers; tql != nil {
			if err := writeTypeQualList(dst, tql); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeTypeQual(dst *strings.Builder, typeQual *cc.TypeQualifier) error {
	// type_qualifier
	//   : CONST
	//   | RESTRICT
	//   | VOLATILE
	//   ;
	switch typeQual.Case {
	case cc.TypeQualifierConst:
		dst.WriteString("const ")
	case cc.TypeQualifierRestrict:
		dst.WriteString("restrict ")
	case cc.TypeQualifierVolatile:
		dst.WriteString("volatile ")
	default:
		return fmt.Errorf("unhandled type_qualifier case %s", typeQual.Case)
	}
	return nil
}

func writeTypeSpec(dst *strings.Builder, typeSpec *cc.TypeSpecifier) error {
	// type_specifier
	//   : VOID
	//   | CHAR
	//   | SHORT
	//   | INT
	//   | LONG
	//   | FLOAT
	//   | DOUBLE
	//   | SIGNED
	//   | UNSIGNED
	//   | BOOL
	//   | COMPLEX
	//   | IMAGINARY
	//   | struct_or_union_specifier
	//   | enum_specifier
	//   | TYPE_NAME
	// ;
	switch typeSpec.Case {
	case cc.TypeSpecifierVoid:
		dst.WriteString("void ")
	case cc.TypeSpecifierChar:
		dst.WriteString("char ")
	case cc.TypeSpecifierShort:
		dst.WriteString("short ")
	case cc.TypeSpecifierInt:
		dst.WriteString("int ")
	case cc.TypeSpecifierLong:
		dst.WriteString("long ")
	case cc.TypeSpecifierFloat:
		dst.WriteString("float ")
	case cc.TypeSpecifierDouble:
		dst.WriteString("double ")
	case cc.TypeSpecifierBool:
		dst.WriteString("bool ")
	case cc.TypeSpecifierComplex:
		dst.WriteString("complex ")
	case cc.TypeSpecifierStructOrUnion:
		// struct_or_union_specifier
		//   : struct_or_union IDENTIFIER '{' struct_declaration_list '}'
		//   | struct_or_union '{' struct_declaration_list '}'
		//   | struct_or_union IDENTIFIER
		//   ;
		sus := typeSpec.StructOrUnionSpecifier
		if sus.AttributeSpecifierList != nil {
			return errors.New("unhandled attribute_specifier_list on struct_or_union_specifier")
		} else if sus.StructDeclarationList != nil {
			return errors.New("unhandled struct_declaration_list on struct_or_union_specifier")
		}
		switch sus.StructOrUnion.Case {
		case cc.StructOrUnionStruct:
			dst.WriteString("struct ")
		case cc.StructOrUnionUnion:
			dst.WriteString("union ")
		default:
			return fmt.Errorf("unhandled struct_or_union case %s", sus.StructOrUnion.Case)
		}
		dst.WriteString(sus.Token.String())
		dst.WriteRune(' ')
		// TODO what to do with token2 and token3?
	case cc.TypeSpecifierEnum:
		// enum_specifier
		//   : ENUM '{' enumerator_list '}'
		//   | ENUM IDENTIFIER '{' enumerator_list '}'
		//   | ENUM '{' enumerator_list ',' '}'
		//   | ENUM IDENTIFIER '{' enumerator_list ',' '}'
		//   | ENUM IDENTIFIER
		//   ;
		es := typeSpec.EnumSpecifier
		if es.AttributeSpecifierList != nil {
			return errors.New("unhandled attribute_specifier_list on enum_specifier")
		} else if es.EnumeratorList != nil {
			return errors.New("unhandled enumerator_list on enum_specifier")
		}
		dst.WriteString("enum ")
		dst.WriteString(es.Token2.String())
		// TODO what to do with other tokens?
	case cc.TypeSpecifierTypedefName:
		dst.WriteString(typeSpec.Token.String())
	default:
		return fmt.Errorf("unhandled type_specifier case %s", typeSpec.Case)
	}
	return nil
}

func writeTypeQualList(dst *strings.Builder, typeQualList *cc.TypeQualifiers) error {
	// type_qualifier_list
	//   : type_qualifier
	//   | type_qualifier_list type_qualifier
	//   ;
	for tql := typeQualList; tql != nil; tql = tql.TypeQualifiers {
		if err := writeTypeQual(dst, tql.TypeQualifier); err != nil {
			return err
		}
	}
	return nil
}
