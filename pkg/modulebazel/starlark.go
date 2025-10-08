package modulebazel

import (
	"fmt"

	"go.starlark.net/starlark"
)

type goStarlarkFunction func(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error)

// Symbol is the type of a Starlark constructor symbol.  It prints more
// favorably than a starlark.String.
type Symbol string

func (s Symbol) String() string             { return string(s) }
func (s Symbol) GoString() string           { return string(s) }
func (s Symbol) Type() string               { return "symbol" }
func (s Symbol) Freeze()                    {} // immutable
func (s Symbol) Truth() starlark.Bool       { return len(s) > 0 }
func (s Symbol) Hash() (uint32, error)      { return starlark.String(s).Hash() }
func (s Symbol) Len() int                   { return len(s) } // bytes
func (s Symbol) Index(i int) starlark.Value { return s[i : i+1] }

func loadStarlarkProgram(filename string, src any, predeclared *permissiveStringDict, reporter func(msg string), errorReporter func(err error)) (*starlark.StringDict, *starlark.Thread, error) {
	newErrorf := func(msg string, args ...any) error {
		err := fmt.Errorf(filename+": "+msg, args...)
		errorReporter(err)
		return err
	}

	// Capture which names are referenced during parsing
	var referencedNames []string
	hasFunc := func(name string) bool {
		referencedNames = append(referencedNames, name)
		return predeclared.Has(name)
	}

	_, program, err := starlark.SourceProgram(filename, src, hasFunc)
	if err != nil {
		return nil, nil, newErrorf("source program error: %v", err)
	}

	// Build a complete StringDict with shims for all referenced names
	completeDict := make(starlark.StringDict)
	for k, v := range predeclared.StringDict {
		completeDict[k] = v
	}
	for _, name := range referencedNames {
		if _, ok := completeDict[name]; !ok {
			completeDict[name] = predeclared.Get(name)
		}
	}

	thread := new(starlark.Thread)
	thread.Print = func(thread *starlark.Thread, msg string) {
		reporter(msg)
	}
	globals, err := program.Init(thread, completeDict)
	if err != nil {
		return nil, nil, newErrorf("eval: %w", err)
	}

	return &globals, thread, nil
}

// Use a permissive predeclared dict with a catch-all so we don't have to define
// all callable symbols
type permissiveStringDict struct {
	starlark.StringDict
}

func (d *permissiveStringDict) Has(name string) bool {
	// Always return true - we can provide a shim for any name
	return true
}

func (d *permissiveStringDict) Get(name string) starlark.Value {
	if v, ok := d.StringDict[name]; ok {
		return v
	}
	// Return a no-op builtin for any undefined name
	return starlark.NewBuiltin(name, func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	})
}
