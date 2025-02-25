package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/Masterminds/semver/v3"
	"github.com/fatih/camelcase"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stoewer/go-strcase"
)

type service struct {
	Spec *openapi3.T
	Name string
	//  overwrite import name of service when it's a keyword ie function in javascript
	ImportName string
}

type example struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	Request      map[string]interface{}
	Response     map[string]interface{}
	RunCheck     bool   `json:"run_check"`
	Idempotent   bool   `json:"idempotent"`
	ShellRequest string `json:"shell_request"`
}

var (
	VanityURL = "go.m3o.com"
)

func funcMap() map[string]interface{} {
	isStream := func(spec *openapi3.T, serviceName, requestType string) bool {
		// eg. "/notes/Notes/Events":
		path := fmt.Sprintf("/%v/%v/%v", serviceName, strings.Title(serviceName), strings.Replace(requestType, "Request", "", -1))
		var p *openapi3.PathItem
		for k, v := range spec.Paths {
			if strings.ToLower(k) == strings.ToLower(path) {
				p = v
			}
		}
		if p == nil {
			panic("path not found: " + path)
		}
		if _, ok := p.Post.Responses["stream"]; ok {
			return true
		}
		return false
	}
	return map[string]interface{}{
		"isCustomShell": func(ex example) bool {
			return len(ex.ShellRequest) > 0
		},
		"recursiveTypeDefinition": func(language, serviceName, typeName string, schemas map[string]*openapi3.SchemaRef) string {
			return schemaToType(language, serviceName, typeName, schemas)
		},
		"requestTypeToEndpointName": func(requestType string) string {
			parts := camelcase.Split(requestType)
			return strings.Join(parts[1:len(parts)-1], "")
		},
		// strips service name from the request type
		"requestType": func(requestType string) string {
			// @todo hack to support examples
			if strings.ToLower(requestType[0:1]) == requestType[0:1] {
				return strings.ToTitle(requestType[0:1]) + requestType[1:] + "Request"
			}
			parts := camelcase.Split(requestType)
			return strings.Join(parts[1:], "")
		},
		"isStream": isStream,
		"isNotStream": func(spec *openapi3.T, serviceName, requestType string) bool {
			return !isStream(spec, serviceName, requestType)
		},
		"requestTypeToResponseType": func(requestType string) string {
			parts := camelcase.Split(requestType)
			return strings.Join(parts[1:len(parts)-1], "") + "Response"
		},
		"endpointComment": func(endpoint string, schemas map[string]*openapi3.SchemaRef) string {
			v := schemas[strings.Title(endpoint)+"Request"]
			if v == nil {
				panic("can't find " + strings.Title(endpoint) + "Request")
			}
			if v.Value == nil {
				return ""
			}
			comm := v.Value.Description
			ret := ""
			for _, line := range strings.Split(comm, "\n") {
				ret += "// " + strings.TrimSpace(line) + "\n"
			}
			return ret
		},
		// @todo same function as above
		"endpointDescription": func(endpoint string, schemas map[string]*openapi3.SchemaRef) string {
			v := schemas[strings.Title(endpoint)+"Request"]
			if v == nil {
				panic("can't find " + strings.Title(endpoint) + "Request")
			}
			if v.Value == nil {
				return ""
			}
			comm := v.Value.Description
			ret := ""
			for _, line := range strings.Split(comm, "\n") {
				ret += strings.TrimSpace(line) + "\n"
			}
			return ret
		},
		"requestTypeToEndpointPath": func(requestType string) string {
			parts := camelcase.Split(requestType)
			return strings.Title(strings.Join(parts[1:len(parts)-1], ""))
		},
		"title": strings.Title,
		"untitle": func(t string) string {
			return strcase.LowerCamelCase(t)
		},
		"goExampleRequest": func(serviceName, endpoint string, schemas map[string]*openapi3.SchemaRef, exampleJSON map[string]interface{}) string {
			return schemaToGoExample(serviceName, strings.Title(endpoint)+"Request", schemas, exampleJSON)
		},
		"tsExampleRequest": func(serviceName, endpoint string, schemas map[string]*openapi3.SchemaRef, exampleJSON map[string]interface{}) string {
			bs, _ := json.MarshalIndent(exampleJSON, "", "  ")
			return string(bs)
		},
	}
}

