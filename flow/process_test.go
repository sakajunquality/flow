package flow

import (
	// "regexp"

	"fmt"
	"testing"

	"github.com/dlclark/regexp2"
	"github.com/stretchr/testify/assert"
)

func TestNewRelease(t *testing.T) {
	cfg = &Config{
		GitAuthor: GitAuthor{
			Name:  "test",
			Email: "test@test.test",
		},
	}

	app := Application{
		SourceOwner:        "wonderland",
		SourceName:         "alice",
		ManifestBaseBranch: "master",
	}
	manifest := Manifest{
		Env: "production",
	}
	version := "bar"

	r := newRelease(app, manifest, version)
	assert.Equal(t, "Rollout production alice bar", r.GetMessage())
	assert.Equal(t, "rollout/production-alice-bar", r.GetRepo().CommitBranch)
	assert.Equal(t, "master", r.GetRepo().BaseBranch)
	assert.Equal(t, []string{"alice", "production"}, r.GetLabels())

	manifest.BaseBranch = "production"
	r2 := newRelease(app, manifest, version)
	assert.Equal(t, "Rollout production alice bar", r2.GetMessage())
	assert.Equal(t, "rollout/production-alice-bar", r2.GetRepo().CommitBranch)
	assert.Equal(t, "production", r2.GetRepo().BaseBranch)
	assert.Equal(t, []string{"alice", "production"}, r2.GetLabels())

	manifest.CommitWithoutPR = true
	r3 := newRelease(app, manifest, version)
	assert.Equal(t, "Rollout production alice bar", r3.GetMessage())
	assert.Equal(t, "production", r3.GetRepo().CommitBranch)
	assert.Equal(t, "production", r3.GetRepo().BaseBranch)
	assert.Equal(t, []string{"alice", "production"}, r3.GetLabels())

	app2 := Application{
		SourceOwner: "abc",
		SourceName:  "123",
	}
	manifest2 := Manifest{
		Env:        "dev",
		BaseBranch: "dev",
	}

	r4 := newRelease(app2, manifest2, version)
	assert.Equal(t, "Rollout dev 123 bar", r4.GetMessage())
	assert.Equal(t, "rollout/dev-123-bar", r4.GetRepo().CommitBranch)
	assert.Equal(t, "dev", r4.GetRepo().BaseBranch)

	manifest2.CommitWithoutPR = true
	r5 := newRelease(app2, manifest2, version)
	assert.Equal(t, "Rollout dev 123 bar", r5.GetMessage())
	assert.Equal(t, "dev", r5.GetRepo().CommitBranch)
	assert.Equal(t, "dev", r5.GetRepo().BaseBranch)

	manifest2.Labels = []string{"bob"}
	r6 := newRelease(app, manifest2, version)
	assert.Equal(t, []string{"alice", "dev", "bob"}, r6.GetLabels())

	cfg.DefaultBranch = "main"
	manifest.BaseBranch = ""
	app.ManifestBaseBranch = ""
	r7 := newRelease(app, manifest, version)
	assert.Equal(t, "main", r7.GetRepo().BaseBranch)

	manifest.BaseBranch = "master"
	r8 := newRelease(app, manifest, version)
	assert.Equal(t, "master", r8.GetRepo().BaseBranch)
}

func TestNewReleaseForDefaultOrg(t *testing.T) {
	cfg = &Config{
		DefaultManifestOwner: "foo-inc",
		DefaultManifestName:  "bar-repo",
	}

	app := Application{
		SourceOwner:        "wonderland",
		SourceName:         "alice",
		ManifestBaseBranch: "master",
	}
	manifest := Manifest{
		Env: "production",
	}
	version := "1234"

	r := newRelease(app, manifest, version)

	assert.Equal(t, "foo-inc", r.GetRepo().SourceOwner)
	assert.Equal(t, "bar-repo", r.GetRepo().SourceRepo)

	app.ManifestOwner = "abc-inc"
	app.ManifestName = "xyz-repo"

	r2 := newRelease(app, manifest, version)
	assert.Equal(t, "abc-inc", r2.GetRepo().SourceOwner)
	assert.Equal(t, "xyz-repo", r2.GetRepo().SourceRepo)

	manifest.ManifestOwner = "another-owner"
	manifest.ManifestName = "another-name"

	r3 := newRelease(app, manifest, version)
	assert.Equal(t, "another-owner", r3.GetRepo().SourceOwner)
	assert.Equal(t, "another-name", r3.GetRepo().SourceRepo)
}

func TestShouldProcess(t *testing.T) {
	// ignore empty and latest
	assert.Equal(t, false, shouldProcess(Manifest{}, ""))
	assert.Equal(t, false, shouldProcess(Manifest{}, "latest"))

	// usual tag
	assert.Equal(t, true, shouldProcess(Manifest{}, "foo"))

	// test include prefix
	m1 := Manifest{
		Filters: Filters{
			IncludePrefixes: []string{
				"v",
			},
		},
	}
	assert.Equal(t, true, shouldProcess(m1, "v123"))
	assert.Equal(t, false, shouldProcess(m1, "release-foo"))

	// test exclude prefix
	m2 := Manifest{
		Filters: Filters{
			ExcludePrefixes: []string{
				"v",
			},
		},
	}
	assert.Equal(t, false, shouldProcess(m2, "v123"))
	assert.Equal(t, true, shouldProcess(m2, "release-foo"))

	// mixed
	m3 := Manifest{
		Filters: Filters{
			IncludePrefixes: []string{
				"v",
			},
			ExcludePrefixes: []string{
				"vv",
			},
		},
	}
	assert.Equal(t, true, shouldProcess(m3, "v123"))
	assert.Equal(t, false, shouldProcess(m3, "vv123"))
	assert.Equal(t, false, shouldProcess(m3, "release-foo"))
}

