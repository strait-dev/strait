package ci

import "os"

// DetectProvider inspects the directory for CI provider configuration files.
func DetectProvider(dir string) string {
	checks := []struct {
		path     string
		isDir    bool
		provider string
	}{
		{path: dir + "/.github", isDir: true, provider: "github"},
		{path: dir + "/.gitlab-ci.yml", isDir: false, provider: "gitlab"},
		{path: dir + "/.circleci", isDir: true, provider: "circleci"},
		{path: dir + "/bitbucket-pipelines.yml", isDir: false, provider: "bitbucket"},
		{path: dir + "/Jenkinsfile", isDir: false, provider: "jenkins"},
	}

	for _, c := range checks {
		info, err := os.Stat(c.path)
		if err != nil {
			continue
		}
		if c.isDir && info.IsDir() {
			return c.provider
		}
		if !c.isDir && !info.IsDir() {
			return c.provider
		}
	}

	return "generic"
}
