// Copyright 2018 The Operator-SDK Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package projutil

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/rogpeppe/go-internal/modfile"
	log "github.com/sirupsen/logrus"

	kbutil "github.com/operator-framework/operator-sdk/internal/util/kubebuilder"
)

const (
	GoPathEnv  = "GOPATH"
	GoFlagsEnv = "GOFLAGS"
	GoModEnv   = "GO111MODULE"
	SrcDir     = "src"

	fsep              = string(filepath.Separator)
	mainFile          = "main.go"
	managerMainFile   = "cmd" + fsep + "manager" + fsep + mainFile
	buildDockerfile   = "build" + fsep + "Dockerfile"
	rolesDir          = "roles"
	requirementsFile  = "requirements.yml"
	moleculeDir       = "molecule"
	goModFile         = "go.mod"
	defaultPermission = 0644

	noticeColor = "\033[1;36m%s\033[0m"
)

// OperatorType - the type of operator
type OperatorType = string

const (
	// OperatorTypeGo - golang type of operator.
	OperatorTypeGo OperatorType = "go"
	// OperatorTypeAnsible - ansible type of operator.
	OperatorTypeAnsible OperatorType = "ansible"
	// OperatorTypeHelm - helm type of operator.
	OperatorTypeHelm OperatorType = "helm"
	// OperatorTypeUnknown - unknown type of operator.
	OperatorTypeUnknown OperatorType = "unknown"
)

type ErrUnknownOperatorType struct {
	Type string
}

func (e ErrUnknownOperatorType) Error() string {
	if e.Type == "" {
		return "unknown operator type"
	}
	return fmt.Sprintf(`unknown operator type "%v"`, e.Type)
}

// MustInProjectRoot checks if the current dir is the project root, and exits
// if not.
func MustInProjectRoot() {
	if err := CheckProjectRoot(); err != nil {
		log.Fatal(err)
	}
}

// CheckProjectRoot checks if the current dir is the project root, and returns
// an error if not.
// "build/Dockerfile" may not be present in all projects
// todo: scaffold Project file for Ansible and Helm with the type information
func CheckProjectRoot() error {
	if kbutil.HasProjectFile() {
		return nil
	}

	// todo(camilamacedo86): remove the following check when we no longer support the legacy scaffold layout
	// If the current directory has a "build/Dockerfile", then it is safe to say
	// we are at the project root.
	if _, err := os.Stat(buildDockerfile); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("must run command in project root dir: project structure requires %s",
				buildDockerfile)
		}
		return fmt.Errorf("error while checking if current directory is the project root: %v", err)
	}
	return nil
}

// TODO: remove this (should use os.Getwd() or Config.ProjectName).
func MustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get working directory: (%v)", err)
	}
	return wd
}

func getHomeDir() (string, error) {
	hd, err := homedir.Dir()
	if err != nil {
		return "", err
	}
	return homedir.Expand(hd)
}

