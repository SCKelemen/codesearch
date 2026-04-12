package structural

import (
	"go/ast"
	"go/parser"
	"go/token"
)

// ExtractGoSymbols extracts top-level and type-member symbols from Go source.
func ExtractGoSymbols(filePath string, src []byte) ([]Symbol, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	symbols := make([]Symbol, 0, len(file.Decls)+1)
	symbols = append(symbols, Symbol{
		Name:     file.Name.Name,
		Kind:     SymbolKindPackage,
		Language: "go",
		Path:     filePath,
		Range:    tokenRange(fset, file.Name.Pos(), file.Name.End()),
		Exported: ast.IsExported(file.Name.Name),
	})

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			symbols = append(symbols, goFuncSymbol(fset, filePath, d))
		case *ast.GenDecl:
			symbols = append(symbols, extractGoGenDeclSymbols(fset, filePath, d)...)
		}
	}

	return symbols, nil
}

func goFuncSymbol(fset *token.FileSet, filePath string, decl *ast.FuncDecl) Symbol {
	symbol := Symbol{
		Name:     decl.Name.Name,
		Kind:     SymbolKindFunction,
		Language: "go",
		Path:     filePath,
		Range:    tokenRange(fset, decl.Pos(), decl.End()),
		Exported: ast.IsExported(decl.Name.Name),
	}
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		symbol.Kind = SymbolKindMethod
		symbol.Container = goReceiverName(decl.Recv.List[0].Type)
	}
	return symbol
}

func extractGoGenDeclSymbols(fset *token.FileSet, filePath string, decl *ast.GenDecl) []Symbol {
	var symbols []Symbol
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			kind := SymbolKindType
			switch s.Type.(type) {
			case *ast.StructType:
				kind = SymbolKindStruct
			case *ast.InterfaceType:
				kind = SymbolKindInterface
			}
			symbols = append(symbols, Symbol{
				Name:     s.Name.Name,
				Kind:     kind,
				Language: "go",
				Path:     filePath,
				Range:    tokenRange(fset, s.Pos(), s.End()),
				Exported: ast.IsExported(s.Name.Name),
			})
			symbols = append(symbols, extractGoTypeMembers(fset, filePath, s)...)
		case *ast.ValueSpec:
			kind := SymbolKindVariable
			if decl.Tok == token.CONST {
				kind = SymbolKindConstant
			}
			for _, name := range s.Names {
				symbols = append(symbols, Symbol{
					Name:     name.Name,
					Kind:     kind,
					Language: "go",
					Path:     filePath,
					Range:    tokenRange(fset, name.Pos(), name.End()),
					Exported: ast.IsExported(name.Name),
				})
			}
		}
	}
	return symbols
}

func extractGoTypeMembers(fset *token.FileSet, filePath string, spec *ast.TypeSpec) []Symbol {
	var symbols []Symbol
	container := spec.Name.Name

	switch typ := spec.Type.(type) {
	case *ast.StructType:
		for _, field := range typ.Fields.List {
			for _, name := range field.Names {
				symbols = append(symbols, Symbol{
					Name:      name.Name,
					Kind:      SymbolKindField,
					Language:  "go",
					Path:      filePath,
					Container: container,
					Range:     tokenRange(fset, name.Pos(), name.End()),
					Exported:  ast.IsExported(name.Name),
				})
			}
		}
	case *ast.InterfaceType:
		for _, field := range typ.Methods.List {
			for _, name := range field.Names {
				symbols = append(symbols, Symbol{
					Name:      name.Name,
					Kind:      SymbolKindMethod,
					Language:  "go",
					Path:      filePath,
					Container: container,
					Range:     tokenRange(fset, name.Pos(), name.End()),
					Exported:  ast.IsExported(name.Name),
				})
			}
		}
	}

	return symbols
}

func goReceiverName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return goReceiverName(e.X)
	case *ast.IndexExpr:
		return goReceiverName(e.X)
	case *ast.IndexListExpr:
		return goReceiverName(e.X)
	default:
		return ""
	}
}

func tokenRange(fset *token.FileSet, start token.Pos, end token.Pos) Range {
	startPos := fset.Position(start)
	endPos := fset.Position(end)
	return Range{
		StartLine:   startPos.Line,
		StartColumn: startPos.Column,
		EndLine:     endPos.Line,
		EndColumn:   endPos.Column,
	}
}
