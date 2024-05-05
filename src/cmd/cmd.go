package cmd

import (
	"errors"
	"path"
	"reflect"
	"runtime"
	"strings"
)

type CmdRename struct {
	Commit bool `help:"really rename files, not just do a dry run"`
}

type CmdMerge struct {
	OutputDirPath string `arg:"--output-dir,required" help:"directory to output merged GoPro video files"`
}

type CmdRoot struct {
	Rename       *CmdRename `arg:"subcommand:rename" help:"rename GoPro video files"`
	Merge        *CmdMerge  `arg:"subcommand:merge" help:"merge GoPro video files"`
	InputDirPath string     `arg:"--input-dir,required" help:"directory containing GoPro video files"`
}

func isSubcommand(s reflect.StructField) bool {

	tag, ok := s.Tag.Lookup("arg")
	if !ok {
		return false
	}

	tagSplit := strings.Split(tag, ",")

	// For each subtag in "arg"...
	for _, subtag := range tagSplit {

		// Split current subtag by ":"
		subtagParts := strings.Split(subtag, ":")

		// Skip if subtag does not have minimum parts or first part is not "subcommand"
		if len(subtagParts) < 2 || subtagParts[0] != "subcommand" {
			continue
		}

		return true

	}

	return false
}

// Run callback on each struct's field
func forField(base reflect.Value, fieldCallback func(f reflect.StructField, v reflect.Value)) {

	// Exit early if base is not a struct
	if base.Type().Kind() != reflect.Struct {
		return
	}

	// For each struct field...
	for i := 0; i < base.Type().NumField(); i++ {

		s := base.Type().Field(i)
		v := base.FieldByName(s.Name)

		fieldCallback(s, v)

	}

}

// Convert pointer-to-struct into a struct; does nothing if already a struct.
func toStruct(base *reflect.Value) error {

	// Do nothing if already a struct
	if base.Type().Kind() == reflect.Struct {
		return nil
	}

	// Error if not a pointer
	if base.Type().Kind() != reflect.Pointer {
		return errors.New("expected pointer")
	}

	// Error if pointer is nil
	if base.IsNil() {
		return errors.New("pointer is nil")
	}

	// Dereference pointer
	deref := base.Elem()

	// Error if what was dereferenced is not a struct
	if deref.Type().Kind() != reflect.Struct {
		return errors.New("pointer is not for a struct")
	}

	*base = deref

	return nil

}

// Cleanup app struct fields with suffix "Dir" or "File"
func cleanDir(base reflect.Value) {

	// Convert to struct, exit early if unable.
	if err := toStruct(&base); err != nil {
		return
	}

	cleanSubcommands := []func(){}

	// For each field in struct...
	forField(base, func(s reflect.StructField, v reflect.Value) {

		// Skip if not exported
		if !s.IsExported() {
			return
		}

		isSettableString := v.CanSet() && v.Type().Kind() == reflect.String
		isPath := strings.HasSuffix(s.Name, "Path")

		// Cleanup field string if settable and field name has "Path" suffix
		if isSettableString && isPath {

			vStr := v.Interface().(string)

			// Workaround for trailing quote on windows
			if runtime.GOOS == "windows" {
				vStr = strings.TrimSuffix(vStr, "\"")
			}

			v.SetString(path.Clean(vStr))

		}

		// Create func for cleaning up sub-paths if field is a subcommand
		if isSubcommand(s) {
			cleanSubcommands = append(cleanSubcommands, func() {
				cleanDir(v)
			})
		}

	})

	// Re-run cleanDir for other fields
	for _, cleanSubcommand := range cleanSubcommands {
		cleanSubcommand()
	}

}

// Verify existence of a walkable tree of subcommands picked by the user.
func verifyCommandTree(base reflect.Value) error {

	next := reflect.Value{}

	// Convert to struct, exit early if unable.
	if err := toStruct(&base); err != nil {
		return nil
	}

	// Count of subcommands in current struct
	containsSubcommand := false

	pickedSubcommand := false

	// For each field in struct...
	forField(base, func(s reflect.StructField, v reflect.Value) {

		// Skip if already found subcommand
		if pickedSubcommand {
			return
		}

		// Skip if not exported
		if !s.IsExported() {
			return
		}

		// Skip if not a pointer
		if v.Type().Kind() != reflect.Pointer {
			return
		}

		// Skip if not a subcommand
		if !isSubcommand(s) {
			return
		}

		containsSubcommand = true

		// Skip if nil, meaning user did not pick this specific subcommand
		if v.IsNil() {
			return
		}

		// If finally here, then user picked this field as the subcommand
		pickedSubcommand = true
		next = v.Elem()

	})

	// If struct has no subcommands, that's fine; exit early.
	if !containsSubcommand {
		return nil
	}

	// Error if struct contains subcommand but user did not pick one.
	if !pickedSubcommand {
		return errors.New("no subcommand was picked")
	}

	// Re-run verifier on subcommand's target.
	if err := verifyCommandTree(next); pickedSubcommand && err != nil {
		return err
	}

	return nil
}

// Run post process funcs for command structure
func (r CmdRoot) PostProcess() error {

	// Cleanup path flags
	rv := reflect.ValueOf(&r)
	cleanDir(rv)

	// Verify walkable tree of subcommands have been picked by the user
	next := reflect.ValueOf(r)
	if err := verifyCommandTree(next); err != nil {
		return err
	}

	return nil

}