// TODO(hasbro17): If this function is called in the subdir of
// a module project it will fail to parse go.mod and return
// the correct import path.
// This needs to be fixed to return the pkg import path for any subdir
// in order for `generate csv` to correctly form pkg imports
// for API pkg paths that are not relative to the root dir.
// This might not be fixable since there is no good way to
// get the project root from inside the subdir of a module project.
//
// GetGoPkg returns the current directory's import path by parsing it from
// wd if this project's repository path is rooted under $GOPATH/src, or
// from go.mod the project uses Go modules to manage dependencies.
// If the project has a go.mod then wd must be the project root.
//
// Example: "github.com/example-inc/app-operator"
func GetGoPkg() string {
	// Default to reading from go.mod, as it should usually have the (correct)
	// package path, and no further processing need be done on it if so.
	if _, err := os.Stat(goModFile); err != nil && !os.IsNotExist(err) {
		log.Fatalf("Failed to read go.mod: %v", err)
	} else if err == nil {
		b, err := ioutil.ReadFile(goModFile)
		if err != nil {
			log.Fatalf("Read go.mod: %v", err)
		}
		mf, err := modfile.Parse(goModFile, b, nil)
		if err != nil {
			log.Fatalf("Parse go.mod: %v", err)
		}
		if mf.Module != nil && mf.Module.Mod.Path != "" {
			return mf.Module.Mod.Path
		}
	}

	// Then try parsing package path from $GOPATH (set env or default).
	goPath, ok := os.LookupEnv(GoPathEnv)
	if !ok || goPath == "" {
		hd, err := getHomeDir()
		if err != nil {
			log.Fatal(err)
		}
		goPath = filepath.Join(hd, "go", "src")
	} else {
		// MustSetWdGopath is necessary here because the user has set GOPATH,
		// which could be a path list.
		goPath = MustSetWdGopath(goPath)
	}
	if !strings.HasPrefix(MustGetwd(), goPath) {
		log.Fatal("Could not determine project repository path: $GOPATH not set, wd in default $HOME/go/src," +
			" or wd does not contain a go.mod")
	}
	return parseGoPkg(goPath)
}

func parseGoPkg(gopath string) string {
	goSrc := filepath.Join(gopath, SrcDir)
	wd := MustGetwd()
	pathedPkg := strings.Replace(wd, goSrc, "", 1)
	// Make sure package only contains the "/" separator and no others, and
	// trim any leading/trailing "/".
	return strings.Trim(filepath.ToSlash(pathedPkg), "/")
}

// GetOperatorType returns type of operator is in cwd.
// This function should be called after verifying the user is in project root.
func GetOperatorType() OperatorType {
	switch {
	// TODO: Differentiate between legacy and KB Go projects
	case IsOperatorGo():
		return OperatorTypeGo
	case IsOperatorAnsible():
		return OperatorTypeAnsible
	case IsOperatorHelm():
		return OperatorTypeHelm
	}
	return OperatorTypeUnknown
}

// PluginKeyToOperatorType converts a plugin key string to an operator project
// type.
// TODO(estroz): this can probably be made more robust by checking known
// plugin keys directly.
func PluginKeyToOperatorType(pluginKey string) OperatorType {
	switch {
	case strings.HasPrefix(pluginKey, "go"):
		return OperatorTypeGo
	case strings.HasPrefix(pluginKey, "helm"):
		return OperatorTypeHelm
	case strings.HasPrefix(pluginKey, "ansible"):
		return OperatorTypeAnsible
	}
	return OperatorTypeUnknown
}

// IsOperatorGo returns true when the layout field in PROJECT file has the Go prefix key.
// NOTE: For the legacy, returns true when the project contains the cmd/manager directory and main.go file.
func IsOperatorGo() bool {
	// If the project has the new layout we will check the type in the config file
	if kbutil.HasProjectFile() {
		cfg, err := kbutil.ReadConfig()
		if err != nil {
			log.Fatalf("Error reading config: %v", err)
		}
		return cfg.IsV2() || PluginKeyToOperatorType(cfg.Layout) == OperatorTypeGo
	}

	// todo: remove the following code when the legacy layout is no longer supported
	// we can check it using the Project File
	_, err := os.Stat(managerMainFile)
	if err == nil || os.IsExist(err) {
		return true
	}
	// Aware of an alternative location for main.go.
	_, err = os.Stat(mainFile)
	return err == nil || os.IsExist(err)
}

// IsOperatorAnsible returns true when the layout field in PROJECT file has the Ansible prefix key.
// NOTE: For the legacy, returns true when the project  contains the roles and the molecule directory.
func IsOperatorAnsible() bool {
	// If the project is in the new layout, check the config file's plugin type.
	if kbutil.HasProjectFile() {
		cfg, err := kbutil.ReadConfig()
		if err != nil {
			log.Fatalf("Error reading config: %v", err)
		}
		return PluginKeyToOperatorType(cfg.Layout) == OperatorTypeAnsible
	}
	// todo(camilamacedo86): remove when the legacy layout is no longer supported
	stat, err := os.Stat(rolesDir)
	if (err == nil && stat.IsDir()) || os.IsExist(err) {
		return true
	}
	stat, err = os.Stat(moleculeDir)
	if (err == nil && stat.IsDir()) || os.IsExist(err) {
		return true
	}
	_, err = os.Stat(requirementsFile)
	return err == nil || os.IsExist(err)
}