func nodeServiceClient(serviceName, tsPath string, service service) {
	templ, err := template.New("ts" + serviceName).Funcs(funcMap()).Parse(tsServiceTemplate)
	if err != nil {
		fmt.Println("Failed to unmarshal", err)
		os.Exit(1)
	}
	var b bytes.Buffer
	buf := bufio.NewWriter(&b)
	err = templ.Execute(buf, map[string]interface{}{
		"service": service,
	})
	if err != nil {
		fmt.Println("Failed to unmarshal", err)
		os.Exit(1)
	}

	err = os.MkdirAll(filepath.Join(tsPath, "src", serviceName), 0777)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	f, err := os.OpenFile(filepath.Join(tsPath, "src", serviceName, "index.ts"), os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0744)
	if err != nil {
		fmt.Println("Failed to open schema file", err)
		os.Exit(1)
	}
	buf.Flush()
	_, err = f.Write(b.Bytes())
	if err != nil {
		fmt.Println("Failed to append to schema file", err)
		os.Exit(1)
	}

	cmd := exec.Command("prettier", "-w", "index.ts")
	cmd.Dir = filepath.Join(tsPath, "src", serviceName)
	outp, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println(fmt.Sprintf("Problem formatting '%v' client: %v %s", serviceName, string(outp), err.Error()))
		os.Exit(1)
	}
}

func nodeTopReadme(serviceName, examplesPath string, service service) {
	// node client service readmes
	templ, err := template.New("tsTopReadme" + serviceName).Funcs(funcMap()).Parse(tsReadmeTopTemplate)
	if err != nil {
		fmt.Println("Failed to unmarshal", err)
		os.Exit(1)
	}
	b := bytes.Buffer{}
	buf := bufio.NewWriter(&b)
	err = templ.Execute(buf, map[string]interface{}{
		"service": service,
	})
	if err != nil {
		fmt.Println("Failed to unmarshal", err)
		os.Exit(1)
	}
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	os.MkdirAll(filepath.Join(examplesPath, "js", serviceName), 0744)
	f, err := os.OpenFile(filepath.Join(examplesPath, "js", serviceName, "README.md"), os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0744)
	if err != nil {
		fmt.Println("Failed to open schema file", err)
		os.Exit(1)
	}
	buf.Flush()
	_, err = f.Write(b.Bytes())
	if err != nil {
		fmt.Println("Failed to append to schema file", err)
		os.Exit(1)
	}
}

func apiSpec(serviceFiles []os.FileInfo, serviceDir string) (*openapi3.T, bool) {
	// detect openapi json file
	apiJSON := ""
	skip := false
	for _, serviceFile := range serviceFiles {
		if strings.Contains(serviceFile.Name(), "api") && strings.Contains(serviceFile.Name(), "-") && strings.HasSuffix(serviceFile.Name(), ".json") {
			apiJSON = filepath.Join(serviceDir, serviceFile.Name())
		}
		if serviceFile.Name() == "skip" {
			skip = true
		}
	}
	if skip {
		return nil, true
	}

	fmt.Println("Processing folder", serviceDir, "api json", apiJSON)

	js, err := ioutil.ReadFile(apiJSON)

	if err != nil {
		fmt.Println("Failed to read json spec", err)
		os.Exit(1)
	}
	spec := &openapi3.T{}
	err = json.Unmarshal(js, &spec)
	if err != nil {
		fmt.Println("Failed to unmarshal", err)
		os.Exit(1)
	}
	return spec, false
}

func goServiceClient(serviceName, goPath string, service service) {
	templ, err := template.New("go" + serviceName).Funcs(funcMap()).Parse(goServiceTemplate)
	if err != nil {
		fmt.Println("Failed to unmarshal", err)
		os.Exit(1)
	}
	b := bytes.Buffer{}
	buf := bufio.NewWriter(&b)
	err = templ.Execute(buf, map[string]interface{}{
		"service": service,
	})
	if err != nil {
		fmt.Println("Failed to unmarshal", err)
		os.Exit(1)
	}
	err = os.MkdirAll(filepath.Join(goPath, serviceName), 0777)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	goClientFile := filepath.Join(goPath, serviceName, serviceName+".go")
	f, err := os.OpenFile(goClientFile, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0744)
	if err != nil {
		fmt.Println("Failed to open schema file", err)
		os.Exit(1)
	}
	buf.Flush()
	_, err = f.Write(b.Bytes())
	if err != nil {
		fmt.Println("Failed to append to schema file", err)
		os.Exit(1)
	}
}