func TestGetBranchName(t *testing.T) {
	app := Application{
		SourceOwner: "foo-inc",
		SourceName:  "bar",
	}
	manifest := Manifest{
		Env: "prod",
	}
	version := "v0.0.0"

	assert.Equal(t, "rollout/prod-bar-v0.0.0", getBranchName(app, manifest, version))
	manifest.ShowSourceOwner = true
	assert.Equal(t, "rollout/prod-foo-inc-bar-v0.0.0", getBranchName(app, manifest, version))
	manifest.HideSourceName = true
	assert.Equal(t, "rollout/prod-v0.0.0", getBranchName(app, manifest, version))
	app.Name = "foo"
	assert.Equal(t, "rollout/prod-foo-v0.0.0", getBranchName(app, manifest, version))
}

func TestGetCommitMessage(t *testing.T) {
	app := Application{
		SourceOwner: "foo-inc",
		SourceName:  "bar",
	}
	manifest := Manifest{
		Env: "prod",
	}
	version := "v0.0.0"

	assert.Equal(t, "Rollout prod bar v0.0.0", getCommitMessage(app, manifest, version))
	manifest.ShowSourceOwner = true
	assert.Equal(t, "Rollout prod foo-inc/bar v0.0.0", getCommitMessage(app, manifest, version))
	manifest.HideSourceName = true
	assert.Equal(t, "Rollout prod v0.0.0", getCommitMessage(app, manifest, version))
}

// test process() itself after refactoring
func TestRegexTemplate(t *testing.T) {
	const (
		oldVersion = "oldoldold"
		newVersion = "newnewnew"
	)

	// test imageRewriteRegexTemplate
	const testImage = "gcr.io/foo/bar"
	image := regexp2.MustCompile(fmt.Sprintf(imageRewriteRegexTemplate, testImage), 0)
	r1, err := image.Replace(fmt.Sprintf("%s:%s", testImage, oldVersion), fmt.Sprintf("%s:%s", testImage, newVersion), 0, -1)
	assert.Nil(t, err)
	assert.Equal(t, r1, fmt.Sprintf("%s:%s", testImage, newVersion))

	// test additionalRewriteKeysRegexTemplate
	const testKey = "hogefuga"
	rewriteKey := regexp2.MustCompile(fmt.Sprintf(additionalRewriteKeysRegexTemplate, testKey), 0)
	r2, err := rewriteKey.Replace(fmt.Sprintf("%s: %s", testKey, oldVersion), fmt.Sprintf("%s: %s", testKey, newVersion), 0, -1)
	assert.Nil(t, err)
	assert.Equal(t, r2, fmt.Sprintf("%s: %s", testKey, newVersion))

	// test additionalRewritePrefixRegexTemplate
	const testPrefix = "-cprof_service_version="
	rewritePrefix := regexp2.MustCompile(fmt.Sprintf(additionalRewritePrefixRegexTemplate, testPrefix), 0)
	r3, err := rewritePrefix.Replace(fmt.Sprintf("%s%s", testPrefix, oldVersion), fmt.Sprintf("%s%s", testPrefix, newVersion), 0, -1)
	assert.Nil(t, err)
	assert.Equal(t, r3, fmt.Sprintf("%s%s", testPrefix, newVersion))
}

func TestVersionREwriteRegex(t *testing.T) {

	testcases := []struct {
		name     string
		change   func(m regexp2.Match) string
		original string
		expected string
	}{
		{
			name:   "no quotes",
			change: func(m regexp2.Match) string { return "version: xyz" },
			original: `
version: abc
version: 123 # do-not-rewrite
test:
  foo:
    bar:
      version: abc
	foo:
      version: aiueo # no-rewrite
`,
			expected: `
version: xyz
version: 123 # do-not-rewrite
test:
  foo:
    bar:
      version: xyz
	foo:
      version: aiueo # no-rewrite
`,
		},
		{
			name:   "with quotes",
			change: func(m regexp2.Match) string { return `version: "xyz"` },

			original: `
version: abc
version: 123 # do-not-rewrite
test:
  foo:
    bar:
      version: abc
	foo:
      version: aiueo # no-rewrite
`,
			expected: `
version: "xyz"
version: 123 # do-not-rewrite
test:
  foo:
    bar:
      version: "xyz"
	foo:
      version: aiueo # no-rewrite
`,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {

			re := regexp2.MustCompile(versionRewriteRegex, 0)
			result, err := re.ReplaceFunc(tc.original, tc.change, 0, -1)

			assert.Nil(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}
