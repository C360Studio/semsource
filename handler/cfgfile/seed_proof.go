package cfgfile

// configParseError preserves the ordinary parser's best-effort policy while
// making a known parse failure ineligible to prove a complete reactivation seed.
func configParseError(base string, content []byte) error {
	switch base {
	case "go.mod":
		_, err := ParseGoMod(content)
		return err
	case "package.json":
		_, err := ParsePackageJSON(content)
		return err
	case "Dockerfile":
		_, err := ParseDockerfile(content)
		return err
	case "pom.xml":
		_, err := ParsePOM(content)
		return err
	case "build.gradle":
		_, err := ParseGradle(content)
		return err
	default:
		return nil
	}
}