func goTopReadme(serviceName, examplesPath string, service service) {
	templ, err := template.New("goTopReadme" + serviceName).Funcs(funcMap()).Parse(goReadmeTopTemplate)
	if err != nil {
		fmt.Println("Failed to unmarshal", err)
		os.Exit(1)
	}
	b := bytes.Buffer{}
	buf := bufio.NewWriter(&b)
	err = templ.Execute(buf, map[string]interface{}{
		"service": service,
	})
	if err != nil {
		fmt.Println("Failed to unmarshal", err)
		os.Exit(1)
	}
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	os.MkdirAll(filepath.Join(examplesPath, "go", serviceName), 0744)
	f, err := os.OpenFile(filepath.Join(examplesPath, "go", serviceName, "README.md"), os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0744)
	if err != nil {
		fmt.Println("Failed to open schema file", err)
		os.Exit(1)
	}
	buf.Flush()
	_, err = f.Write(b.Bytes())
	if err != nil {
		fmt.Println("Failed to append to schema file", err)
		os.Exit(1)
	}
}

func goExampleAndReadmeEdit(examplesPath, serviceName, endpoint, title string, service service, example example) {
	templ, err := template.New("go" + serviceName + endpoint).Funcs(funcMap()).Parse(goExampleTemplate)
	if err != nil {
		fmt.Println("Failed to unmarshal", err)
		os.Exit(1)
	}
	b := bytes.Buffer{}
	buf := bufio.NewWriter(&b)
	err = templ.Execute(buf, map[string]interface{}{
		"service":  service,
		"example":  example,
		"endpoint": endpoint,
		"funcName": strcase.UpperCamelCase(title),
	})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// create go examples directory
	err = os.MkdirAll(filepath.Join(examplesPath, "go", serviceName, endpoint, title), 0777)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	goExampleFile := filepath.Join(examplesPath, "go", serviceName, endpoint, title, "main.go")
	f, err := os.OpenFile(goExampleFile, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0744)
	if err != nil {
		fmt.Println("Failed to open schema file", err)
		os.Exit(1)
	}

	buf.Flush()
	_, err = f.Write(b.Bytes())
	if err != nil {
		fmt.Println("Failed to append to schema file", err)
		os.Exit(1)
	}

	if example.RunCheck && example.Idempotent {
		err = ioutil.WriteFile(filepath.Join(examplesPath, "go", serviceName, endpoint, title, ".run"), []byte{}, 0744)
		if err != nil {
			fmt.Println("Failed to write run file", err)
			os.Exit(1)
		}
	}

	// per endpoint go readme examples
	templ, err = template.New("goReadmebottom" + serviceName + endpoint).Funcs(funcMap()).Parse(goReadmeBottomTemplate)
	if err != nil {
		fmt.Println("Failed to unmarshal", err)
		os.Exit(1)
	}
	b = bytes.Buffer{}
	buf = bufio.NewWriter(&b)
	err = templ.Execute(buf, map[string]interface{}{
		"service":  service,
		"example":  example,
		"endpoint": endpoint,
		"funcName": strcase.UpperCamelCase(title),
	})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	goReadmeAppend := filepath.Join(examplesPath, "go", serviceName, "README.md")
	f, err = os.OpenFile(goReadmeAppend, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0744)
	if err != nil {
		fmt.Println("Failed to open schema file", err)
		os.Exit(1)
	}

	buf.Flush()
	_, err = f.Write(b.Bytes())
	if err != nil {
		fmt.Println("Failed to append to schema file", err)
		os.Exit(1)
	}
}

