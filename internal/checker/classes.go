package checker

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type classDefinition struct {
	Name    string
	Path    string
	Fields  map[string]struct{}
	Statics map[string]struct{}
	Methods map[string]struct{}
}

var (
	classDeclRE  = regexp.MustCompile(`(?i)^\s*(FIELD|STATIC)\s+(?:(?:PUBLIC|PRIVATE)\s+)?(.+)$`)
	methodDeclRE = regexp.MustCompile(`(?i)^\s*METHOD\s+([A-Za-z_][A-Za-z0-9_]*(?:[$#])?)\s*$`)
	useClassRE   = regexp.MustCompile(`(?i)^\s*USECLASS\s+(.+)$`)
	newClassRE   = regexp.MustCompile(`(?i)\bNEW\s*\(\s*([A-Za-z_][A-Za-z0-9_]*)`)
)

func loadClassLibraries(dirs []string) map[string]classDefinition {
	classes := make(map[string]classDefinition)
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".bas") {
				return nil
			}
			if class, ok := readClassDefinition(path); ok {
				classes[class.Name] = class
			}
			return nil
		})
	}
	return classes
}

func readClassDefinition(path string) (classDefinition, bool) {
	f, err := os.Open(path)
	if err != nil {
		return classDefinition{}, false
	}
	defer f.Close()
	class := classDefinition{
		Name: strings.ToUpper(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))), Path: path,
		Fields: map[string]struct{}{}, Statics: map[string]struct{}{}, Methods: map[string]struct{}{},
	}
	found := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if m := classDeclRE.FindStringSubmatch(line); m != nil {
			target := class.Fields
			if strings.EqualFold(m[1], "STATIC") {
				target = class.Statics
			}
			for _, raw := range strings.Split(m[2], ",") {
				name := strings.ToUpper(strings.TrimSpace(raw))
				if name != "" {
					target[name] = struct{}{}
				}
			}
			found = true
		} else if m := methodDeclRE.FindStringSubmatch(line); m != nil {
			class.Methods[strings.ToUpper(m[1])] = struct{}{}
			found = true
		}
	}
	return class, found
}

func (c *context) checkClassStatement(line int, statement string) bool {
	if m := useClassRE.FindStringSubmatch(statement); m != nil {
		for _, raw := range strings.Split(m[1], ",") {
			name := strings.ToUpper(strings.TrimSpace(raw))
			if name == "" {
				continue
			}
			if c.validateClasses {
				if _, exists := c.classes[name]; !exists {
					c.add(line, 1, Error, "KM6001", "クラス "+name+" がクラスライブラリにありません")
					continue
				}
			}
			c.usedClasses[name] = struct{}{}
		}
		return true
	}
	u := strings.ToUpper(strings.TrimSpace(statement))
	if m := classDeclRE.FindStringSubmatch(statement); m != nil {
		for _, raw := range strings.Split(m[2], ",") {
			name := strings.TrimSpace(raw)
			base := strings.ToUpper(strings.TrimSuffix(strings.TrimSuffix(name, "$"), "#"))
			if base != "" {
				c.declaredVars[base] = line
			}
		}
		return true
	}
	if strings.HasPrefix(u, "METHOD ") {
		if m := methodDeclRE.FindStringSubmatch(statement); m != nil {
			name := strings.ToUpper(m[1])
			if _, exists := c.labels[name]; !exists {
				c.labels[name] = line
			}
		}
		return true // クラスファイル内の宣言。通常変数として検査しない。
	}
	if u == "OPTION CLASSCODE" {
		return true
	}
	for _, m := range newClassRE.FindAllStringSubmatch(statement, -1) {
		name := strings.ToUpper(m[1])
		if c.validateClasses {
			if _, exists := c.classes[name]; !exists {
				c.add(line, 1, Error, "KM6001", "クラス "+name+" がクラスライブラリにありません")
				continue
			}
		}
		if _, used := c.usedClasses[name]; !used {
			c.add(line, 1, Error, "KM6002", "クラス "+name+" はNEWより前にUSECLASSで指定してください")
		}
	}
	return false
}
