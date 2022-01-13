package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/go-fluid/fluid"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// todo: generate `entity` directory, each entity gets its own file: {{snake(nameSingular)}}.go
// todo: generate `contract` directory, each contract get its own file: {{snake(name)}}.{{snake(type)}}.go

const (
	BaseApiLatestReleaseInfo           = "https://api.github.com/repos/go-uniform/base-api/releases/latest"
	BaseLogicLatestReleaseInfo         = "https://api.github.com/repos/go-uniform/base-logic/releases/latest"
	BasePortalIonicLatestReleaseInfo   = "https://api.github.com/repos/go-fluid/base-portal-ionic/releases/latest"
	BasePortalVuetifyLatestReleaseInfo = "https://api.github.com/repos/go-fluid/base-portal-vuetify/releases/latest"
)

var buildProject = func(project fluid.Project, directory string) {
	if errs := project.Validate(); errs != nil {
		panic(errs.Error())
	}

	if err := os.MkdirAll(directory, os.ModePerm); err != nil {
		panic(err)
	}

	projectSlug := kebabCase(project.Name)
	temporaryDirectory, err := ioutil.TempDir("", "*")

	if err != nil {
		panic(err)
	}

	projectDirectory := filepath.Join(temporaryDirectory, projectSlug)

	if err := os.MkdirAll(projectDirectory, os.ModePerm); err != nil {
		panic(err)
	}

	buildApi(project, projectDirectory)
	buildLogic(project, projectDirectory)

	for _, portal := range project.Portals {

		switch portal.Type {
		case fluid.PortalTypeIonic:
			buildPortalIonic(portal, projectDirectory)
		case fluid.PortalTypeVuetify:
			buildPortalVuetify(portal, projectDirectory)
		default:
			panic(fmt.Sprintf("portal type '%s' not supported", portal.Type))
		}

	}

	projectFile := filepath.Join(directory, fmt.Sprintf("%s-%s.tar.gz", projectSlug, project.Version))
	cmd := exec.Command("tar", "-czvf", projectFile, "-C", temporaryDirectory, strings.TrimPrefix(projectDirectory, temporaryDirectory+"/"))
	fmt.Println(cmd.String())
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}

var buildPortalIonic = func(portal fluid.Portal, temporaryDirectory string) {

	if portal.Type != fluid.PortalTypeIonic {
		panic(fmt.Sprintf("invalid portal type '%s' detected", portal.Type))
	}

	templateDirectory := filepath.Join(getCacheDirectory(), "portal-ionic/latest")
	portalSlug := kebabCase(portal.Name)
	targetDirectory := filepath.Join(temporaryDirectory, portalSlug)
	copyDirectory(templateDirectory, targetDirectory)

}

var buildPortalVuetify = func(portal fluid.Portal, temporaryDirectory string) {

	if portal.Type != fluid.PortalTypeVuetify {
		panic(fmt.Sprintf("invalid portal type '%s' detected", portal.Type))
	}

	templateDirectory := filepath.Join(getCacheDirectory(), "portal-vuetify/latest")
	portalSlug := kebabCase(portal.Name)
	targetDirectory := filepath.Join(temporaryDirectory, portalSlug)
	copyDirectory(templateDirectory, targetDirectory)

}

var buildApi = func(project fluid.Project, temporaryDirectory string) {

	templateDirectory := filepath.Join(getCacheDirectory(), "api/latest")
	targetDirectory := filepath.Join(temporaryDirectory, "api")
	copyDirectory(templateDirectory, targetDirectory)
	contractsDirectory := filepath.Join(targetDirectory, "service", "contracts")

	for _, contract := range project.Contracts {

		buildContractFile(contract, contractsDirectory)

	}

	cmd := exec.Command("gofmt", "-s", "-w", contractsDirectory)
	fmt.Println(cmd.String())
	if err := cmd.Run(); err != nil {
		panic(err)
	}

}

var buildLogic = func(project fluid.Project, temporaryDirectory string) {

	templateDirectory := filepath.Join(getCacheDirectory(), "logic", "latest")
	targetDirectory := filepath.Join(temporaryDirectory, "logic")
	copyDirectory(templateDirectory, targetDirectory)
	entitiesDirectory := filepath.Join(targetDirectory, "service", "entities")

	for _, entity := range project.Entities {

		buildEntityFile(entity, entitiesDirectory)

	}

	cmd := exec.Command("gofmt", "-s", "-w", entitiesDirectory)
	fmt.Println(cmd.String())
	if err := cmd.Run(); err != nil {
		panic(err)
	}

}