func nodeExampleAndReadmeEdit(examplesPath, serviceName, endpoint, title string, service service, example example) {
	// node example
	templ, err := template.New("ts" + serviceName + endpoint).Funcs(funcMap()).Parse(tsExampleTemplate)
	if err != nil {
		fmt.Println("Failed to unmarshal", err)
		os.Exit(1)
	}
	b := bytes.Buffer{}
	buf := bufio.NewWriter(&b)
	err = templ.Execute(buf, map[string]interface{}{
		"service":  service,
		"example":  example,
		"endpoint": endpoint,
		"funcName": strcase.UpperCamelCase(title),
	})

	err = os.MkdirAll(filepath.Join(examplesPath, "js", serviceName, endpoint), 0777)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	tsExampleFile := filepath.Join(examplesPath, "js", serviceName, endpoint, title+".js")
	f, err := os.OpenFile(tsExampleFile, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0744)
	if err != nil {
		fmt.Println("Failed to open schema file", err)
		os.Exit(1)
	}

	buf.Flush()
	_, err = f.Write(b.Bytes())
	if err != nil {
		fmt.Println("Failed to append to schema file", err)
		os.Exit(1)
	}

	if example.RunCheck && example.Idempotent {
		err = ioutil.WriteFile(filepath.Join(examplesPath, "js", serviceName, endpoint, ".run"+strcase.UpperCamelCase(title)), []byte{}, 0744)
		if err != nil {
			fmt.Println("Failed to write run file", err)
			os.Exit(1)
		}
	}

	// per endpoint readme examples
	templ, err = template.New("tsBottomReadme" + serviceName + endpoint).Funcs(funcMap()).Parse(tsReadmeBottomTemplate)
	if err != nil {
		fmt.Println("Failed to unmarshal", err)
		os.Exit(1)
	}
	b = bytes.Buffer{}
	buf = bufio.NewWriter(&b)
	err = templ.Execute(buf, map[string]interface{}{
		"service":  service,
		"example":  example,
		"endpoint": endpoint,
		"funcName": strcase.UpperCamelCase(title),
	})

	tsReadmeAppend := filepath.Join(examplesPath, "js", serviceName, "README.md")
	f, err = os.OpenFile(tsReadmeAppend, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0744)
	if err != nil {
		fmt.Println("Failed to open schema file", err)
		os.Exit(1)
	}

	buf.Flush()
	_, err = f.Write(b.Bytes())
	if err != nil {
		fmt.Println("Failed to append to schema file", err)
		os.Exit(1)
	}

	cmd := exec.Command("prettier", "-w", title+".js")
	cmd.Dir = filepath.Join(examplesPath, "js", serviceName, endpoint)
	outp, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println(fmt.Sprintf("Problem with '%v' example '%v': %v", serviceName, endpoint, string(outp)))
		os.Exit(1)
	}
}

func curlExample(examplesPath, serviceName, endpoint, title string, service service, example example) {
	// curl example
	templ, err := template.New("curl" + serviceName + endpoint).Funcs(funcMap()).Parse(curlExampleTemplate)
	if err != nil {
		fmt.Println("Failed to unmarshal", err)
		os.Exit(1)
	}
	b := bytes.Buffer{}
	buf := bufio.NewWriter(&b)
	err = templ.Execute(buf, map[string]interface{}{
		"service":  service,
		"example":  example,
		"endpoint": endpoint,
		"funcName": strcase.UpperCamelCase(title),
	})

	err = os.MkdirAll(filepath.Join(examplesPath, "curl", serviceName, endpoint), 0777)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	curlExampleFile := filepath.Join(examplesPath, "curl", serviceName, endpoint, title+".sh")
	f, err := os.OpenFile(curlExampleFile, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0744)
	if err != nil {
		fmt.Println("Failed to open schema file", err)
		os.Exit(1)
	}

	buf.Flush()
	_, err = f.Write(b.Bytes())
	if err != nil {
		fmt.Println("Failed to append to schema file", err)
		os.Exit(1)
	}
}

func goIndexFile(goPath string, services []service) {
	templ, err := template.New("goclient").Funcs(funcMap()).Parse(goIndexTemplate)
	if err != nil {
		fmt.Println("Failed to unmarshal", err)
		os.Exit(1)
	}
	b := bytes.Buffer{}
	buf := bufio.NewWriter(&b)
	err = templ.Execute(buf, map[string]interface{}{
		"services": services,
	})
	if err != nil {
		fmt.Println("Failed to unmarshal", err)
		os.Exit(1)
	}
	f, err := os.OpenFile(filepath.Join(goPath, "m3o.go"), os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0744)
	if err != nil {
		fmt.Println("Failed to open schema file", err)
		os.Exit(1)
	}
	buf.Flush()
	_, err = f.Write(b.Bytes())
	if err != nil {
		fmt.Println("Failed to append to schema file", err)
		os.Exit(1)
	}
}

