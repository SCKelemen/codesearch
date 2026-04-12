package query

import "testing"

func TestCompileAndEval(t *testing.T) {
	t.Parallel()
	f, err := Compile("language == \"go\"")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	ok, err := f.Eval(Document{Language: "go"})
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if !ok {
		t.Error("expected match")
	}
	ok, err = f.Eval(Document{Language: "rust"})
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if ok {
		t.Error("expected no match")
	}
}

func TestCompileComplexFilter(t *testing.T) {
	t.Parallel()
	f, err := Compile("language == \"go\" && exported == true && refs > 5")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	ok, _ := f.Eval(Document{Language: "go", Exported: true, Refs: 10})
	if !ok {
		t.Error("expected match")
	}
	ok, _ = f.Eval(Document{Language: "go", Exported: true, Refs: 2})
	if ok {
		t.Error("expected no match for refs=2")
	}
}

func TestCompileStringFunctions(t *testing.T) {
	t.Parallel()
	f, err := Compile("file.endsWith(\"_test.go\")")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	ok, _ := f.Eval(Document{File: "auth_test.go"})
	if !ok {
		t.Error("expected match for _test.go file")
	}
	ok, _ = f.Eval(Document{File: "auth.go"})
	if ok {
		t.Error("expected no match for .go file")
	}
}

func TestCompileInOperator(t *testing.T) {
	t.Parallel()
	f, err := Compile("language in [\"go\", \"rust\", \"python\"]")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	ok, _ := f.Eval(Document{Language: "rust"})
	if !ok {
		t.Error("expected match for rust")
	}
	ok, _ = f.Eval(Document{Language: "java"})
	if ok {
		t.Error("expected no match for java")
	}
}

func TestCompileInvalidExpression(t *testing.T) {
	t.Parallel()
	_, err := Compile("invalid ++ syntax")
	if err == nil {
		t.Fatal("expected error for invalid expression")
	}
}

func TestCompileNonBoolExpression(t *testing.T) {
	t.Parallel()
	_, err := Compile("name")
	if err == nil {
		t.Fatal("expected error for non-bool expression")
	}
}

func TestEvalMany(t *testing.T) {
	t.Parallel()
	f := MustCompile("exported == true")
	docs := []Document{
		{Name: "Foo", Exported: true},
		{Name: "bar", Exported: false},
		{Name: "Baz", Exported: true},
	}
	results, err := f.EvalMany(docs)
	if err != nil {
		t.Fatalf("EvalMany: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	if err := Validate("language == \"go\""); err != nil {
		t.Errorf("valid expression failed: %v", err)
	}
	if err := Validate("invalid ++ syntax"); err == nil {
		t.Error("expected error for invalid expression")
	}
}

func TestExpression(t *testing.T) {
	t.Parallel()
	expr := "language == \"go\""
	f := MustCompile(expr)
	if f.Expression() != expr {
		t.Errorf("Expression() = %q, want %q", f.Expression(), expr)
	}
}

func TestVariables(t *testing.T) {
	t.Parallel()
	vars := Variables()
	if len(vars) == 0 {
		t.Error("expected variables")
	}
}
