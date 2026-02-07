package framework

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.starlark.net/starlark"
)

type SetupAPI interface {
	Set(key string, value any)
}

func RunSetup(stateDir string, api SetupAPI) error {
	path := filepath.Join(stateDir, "setup.star")
	
	// 1. Auto-Templating
	if _, err := os.Stat(path); os.IsNotExist(err) {
		examplePath := "setup.star.example"
		if _, err := os.Stat(examplePath); err == nil {
			copyFile(examplePath, path)
		}
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // No setup file, no problem
	}

	// Define built-in functions for the Starlark script
	globals := starlark.StringDict{
		"setup": starlark.NewBuiltin("setup", func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			for _, kwarg := range kwargs {
				key := string(kwarg[0].(starlark.String))
				val := kwarg[1]
				
				var goVal any
				switch v := val.(type) {
				case starlark.String:
					goVal = string(v)
				case starlark.Int:
					i, _ := v.Int64()
					goVal = i
				case starlark.Bool:
					goVal = bool(v)
				default:
					goVal = v.String()
				}
				api.Set(key, goVal)
			}
			return starlark.None, nil
		}),
	}

	thread := &starlark.Thread{Name: "setup"}
	_, err := starlark.ExecFile(thread, path, nil, globals)
	if err != nil {
		return fmt.Errorf("failed to execute setup.star: %v", err)
	}

	return nil
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()
	_, err = io.Copy(destination, source)
	return err
}