func nodeIndexFile(workDir, tsPath string, services []service) {
	// add file list to gitignore
	f, err := os.OpenFile(filepath.Join(tsPath, ".gitignore"), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0744)
	//for _, sname := range tsFileList {
	//	_, err := f.Write([]byte(sname + "\n"))
	//	if err != nil {
	//		fmt.Println("failed to append service to gitignore", err)
	//		os.Exit(1)
	//	}
	//}

	templ, err := template.New("tsclient").Funcs(funcMap()).Parse(tsIndexTemplate)
	if err != nil {
		fmt.Println("Failed to unmarshal", err)
		os.Exit(1)
	}
	var b bytes.Buffer
	buf := bufio.NewWriter(&b)
	err = templ.Execute(buf, map[string]interface{}{
		"services": services,
	})
	if err != nil {
		fmt.Println("Failed to unmarshal", err)
		os.Exit(1)
	}

	f, err = os.OpenFile(filepath.Join(tsPath, "src", "index.ts"), os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0744)
	if err != nil {
		fmt.Println("Failed to open schema file", err)
		os.Exit(1)
	}
	buf.Flush()
	_, err = f.Write(b.Bytes())
	if err != nil {
		fmt.Println("Failed to append to schema file", err)
		os.Exit(1)
	}
	cmd := exec.Command("prettier", "-w", "index.ts")
	cmd.Dir = filepath.Join(tsPath, "src")
	outp, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println(fmt.Sprintf("Problem with prettifying clients index.ts '%v", string(outp)))
		os.Exit(1)
	}
	tsFiles := filepath.Join(workDir, "cmd", "client-generator", "ts")
	cmd = exec.Command("cp", filepath.Join(tsFiles, "package.json"), filepath.Join(tsFiles, ".gitignore"),
		filepath.Join(tsFiles, "package-lock.json"), filepath.Join(tsFiles, "package-lock.json"),
		filepath.Join(tsFiles, "build.js"), filepath.Join(tsFiles, "tsconfig.es.json"),
		filepath.Join(tsFiles, "package-lock.json"), filepath.Join(tsFiles, "tsconfig.json"),
		filepath.Join(tsFiles, "README.md"), filepath.Join(workDir, "clients", "ts"))

	outp, err = cmd.CombinedOutput()
	if err != nil {
		fmt.Println(fmt.Sprintf("Problem with prettifying clients index.ts '%v", string(outp)))
		os.Exit(1)
	}
}

func publishToNpm(tsPath string, tsFileList []string) {
	// login to NPM
	f, err := os.OpenFile(filepath.Join(tsPath, ".npmrc"), os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		fmt.Println("Failed to open npmrc", err)
		os.Exit(1)
	}

	defer f.Close()
	if len(os.Getenv("NPM_TOKEN")) == 0 {
		fmt.Println("No NPM_TOKEN env found")
		os.Exit(1)
	}
	if _, err = f.WriteString("//registry.npmjs.org/:_authToken=" + os.Getenv("NPM_TOKEN")); err != nil {
		fmt.Println("Failed to open npmrc", err)
		os.Exit(1)
	}

	// get latest version from github
	getVersions := exec.Command("npm", "show", "m3o", "--time", "--json")
	getVersions.Dir = tsPath

	outp, err := getVersions.CombinedOutput()
	if err != nil {
		fmt.Println("Failed to get versions of NPM package", string(outp))
		os.Exit(1)
	}
	type npmVers struct {
		Versions []string `json:"versions"`
	}

	beta := os.Getenv("IS_BETA") != ""
	if beta {
		fmt.Println("creating beta version")
	} else {
		fmt.Println("creating live version")
	}

	npmOutput := &npmVers{}
	var latest *semver.Version
	if len(outp) > 0 {
		err = json.Unmarshal(outp, npmOutput)
		if err != nil {
			fmt.Println("Failed to unmarshal versions", string(outp))
			os.Exit(1)
		}
	}
	fmt.Println("npm output version: ", npmOutput.Versions)

	for _, version := range npmOutput.Versions {
		v, err := semver.NewVersion(version)
		if err != nil {
			fmt.Println("Failed to parse semver", err)
			os.Exit(1)
		}
		if latest == nil {
			latest = v
		}
		if v.GreaterThan(latest) {
			latest = v
		}

	}

	if latest == nil {
		fmt.Println("found no semver version")
		os.Exit(1)
	}

	var newV semver.Version
	if beta {
		// bump a beta version
		if strings.Contains(latest.String(), "beta") {
			newV = incBeta(*latest)
		} else {
			// make beta out of latest non beta version
			v, _ := semver.NewVersion(latest.IncPatch().String() + "-beta1")
			newV = *v
		}
	} else {
		newV = latest.IncPatch()
	}

	// bump package to latest version
	fmt.Println("Bumping to ", newV.String())
	repl := exec.Command("sed", "-i", "-e", "s/1.0.1/"+newV.String()+"/g", "package.json")
	repl.Dir = tsPath
	outp, err = repl.CombinedOutput()
	if err != nil {
		fmt.Println("Failed to make docs", string(outp))
		os.Exit(1)
	}

	// apppend exports to to package.json
	pak, err := ioutil.ReadFile(filepath.Join(tsPath, "package.json"))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	m := map[string]interface{}{}
	err = json.Unmarshal(pak, &m)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	m["files"] = tsFileList
	pakJS, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	f, err = os.OpenFile(filepath.Join(tsPath, "package.json"), os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0744)
	if err != nil {
		fmt.Println("Failed to open package.json", err)
		os.Exit(1)
	}
	_, err = f.Write(pakJS)
	if err != nil {
		fmt.Println("Failed to write to package.json", err)
		os.Exit(1)
	}
}

