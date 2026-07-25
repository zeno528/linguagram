package main

import "testing"

func TestDetectDeclaredVersions(t *testing.T) {
	tests := []struct {
		name, path, content, language, want string
	}{
		{"Go module", "go.mod", "module example.com/demo\n\ngo 1.26.0\n", "Go", "1.26.0"},
		{"TypeScript package", "package.json", `{"devDependencies":{"typescript":"~6.0.3"}}`, "TypeScript", "~6.0.3"},
		{"Vue package", "package.json", `{"dependencies":{"vue":"^3.5.13"}}`, "Vue", "^3.5.13"},
		{"Astro package", "package.json", `{"dependencies":{"astro":"^5.12.0"}}`, "Astro", "^5.12.0"},
		{"Svelte package", "package.json", `{"devDependencies":{"svelte":"^5.0.0"}}`, "Svelte", "^5.0.0"},
		{"Node engine", "package.json", `{"engines":{"node":">=20"}}`, "JavaScript", "Node >=20"},
		{"Python project", "pyproject.toml", "[project]\nrequires-python = \">=3.11\"\n", "Python", ">=3.11"},
		{"FastAPI project", "pyproject.toml", "[project]\ndependencies = [\"fastapi[standard]>=0.115.0\"]\n", "$framework:FastAPI", "0.115.0"},
		{"Django requirements", "requirements.txt", "Django==5.1.3\n", "$framework:Django", "5.1.3"},
		{"Maven Java", "pom.xml", "<maven.compiler.release>21</maven.compiler.release>", "Java", "21"},
		{"Spring Boot Maven", "pom.xml", "<parent><artifactId>spring-boot-starter-parent</artifactId><version>3.4.1</version></parent><dependency><groupId>org.springframework.boot</groupId></dependency>", "$framework:Spring Boot", "3.4.1"},
		{"Gradle Java", "build.gradle.kts", "languageVersion.set(JavaLanguageVersion.of(17))", "Java", "17"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			versions := make(map[string]string)
			detectDeclaredVersions(tt.path, []byte(tt.content), versions)
			if got := versions[tt.language]; got != tt.want {
				t.Fatalf("%s version = %q, want %q", tt.language, got, tt.want)
			}
		})
	}
}

func TestBuildStatsAttachesNonNpmFramework(t *testing.T) {
	versions := make(map[string]string)
	detectDeclaredVersions("requirements.txt", []byte("fastapi>=0.115.0\n"), versions)
	stats := buildStats(map[string]int64{"Python": 100}, versions, 100)
	if got := stats[0].Version; got != "FastAPI 0.115.0" {
		t.Fatalf("Python framework version = %q, want FastAPI 0.115.0", got)
	}
}

func TestDetectDeclaredVersionsIncludesNestedManifests(t *testing.T) {
	versions := make(map[string]string)
	detectDeclaredVersions("packages/demo/package.json", []byte(`{"dependencies":{"vue":"^3.5.13"}}`), versions)
	if got := versions["Vue"]; got != "^3.5.13" {
		t.Fatalf("nested Vue version = %q, want ^3.5.13", got)
	}
}

func TestBuildStatsAttachesFrameworkToBestLanguageRow(t *testing.T) {
	versions := make(map[string]string)
	detectDeclaredVersions("package.json", []byte(`{
		"dependencies":{"react":"^19.1.0"},
		"devDependencies":{"typescript":"~6.0.3"}
	}`), versions)
	stats := buildStats(map[string]int64{"TypeScript": 80, "JavaScript": 20}, versions, 100)

	byLanguage := make(map[string]LanguageStat, len(stats))
	for _, stat := range stats {
		byLanguage[stat.Name] = stat
	}
	if got := byLanguage["TypeScript"].Version; got != "~6.0.3 · React ^19.1.0" {
		t.Fatalf("TypeScript version = %q", got)
	}
	if got := byLanguage["JavaScript"].Version; got != "" {
		t.Fatalf("React version should not be duplicated on JavaScript row: %q", got)
	}
}

func TestAddVersionMergesNestedManifestVersionsDeterministically(t *testing.T) {
	versions := make(map[string]string)
	addVersion(versions, "Vue", "^3.5.0")
	addVersion(versions, "Vue", "^2.7.0")
	addVersion(versions, "Vue", "^3.5.0")
	if got := versions["Vue"]; got != "^2.7.0 / ^3.5.0" {
		t.Fatalf("merged Vue versions = %q", got)
	}
}

func TestClassifyFileUsesLinguistLanguageGroup(t *testing.T) {
	got, counted := classifyFile("src/App.tsx", []byte("export const App = () => <main />"))
	if !counted {
		t.Fatal("TSX file should be counted")
	}
	if got != "TypeScript" {
		t.Fatalf("TSX language group = %q, want TypeScript", got)
	}
}