const entityFileTemplate = `package entities

const (
	Collection{{ .NamePlural | PascalCase }} = "{{ .NamePlural | CamelCase }}"
)

type {{ .NameSingular | PascalCase }} struct {

{{range .Fields}}    {{ .Name | PascalCase }} {{ .Type }} ` + "`" + `bson:"{{ .Name | FieldCase }}"` + "`" + `
{{end}}
}
`

var buildEntityFile = func(entity fluid.Entity, directory string) {

	entityFileName := fmt.Sprintf("%s.go", snakeCase(entity.NameSingular))
	entityFilePath := filepath.Join(strings.TrimSuffix(directory, "entities"), "entities", entityFileName)
	entityFile, err := os.Create(entityFilePath)

	if err != nil {
		panic(err)
	}

	defer func() { _ = entityFile.Close() }()

	/* todo: add hidden fields
	- createdAt
	- modifiedAt
	- deletedAt

	- lockedAt (if has password field)
	- loginAt (if has password field)
	- loginAttempts (if has password field)
	*/
	// todo: handle link fields
	// todo: handle attribute fields

	tmpl := template.Must(
		template.New(entity.NameSingular).Funcs(
			template.FuncMap{
				"FieldCase": func(value string) string {
					if strings.ToLower(value) == "id" {
						return "_id"
					}
					return camelCase(value)
				},
				"SnakeCase":  snakeCase,
				"CamelCase":  camelCase,
				"KebabCase":  kebabCase,
				"TitleCase":  titleCase,
				"PascalCase": pascalCase,
			},
		).Parse(
			entityFileTemplate,
		),
	)

	if err := tmpl.Execute(
		entityFile,
		entity,
	); err != nil {
		panic(err)
	}

}

const contractFileTemplate = `package contracts

type {{ .Name | PascalCase }}{{ .Type | PascalCase }} struct {

{{range .Fields}}    {{ .Name | PascalCase }} {{ .Type }} ` + "`" + `bson:"{{ .Name | FieldCase }}"` + "`" + `
{{end}}
}
`

var buildContractFile = func(contract fluid.Contract, directory string) {

	contractFileName := fmt.Sprintf("%s.go", snakeCase(fmt.Sprintf("%s %s", contract.Name, strings.ToTitle(contract.Type))))
	contractFilePath := filepath.Join(strings.TrimSuffix(directory, "contracts"), "contracts", contractFileName)
	contractFile, err := os.Create(contractFilePath)

	if err != nil {
		panic(err)
	}

	defer func() { _ = contractFile.Close() }()

	/* todo: add hidden fields
	- createdAt
	- modifiedAt
	- deletedAt

	- lockedAt (if has password field)
	- loginAt (if has password field)
	- loginAttempts (if has password field)
	*/
	// todo: handle link fields
	// todo: handle attribute fields

	tmpl := template.Must(
		template.New(contract.Name).Funcs(
			template.FuncMap{
				"FieldCase": func(value string) string {
					if strings.ToLower(value) == "id" {
						return "_id"
					}
					return camelCase(value)
				},
				"SnakeCase":  snakeCase,
				"CamelCase":  camelCase,
				"KebabCase":  kebabCase,
				"TitleCase":  titleCase,
				"PascalCase": pascalCase,
			},
		).Parse(
			contractFileTemplate,
		),
	)

	if err := tmpl.Execute(
		contractFile,
		contract,
	); err != nil {
		panic(err)
	}

}

func main() {
	homeDirectory, err := os.UserHomeDir()

	if err != nil {
		panic(err)
	}

	downloadDirectory := filepath.Join(homeDirectory, "Downloads")

	updateCaches()
	buildProject(fluidProjectScheme, downloadDirectory)
}

/* Routines */

type BaseTemplateRepository struct {
	LatestReleaseInfo string
	CacheDirectory    string
}