func main() {

	serviceFlag := flag.String("service", "", "the service dir to process")
	flag.Parse()

	files, err := ioutil.ReadDir(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}
	workDir, _ := os.Getwd()
	tsPath := filepath.Join(workDir, "clients", "ts")
	err = os.MkdirAll(tsPath, 0777)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	goPath := filepath.Join(workDir, "clients", "go")
	err = os.MkdirAll(goPath, 0777)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	examplesPath := filepath.Join(workDir, "examples")
	err = os.MkdirAll(goPath, 0777)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	services := []service{}
	tsFileList := []string{"esm", "index.js", "index.d.ts"}
	for _, f := range files {
		if len(*serviceFlag) > 0 && f.Name() != *serviceFlag {
			continue
		}
		if strings.Contains(f.Name(), "clients") || strings.Contains(f.Name(), "examples") {
			continue
		}
		if f.IsDir() && !strings.HasPrefix(f.Name(), ".") {
			serviceName := f.Name()
			serviceDir := filepath.Join(workDir, f.Name())
			cmd := exec.Command("make", "api")
			cmd.Dir = serviceDir
			outp, err := cmd.CombinedOutput()
			if err != nil {
				fmt.Println(string(outp))
			}

			serviceFiles, err := ioutil.ReadDir(serviceDir)
			if err != nil {
				fmt.Println("Failed to read service dir", err)
				os.Exit(1)
			}
			skip := false

			spec, skip := apiSpec(serviceFiles, serviceDir)
			if skip {
				continue
			}
			tsFileList = append(tsFileList, f.Name())
			service := service{
				Name:       serviceName,
				ImportName: serviceName,
				Spec:       spec,
			}
			if service.Name == "function" {
				service.ImportName = "fx"
			}
			services = append(services, service)

			nodeServiceClient(serviceName, tsPath, service)
			nodeTopReadme(serviceName, examplesPath, service)

			goServiceClient(serviceName, goPath, service)
			goTopReadme(serviceName, examplesPath, service)

			exam, err := ioutil.ReadFile(filepath.Join(workDir, serviceName, "examples.json"))
			if err != nil {
				// try read config/examples.json
				exam, err = ioutil.ReadFile(filepath.Join(workDir, serviceName, "config", "examples.json"))
			}
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			m := map[string][]example{}
			err = json.Unmarshal(exam, &m)
			if err != nil {
				fmt.Println(string(exam), err)
				os.Exit(1)
			}
			if len(service.Spec.Paths) != len(m) {
				fmt.Printf("Service has %v endpoints, but only %v examples\n", len(service.Spec.Paths), len(m))
			}
			for endpoint, examples := range m {
				for _, example := range examples {
					title := regexp.MustCompile("[^a-zA-Z0-9]+").ReplaceAllString(strcase.LowerCamelCase(strings.Replace(example.Title, " ", "_", -1)), "")

					goExampleAndReadmeEdit(examplesPath, serviceName, endpoint, title, service, example)
					nodeExampleAndReadmeEdit(examplesPath, serviceName, endpoint, title, service, example)
					curlExample(examplesPath, serviceName, endpoint, title, service, example)
				}
			}
		}
	}

	nodeIndexFile(workDir, tsPath, services)
	goIndexFile(goPath, services)

	publishToNpm(tsPath, tsFileList)
}