// IsOperatorHelm returns true when the layout field in PROJECT file has the Helm prefix key.
func IsOperatorHelm() bool {
	if !kbutil.HasProjectFile() {
		return false
	}
	cfg, err := kbutil.ReadConfig()
	if err != nil {
		log.Fatalf("Error reading config: %v", err)
	}
	return PluginKeyToOperatorType(cfg.Layout) == OperatorTypeHelm
}

// MustSetWdGopath sets GOPATH to the first element of the path list in
// currentGopath that prefixes the wd, then returns the set path.
// If GOPATH cannot be set, MustSetWdGopath exits.
func MustSetWdGopath(currentGopath string) string {
	var (
		newGopath   string
		cwdInGopath bool
		wd          = MustGetwd()
	)
	for _, newGopath = range filepath.SplitList(currentGopath) {
		if strings.HasPrefix(filepath.Dir(wd), newGopath) {
			cwdInGopath = true
			break
		}
	}
	if !cwdInGopath {
		log.Fatalf("Project not in $GOPATH")
	}
	if err := os.Setenv(GoPathEnv, newGopath); err != nil {
		log.Fatal(err)
	}
	return newGopath
}

var flagRe = regexp.MustCompile("(.* )?-v(.* )?")

// SetGoVerbose sets GOFLAGS="${GOFLAGS} -v" if GOFLAGS does not
// already contain "-v" to make "go" command output verbose.
func SetGoVerbose() error {
	gf, ok := os.LookupEnv(GoFlagsEnv)
	if !ok || len(gf) == 0 {
		return os.Setenv(GoFlagsEnv, "-v")
	}
	if !flagRe.MatchString(gf) {
		return os.Setenv(GoFlagsEnv, gf+" -v")
	}
	return nil
}

// CheckGoModules ensures that go modules are enabled.
func CheckGoModules() error {
	goModOn, err := GoModOn()
	if err != nil {
		return err
	}
	if !goModOn {
		return fmt.Errorf(`using go modules requires GO111MODULE="on", "auto", or unset.` +
			` More info: https://sdk.operatorframework.io/docs/golang/quickstart/#a-note-on-dependency-management`)
	}
	return nil
}

// PrintDeprecationWarning prints a colored warning wrapping msg to the terminal.
func PrintDeprecationWarning(msg string) {
	fmt.Fprintf(os.Stderr, noticeColor, "[Deprecation Notice] "+msg+"\n")
}

// RewriteFileContents adds newContent to the line after the last occurrence of target in filename's contents,
// then writes the updated contents back to disk.
func RewriteFileContents(filename, target, newContent string) error {
	text, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("error in getting contents from the file, %v", err)
	}

	modifiedContent, err := appendContent(string(text), target, newContent)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(filename, []byte(modifiedContent), defaultPermission)
	if err != nil {
		return fmt.Errorf("error writing modified contents to file, %v", err)
	}
	return nil
}

func appendContent(fileContents, target, newContent string) (string, error) {
	labelIndex := strings.LastIndex(fileContents, target)
	if labelIndex == -1 {
		return "", fmt.Errorf("no prior string %s in newContent", target)
	}

	separationIndex := strings.Index(fileContents[labelIndex:], "\n")
	if separationIndex == -1 {
		return "", fmt.Errorf("no new line at the end of string %s", fileContents[labelIndex:])
	}

	index := labelIndex + separationIndex + 1
	return fileContents[:index] + newContent + fileContents[index:], nil

}