var extractTarball = func(gzipStream io.Reader, directory string) {
	if err := os.MkdirAll(directory, os.ModePerm); err != nil {
		panic(err)
	}

	file, err := ioutil.TempFile("", "*.tag.gz")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.Remove(file.Name()) }()

	if _, err := io.Copy(file, gzipStream); err != nil {
		panic(err)
	}

	cmd := exec.Command("tar", "-xf", file.Name(), "-C", directory, "--strip-components=1")
	fmt.Println(cmd.String())
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}

var copyDirectory = func(sourceDirectory, targetDirectory string) {

	cmd := exec.Command("cp", "-RL", sourceDirectory, targetDirectory)
	fmt.Println(cmd.String())
	if err := cmd.Run(); err != nil {
		panic(err)
	}

}

var doRequest = func(client *http.Client, request *http.Request) ([]byte, int, error) {
	var body []byte = nil
	var code int = -1

	response, err := client.Do(request)
	if err != nil {
		return nil, -1, err
	}
	code = response.StatusCode

	if response.Body != nil {
		defer func() { _ = response.Body.Close() }()

		data, readErr := ioutil.ReadAll(response.Body)
		if readErr != nil {
			return nil, -1, err
		}

		body = data
	}

	return body, code, nil
}

var doStreamRequest = func(client *http.Client, request *http.Request) (io.ReadCloser, int, error) {
	var code int = -1

	response, err := client.Do(request)
	if err != nil {
		return nil, -1, err
	}
	code = response.StatusCode

	if response.Body != nil {
		return response.Body, code, err
	}

	return nil, code, nil
}

var getJson = func(uri string, model interface{}) {
	client := http.Client{
		Timeout: time.Second * 2,
	}

	request, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		panic(err)
	}

	body, code, err := doRequest(&client, request)
	if err != nil {
		panic(err)
	}

	if code != 200 {
		panic(fmt.Sprintf("error code '%d' received", code))
	}

	if err := json.Unmarshal(body, &model); err != nil {
		panic(err)
	}
}

var getDownloadStream = func(uri string) io.ReadCloser {
	client := http.Client{
		Timeout: time.Minute * 2,
	}

	request, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		panic(err)
	}

	stream, code, err := doStreamRequest(&client, request)
	if err != nil {
		panic(err)
	}

	if code != 200 {
		panic(fmt.Sprintf("error code '%d' received", code))
	}

	if stream == nil {
		panic("error not stream data received")
	}

	return stream
}

var getCacheDirectory = func() string {
	homeDirectory, err := os.UserHomeDir()

	if err != nil {
		panic(err)
	}

	cacheDirectory := filepath.Join(homeDirectory, ".cache/fluid")
	return cacheDirectory
}

var updateCaches = func() {
	cacheDirectory := getCacheDirectory()

	if err := os.MkdirAll(cacheDirectory, os.ModePerm); err != nil {
		panic(err)
	}

	templateRepositories := []BaseTemplateRepository{
		{
			LatestReleaseInfo: BaseApiLatestReleaseInfo,
			CacheDirectory:    filepath.Join(cacheDirectory, "api"),
		},
		{
			LatestReleaseInfo: BaseLogicLatestReleaseInfo,
			CacheDirectory:    filepath.Join(cacheDirectory, "logic"),
		},
		{
			LatestReleaseInfo: BasePortalIonicLatestReleaseInfo,
			CacheDirectory:    filepath.Join(cacheDirectory, "portal-ionic"),
		},
		{
			LatestReleaseInfo: BasePortalVuetifyLatestReleaseInfo,
			CacheDirectory:    filepath.Join(cacheDirectory, "portal-vuetify"),
		},
	}

	for _, templateRepository := range templateRepositories {

		var releaseInfo struct {
			TagName    string `json:"tag_name"`
			TarballUrl string `json:"tarball_url"`
		}

		ok := false
		func() {
			defer func() {
				_ = recover()
			}()
			getJson(templateRepository.LatestReleaseInfo, &releaseInfo)
			ok = true
		}()

		if !ok {
			continue
		}

		releaseInfo.TagName = strings.TrimSpace(releaseInfo.TagName)
		releaseInfo.TarballUrl = strings.TrimSpace(releaseInfo.TarballUrl)

		if releaseInfo.TagName == "" {
			panic("release info tag name may not be empty")
		}

		if releaseInfo.TarballUrl == "" {
			panic("release info tarball url may not be empty")
		}

		latestReleaseCacheDirectory := filepath.Join(templateRepository.CacheDirectory, releaseInfo.TagName)

		if _, err := os.Stat(latestReleaseCacheDirectory); os.IsNotExist(err) {
			func() {
				stream := getDownloadStream(releaseInfo.TarballUrl)
				defer func() { _ = stream.Close() }()
				extractTarball(stream, latestReleaseCacheDirectory)
			}()
		}

		symLinkDirectory := filepath.Join(templateRepository.CacheDirectory, "latest")
		_ = os.Remove(symLinkDirectory)
		if err := os.Symlink(latestReleaseCacheDirectory, symLinkDirectory); err != nil {
			panic(err)
		}
	}
}