func incBeta(ver semver.Version) semver.Version {
	s := ver.String()
	parts := strings.Split(s, "beta")
	if len(parts) < 2 {
		panic("not a beta version " + s)
	}
	i, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		panic(err)
	}
	i++
	v, err := semver.NewVersion(parts[0] + "beta" + fmt.Sprintf("%v", i))
	if err != nil {
		panic(err)
	}
	return *v
}

func schemaToType(language, serviceName, typeName string, schemas map[string]*openapi3.SchemaRef) string {
	var recurse func(props map[string]*openapi3.SchemaRef, level int) string

	var spec *openapi3.SchemaRef = schemas[typeName]
	detectType := func(currentType string, properties map[string]*openapi3.SchemaRef) (string, bool) {
		index := map[string]bool{}
		for key, prop := range properties {
			index[key+prop.Value.Title+prop.Value.Description] = true
		}

		for k, schema := range schemas {
			// we don't want to return the type matching itself
			if strings.ToLower(k) == currentType {
				continue
			}
			if strings.HasSuffix(k, "Request") || strings.HasSuffix(k, "Response") {
				continue
			}
			if len(schema.Value.Properties) != len(properties) {
				continue
			}
			found := false
			for key, prop := range schema.Value.Properties {
				_, ok := index[key+prop.Value.Title+prop.Value.Description]
				found = ok
				if !ok {
					break
				}
			}
			if found {
				return schema.Value.Title, true
			}
		}
		return "", false
	}
	var fieldSeparator, arrayPrefix, arrayPostfix, fieldDelimiter, stringType, numberType, boolType string
	var int32Type, int64Type, floatType, doubleType, mapType, anyType, typePrefix string
	var fieldUpperCase bool
	switch language {
	case "typescript":
		fieldUpperCase = false
		fieldSeparator = "?: "
		arrayPrefix = ""
		arrayPostfix = "[]"
		//objectOpen = "{\n"
		//objectClose = "}"
		fieldDelimiter = ";"
		stringType = "string"
		numberType = "number"
		boolType = "boolean"
		int32Type = "number"
		int64Type = "number"
		floatType = "number"
		doubleType = "number"
		anyType = "any"
		mapType = "{ [key: string]: %v }"
		typePrefix = ""
	case "go":
		fieldUpperCase = true
		fieldSeparator = " "
		arrayPrefix = "[]"
		arrayPostfix = ""
		//objectOpen = "{"
		//	objectClose = "}"
		fieldDelimiter = ""
		stringType = "string"
		numberType = "int64"
		boolType = "bool"
		int32Type = "int32"
		int64Type = "int64"
		floatType = "float32"
		doubleType = "float64"
		mapType = "map[string]%v"
		anyType = "interface{}"
		typePrefix = "*"
	}

	valueToType := func(v *openapi3.SchemaRef) string {
		switch v.Value.Type {
		case "string":
			return stringType
		case "boolean":
			return boolType
		case "number":
			switch v.Value.Format {
			case "int32":
				return int32Type
			case "int64":
				return int64Type
			case "float":
				return floatType
			case "double":
				return doubleType
			}
		default:
			return "unrecognized: " + v.Value.Type
		}
		return ""
	}

	recurse = func(props map[string]*openapi3.SchemaRef, level int) string {
		ret := ""

		i := 0
		var keys []string
		for k := range props {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := props[k]
			ret += strings.Repeat("  ", level)
			if v.Value.Description != "" {
				for _, commentLine := range strings.Split(v.Value.Description, "\n") {
					ret += "// " + strings.TrimSpace(commentLine) + "\n" + strings.Repeat("  ", level)
				}

			}

			// save k
			fieldName := k

			if fieldUpperCase {
				k = strcase.UpperCamelCase(k)
			}

			var typ string
			// @todo clean up this piece of code by
			// separating out type string marshaling and not
			// repeating code
			switch v.Value.Type {
			case "object":
				typ, found := detectType(k, v.Value.Properties)
				if found {
					ret += k + fieldSeparator + typePrefix + strings.Title(typ) + fieldDelimiter
				} else {
					// type is a dynamic map
					// if additional properties is not present, it's an any type,
					// like the proto struct type
					if v.Value.AdditionalProperties != nil {
						ret += k + fieldSeparator + fmt.Sprintf(mapType, valueToType(v.Value.AdditionalProperties)) + fieldDelimiter
					} else {
						ret += k + fieldSeparator + fmt.Sprintf(mapType, anyType) + fieldDelimiter
					}
				}
			case "array":
				typ, found := detectType(k, v.Value.Items.Value.Properties)
				if found {
					ret += k + fieldSeparator + arrayPrefix + strings.Title(typ) + arrayPostfix + fieldDelimiter
				} else {
					switch v.Value.Items.Value.Type {
					case "string":
						ret += k + fieldSeparator + arrayPrefix + stringType + arrayPostfix + fieldDelimiter
					case "number":
						typ := numberType
						switch v.Value.Format {
						case "int32":
							typ = int32Type
						case "int64":
							typ = int64Type
						case "float":
							typ = floatType
						case "double":
							typ = doubleType
						}
						ret += k + fieldSeparator + arrayPrefix + typ + arrayPostfix + fieldDelimiter
					case "boolean":
						ret += k + fieldSeparator + arrayPrefix + boolType + arrayPostfix + fieldDelimiter
					case "object":
						// type is a dynamic map
						// if additional properties is not present, it's an any type,
						// like the proto struct type
						if v.Value.AdditionalProperties != nil {
							ret += k + fieldSeparator + arrayPrefix + fmt.Sprintf(mapType, valueToType(v.Value.AdditionalProperties)) + arrayPostfix + fieldDelimiter
						} else {
							ret += k + fieldSeparator + arrayPrefix + fmt.Sprintf(mapType, anyType) + arrayPostfix + fieldDelimiter
						}
					}
				}
			case "string":
				ret += k + fieldSeparator + stringType + fieldDelimiter
			case "number":
				typ = numberType
				switch v.Value.Format {
				case "int32":
					typ = int32Type
				case "int64":
					typ = int64Type
				case "float":
					typ = floatType
				case "double":
					typ = doubleType
				}
				ret += k + fieldSeparator + typ + fieldDelimiter
			case "boolean":
				ret += k + fieldSeparator + boolType + fieldDelimiter
			}
			// go specific hack for lowercase json
			if language == "go" {
				ret += " " + "`json:\"" + fieldName
				if typ == int64Type {
					ret += ",string"
				}
				ret += ",omitempty\"`"
			}

			if i < len(props) {
				ret += "\n"
			}
			i++

		}
		return ret
	}
	return recurse(spec.Value.Properties, 1)
}

