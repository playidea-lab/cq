package dartast

import (
	"os"
	"path/filepath"
	"testing"
)

const testDartSource = `import 'package:flutter/material.dart';

/// A counter that notifies listeners.
class Counter extends ChangeNotifier {
  int _count = 0;

  /// Gets the current count.
  int get count => _count;

  set count(int value) {
    _count = value;
    notifyListeners();
  }

  void increment() {
    _count++;
    notifyListeners();
  }

  Counter(this._count);

  Counter.zero() : _count = 0;

  factory Counter.fromJson(Map<String, dynamic> json) {
    return Counter(json['count'] as int);
  }
}

abstract class BaseService {
  Future<void> init();
  void dispose();
}

mixin Draggable on Widget {
  void onDrag(DragUpdateDetails details) {}
}

enum Status { active, inactive, pending }

extension StringX on String {
  String capitalize() {
    if (isEmpty) return this;
    return '${this[0].toUpperCase()}${substring(1)}';
  }
}

typedef Callback = void Function(int);

void main() {
  final counter = Counter(0);
  counter.increment();
}
`

func writeDartFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	path := writeDartFile(t, dir, "counter.dart", testDartSource)

	symbols, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(symbols) == 0 {
		t.Fatal("expected symbols, got 0")
	}

	// Count by kind
	kinds := map[string]int{}
	for _, s := range symbols {
		kinds[s.Kind]++
	}

	// Should have: Counter (class), BaseService (class), Draggable (mixin),
	// Status (enum), StringX (extension), Callback (typedef), main (function),
	// + methods/constructors/getters/setters inside classes
	if kinds["class"] < 2 {
		t.Errorf("expected at least 2 classes, got %d", kinds["class"])
	}
	if kinds["mixin"] != 1 {
		t.Errorf("expected 1 mixin, got %d", kinds["mixin"])
	}
	if kinds["enum"] != 1 {
		t.Errorf("expected 1 enum, got %d", kinds["enum"])
	}
	if kinds["extension"] != 1 {
		t.Errorf("expected 1 extension, got %d", kinds["extension"])
	}
	if kinds["typedef"] != 1 {
		t.Errorf("expected 1 typedef, got %d", kinds["typedef"])
	}
	if kinds["function"] < 1 {
		t.Errorf("expected at least 1 function (main), got %d", kinds["function"])
	}
}

func TestParseFileDetails(t *testing.T) {
	dir := t.TempDir()
	path := writeDartFile(t, dir, "counter.dart", testDartSource)

	symbols, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Find Counter class
	var counter *Symbol
	for i, s := range symbols {
		if s.Name == "Counter" && s.Kind == "class" {
			counter = &symbols[i]
			break
		}
	}
	if counter == nil {
		t.Fatal("Counter class not found")
	}
	if counter.Docstring != "A counter that notifies listeners." {
		t.Errorf("unexpected docstring: %q", counter.Docstring)
	}
	if counter.Line != 4 {
		t.Errorf("Counter should be on line 4, got %d", counter.Line)
	}

	// Find getter
	var getter *Symbol
	for i, s := range symbols {
		if s.Name == "count" && s.Kind == "getter" {
			getter = &symbols[i]
			break
		}
	}
	if getter == nil {
		t.Fatal("count getter not found")
	}
	if getter.ParentName != "Counter" {
		t.Errorf("getter parent should be Counter, got %q", getter.ParentName)
	}
	if getter.Docstring != "Gets the current count." {
		t.Errorf("unexpected getter docstring: %q", getter.Docstring)
	}

	// Find increment method
	var incr *Symbol
	for i, s := range symbols {
		if s.Name == "increment" && s.Kind == "method" {
			incr = &symbols[i]
			break
		}
	}
	if incr == nil {
		t.Fatal("increment method not found")
	}
	if incr.ParentName != "Counter" {
		t.Errorf("increment parent should be Counter, got %q", incr.ParentName)
	}
}

func TestConstructors(t *testing.T) {
	dir := t.TempDir()
	path := writeDartFile(t, dir, "counter.dart", testDartSource)

	symbols, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}

	constructors := map[string]bool{}
	for _, s := range symbols {
		if s.Kind == "constructor" {
			constructors[s.Name] = true
		}
	}

	// Counter(this._count), Counter.zero(), factory Counter.fromJson()
	if !constructors["Counter"] {
		t.Error("unnamed constructor Counter not found")
	}
	if !constructors["Counter.zero"] {
		t.Error("named constructor Counter.zero not found")
	}
	if !constructors["Counter.fromJson"] {
		t.Error("factory constructor Counter.fromJson not found")
	}
}