/* Helpers */

func isUpperCase(r rune) bool {
	if strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZ", r) {
		return true
	}
	return false
}

func containsAnyUpperCase(text string) bool {
	l := len(text)

	if l <= 0 {
		return false
	}

	i := 0
	for i < l {
		if isUpperCase(rune(text[i])) {
			return true
		}
		i++
	}

	return false
}

func keep(text string, keepset string) string {
	l := len(text)
	if l <= 0 || len(keepset) <= 0 {
		return text
	}

	keep := bytes.NewBufferString("")
	i := 0
	for i < l {
		r := rune(text[i])
		if strings.ContainsRune(keepset, r) {
			if _, err := keep.WriteRune(r); err != nil {
				panic(err)
			}
		}
		i++
	}

	return keep.String()
}

func caseSensitiveToKebab(caseSensitive string) string {
	l := len(caseSensitive)
	if l <= 1 {
		return strings.ToLower(caseSensitive)
	}
	if !containsAnyUpperCase(caseSensitive[1:]) {
		return strings.ToLower(caseSensitive)
	}

	kebab := bytes.NewBufferString("")
	kebab.WriteRune(rune(caseSensitive[0]))
	i := 1
	for i < l {
		r := rune(caseSensitive[i])
		if isUpperCase(r) {
			kebab.WriteString("-")
		}
		kebab.WriteRune(r)
		i++
	}

	return strings.ToLower(kebab.String())
}