func schemaToMethods(title string, spec *openapi3.RequestBodyRef) string {
	return ""
}

// CopyFile copies a file from src to dst. If src and dst files exist, and are
// the same, then return success. Otherise, attempt to create a hard link
// between the two files. If that fail, copy the file contents from src to dst.
// from https://stackoverflow.com/questions/21060945/simple-way-to-copy-a-file-in-golang
func CopyFile(src, dst string) (err error) {
	sfi, err := os.Stat(src)
	if err != nil {
		return
	}
	if !sfi.Mode().IsRegular() {
		// cannot copy non-regular files (e.g., directories,
		// symlinks, devices, etc.)
		return fmt.Errorf("CopyFile: non-regular source file %s (%q)", sfi.Name(), sfi.Mode().String())
	}
	dfi, err := os.Stat(dst)
	if err != nil {
		if !os.IsNotExist(err) {
			return
		}
	} else {
		if !(dfi.Mode().IsRegular()) {
			return fmt.Errorf("CopyFile: non-regular destination file %s (%q)", dfi.Name(), dfi.Mode().String())
		}
		if os.SameFile(sfi, dfi) {
			return
		}
	}
	if err = os.Link(src, dst); err == nil {
		return
	}
	err = copyFileContents(src, dst)
	return
}

// copyFileContents copies the contents of the file named src to the file named
// by dst. The file will be created if it does not already exist. If the
// destination file exists, all it's contents will be replaced by the contents
// of the source file.
func copyFileContents(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return
	}
	err = out.Sync()
	return
}