func TestAbstractMethods(t *testing.T) {
	dir := t.TempDir()
	path := writeDartFile(t, dir, "counter.dart", testDartSource)

	symbols, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// BaseService.init() and BaseService.dispose() are abstract (end with ;)
	var initMethod *Symbol
	for i, s := range symbols {
		if s.Name == "init" && s.ParentName == "BaseService" {
			initMethod = &symbols[i]
			break
		}
	}
	if initMethod == nil {
		t.Fatal("BaseService.init not found")
	}
	if initMethod.Kind != "method" {
		t.Errorf("init should be method, got %s", initMethod.Kind)
	}
	// Abstract method: line == endline (single line declaration)
	if initMethod.EndLine != initMethod.Line {
		t.Errorf("abstract method should have endLine == line, got %d != %d", initMethod.EndLine, initMethod.Line)
	}
}

func TestFindSymbolByName(t *testing.T) {
	dir := t.TempDir()
	writeDartFile(t, dir, "counter.dart", testDartSource)

	// Simple name
	matches, err := FindSymbolByName("Counter", dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("Counter not found")
	}
	if matches[0].Kind != "class" {
		t.Errorf("expected class, got %s", matches[0].Kind)
	}

	// Parent/member notation
	matches, err = FindSymbolByName("Counter/increment", dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for Counter/increment, got %d", len(matches))
	}
	if matches[0].Kind != "method" {
		t.Errorf("expected method, got %s", matches[0].Kind)
	}

	// No match
	matches, err = FindSymbolByName("NonExistent", dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matches))
	}
}

func TestSymbolsOverview(t *testing.T) {
	dir := t.TempDir()
	writeDartFile(t, dir, "counter.dart", testDartSource)

	overview, err := SymbolsOverview(dir)
	if err != nil {
		t.Fatal(err)
	}

	if overview["success"] != true {
		t.Error("expected success=true")
	}

	classes, ok := overview["classes"].([]map[string]any)
	if !ok || len(classes) == 0 {
		t.Error("expected classes in overview")
	}

	functions, ok := overview["functions"].([]map[string]any)
	if !ok || len(functions) == 0 {
		t.Error("expected functions in overview")
	}
}

func TestHasDartFiles(t *testing.T) {
	dir := t.TempDir()

	if HasDartFiles(dir) {
		t.Error("empty dir should not have dart files")
	}

	writeDartFile(t, dir, "main.dart", "void main() {}")

	if !HasDartFiles(dir) {
		t.Error("dir with .dart file should return true")
	}

	// Subdirectory
	subDir := filepath.Join(dir, "lib")
	os.MkdirAll(subDir, 0755)
	dir2 := t.TempDir()
	subDir2 := filepath.Join(dir2, "lib")
	os.MkdirAll(subDir2, 0755)
	writeDartFile(t, subDir2, "app.dart", "class App {}")

	if !HasDartFiles(dir2) {
		t.Error("dir with dart files in subdirectory should return true")
	}
}

func TestEnumWithMethods(t *testing.T) {
	source := `enum Color {
  red('Red'),
  green('Green'),
  blue('Blue');

  final String label;
  const Color(this.label);

  String display() => label;
}
`
	dir := t.TempDir()
	writeDartFile(t, dir, "color.dart", source)

	symbols, err := ParseFile(filepath.Join(dir, "color.dart"))
	if err != nil {
		t.Fatal(err)
	}

	// Should find: Color (enum), Color constructor, display method
	var enumSym, displaySym *Symbol
	for i, s := range symbols {
		if s.Name == "Color" && s.Kind == "enum" {
			enumSym = &symbols[i]
		}
		if s.Name == "display" && s.Kind == "method" {
			displaySym = &symbols[i]
		}
	}

	if enumSym == nil {
		t.Fatal("Color enum not found")
	}
	if displaySym == nil {
		t.Fatal("display method not found")
	}
	if displaySym.ParentName != "Color" {
		t.Errorf("display parent should be Color, got %q", displaySym.ParentName)
	}

	// Enum values (red, green, blue) should NOT appear as symbols
	for _, s := range symbols {
		if s.Name == "red" || s.Name == "green" || s.Name == "blue" {
			t.Errorf("enum value %q should not be detected as symbol", s.Name)
		}
	}
}

func TestParseDir(t *testing.T) {
	dir := t.TempDir()
	writeDartFile(t, dir, "main.dart", "void main() {}")
	sub := filepath.Join(dir, "lib")
	os.MkdirAll(sub, 0755)
	writeDartFile(t, sub, "app.dart", "class App {}\nclass Home {}")

	symbols, err := ParseDir(dir, 10)
	if err != nil {
		t.Fatal(err)
	}

	// main + App + Home = at least 3
	if len(symbols) < 3 {
		t.Errorf("expected at least 3 symbols, got %d", len(symbols))
	}
}

func TestStripForBraces(t *testing.T) {
	tests := []struct {
		input    string
		expected int // net braces
	}{
		{`class Foo {`, 1},
		{`}`, -1},
		{`String s = "hello {world}";`, 0},
		{`String s = 'test {';`, 0},
		{`void f() { // comment {`, 1},
		{`int x = 0;`, 0},
	}

	for _, tt := range tests {
		got := netBraces(tt.input)
		if got != tt.expected {
			t.Errorf("netBraces(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}