func kebabCase(anyCase string) string {
	l := len(anyCase)

	if l <= 0 {
		return anyCase
	}

	// strip special characters (expect '-' and '_')
	anyCase = keep(anyCase, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_ ")

	if strings.ContainsRune(anyCase, ' ') {
		// process title case
		anyCase = strings.Replace(anyCase, " ", "-", -1)
	} else if containsAnyUpperCase(anyCase) {
		// process pascal & camel case
		anyCase = caseSensitiveToKebab(anyCase)
	} else if strings.ContainsRune(anyCase, '_') {
		// process snake case
		anyCase = strings.Replace(anyCase, "_", "-", -1)
	}

	return strings.Replace(strings.Trim(keep(strings.ToLower(anyCase), "abcdefghijklmnopqrstuvwxyz0123456789-"), "-"), "--", "-", -1)
}

func camelCase(kebab string) (camelCase string) {
	kebab = kebabCase(kebab)

	isToUpper := false
	for _, runeValue := range kebab {
		if isToUpper {
			camelCase += strings.ToUpper(string(runeValue))
			isToUpper = false
		} else {
			if runeValue == '-' {
				isToUpper = true
			} else {
				camelCase += string(runeValue)
			}
		}
	}
	return
}

func pascalCase(kebab string) (pascalCase string) {
	kebab = kebabCase(kebab)

	isToUpper := false
	for i, runeValue := range kebab {
		if isToUpper || i == 0 {
			pascalCase += strings.ToUpper(string(runeValue))
			isToUpper = false
		} else {
			if runeValue == '-' {
				isToUpper = true
			} else {
				pascalCase += string(runeValue)
			}
		}
	}
	return
}

func snakeCase(kebab string) string {
	return strings.Replace(strings.ToLower(kebabCase(kebab)), "-", "_", -1)
}

func titleCase(kebab string) (titleCase string) {
	kebab = kebabCase(kebab)

	isToUpper := false
	for i, runeValue := range kebab {
		if isToUpper || i == 0 {
			titleCase += strings.ToUpper(string(runeValue))
			isToUpper = false
		} else {
			if runeValue == '-' {
				isToUpper = true
				titleCase += " "
			} else {
				titleCase += string(runeValue)
			}
		}
	}
	return
}

/* Project */

var fluidProjectScheme = fluid.Project{
	Name:    "Fluid",
	Version: "v2.0.alpha",

	Portals: []fluid.Portal{
		{
			Name:              "Administration",
			Type:              fluid.PortalTypeVuetify,
			AccountEntityKeys: []string{"administrator"},
			LightTheme: fluid.Theme{
				Primary: fluid.MaterialColor{
					Color: "#fff",
				},
				Secondary: fluid.MaterialColor{
					Color: "#fff",
				},
				Tertiary: fluid.MaterialColor{
					Color: "#fff",
				},
				Warning: fluid.MaterialColor{
					Color: "#fff",
				},
				Error: fluid.MaterialColor{
					Color: "#fff",
				},
				Success: fluid.MaterialColor{
					Color: "#fff",
				},
				Medium: fluid.MaterialColor{
					Color: "#fff",
				},
				Light: fluid.MaterialColor{
					Color: "#fff",
				},
				Dark: fluid.MaterialColor{
					Color: "#fff",
				},
			},
			DarkTheme: fluid.Theme{
				Primary: fluid.MaterialColor{
					Color: "#fff",
				},
				Secondary: fluid.MaterialColor{
					Color: "#fff",
				},
				Tertiary: fluid.MaterialColor{
					Color: "#fff",
				},
				Warning: fluid.MaterialColor{
					Color: "#fff",
				},
				Error: fluid.MaterialColor{
					Color: "#fff",
				},
				Success: fluid.MaterialColor{
					Color: "#fff",
				},
				Medium: fluid.MaterialColor{
					Color: "#fff",
				},
				Light: fluid.MaterialColor{
					Color: "#fff",
				},
				Dark: fluid.MaterialColor{
					Color: "#fff",
				},
			},
		},
	},
	Entities: []fluid.Entity{
		{
			NameSingular: "Administrator",
			NamePlural:   "Administrators",
			Fields: []fluid.EntityField{
				{
					Group:       "Personal Details",
					Name:        "First Name",
					Description: "A personal name given to someone at birth or baptism and used before a family name.",
					Type:        fluid.EntityFieldTypeString,
				},
				{
					Group:       "Personal Details",
					Name:        "Last Name",
					Description: "A hereditary name common to all members of a family, as distinct from a forename or given name.",
					Type:        fluid.EntityFieldTypeString,
				},
				{
					Group:       "Personal Details",
					Name:        "Mobile",
					Description: "Identifies a mobile phone to which messages are delivered.",
					Type:        fluid.EntityFieldTypeString,
				},
				{
					Group:       "Personal Details",
					Name:        "Email",
					Description: "Identifies an email box to which messages are delivered.",
					Type:        fluid.EntityFieldTypeString,
				},
				{
					Group:       "Personal Details",
					Name:        "Password",
					Description: "A secret word or phrase that must be used to gain admission to a place.",
					Type:        fluid.EntityFieldTypePassword,
				},
			},
		},
		{
			NameSingular: "Project",
			NamePlural:   "Projects",
			Fields: []fluid.EntityField{
				{
					Name:        "Name",
					Description: "A name is a term used for identification by an external observer.",
					Type:        fluid.EntityFieldTypeString,
				},
			},
			Actions: []fluid.EntityAction{
				{
					Name:                       "Build",
					Description:                "Generate code for api, logic and portals based on project's structure",
					Type:                       fluid.EntityActionTypeList,
					Method:                     fluid.EntityActionMethodGet,
					EnableFileDownloadResponse: true,
				},
				{
					Name:                       "Scheme",
					Description:                "Generate fluid scheme json object blueprint",
					Type:                       fluid.EntityActionTypeList,
					Method:                     fluid.EntityActionMethodGet,
					EnableFileDownloadResponse: true,
				},
			},
		},
	},
	Contracts: []fluid.Contract{
		{
			Key:  "build-request",
			Name: "Build",
			Type: fluid.ContractTypeParameters,
			Fields: []fluid.ContractField{
				{
					Name:        "Full",
					Description: "A flag indicating if the build should include all once off resource as well. This is normally only done for the first build.",
					Type:        fluid.ContractFieldTypeBoolean,
				},
			},
		},
	},
}
