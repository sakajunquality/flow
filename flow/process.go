package flow

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/dlclark/regexp2"
	"github.com/sakajunquality/flow/gitbot"
)

type PullRequests []PullRequest

type PullRequest struct {
	env string
	url string
	err error
}

const (
	// Need to test every regex because failures in regexp2.MustCompile results in panic
	// rewrite version but do not if there is comment "# do-not-rewrite" or "# no-rewrite"
	versionRewriteRegex = "(?!.*(do-not-rewrite|no-rewrite).*)(version: +(?<version>.*))"
	// the followings will be used with fmt.Sprintf and %s will be replaced
	imageRewriteRegexTemplate            = "%s:(?<version>.*)"
	additionalRewriteKeysRegexTemplate   = "%s: +(?<version>.*)"
	additionalRewritePrefixRegexTemplate = "%s(?<version>.*)"
)

func (f *Flow) processImage(ctx context.Context, image, version string) error {
	app, err := getApplicationByImage(image)
	if err != nil {
		return err
	}

	prs := f.process(ctx, app, version)

	for _, pr := range prs {
		log.Printf("Processed PR: %s\n", pr.url)
	}
	return nil
}

func (f *Flow) process(ctx context.Context, app *Application, version string) PullRequests {
	var prs PullRequests
	client := gitbot.NewGitHubClient(ctx, f.githubToken)

	for _, manifest := range app.Manifests {
		if !shouldProcess(manifest, version) {
			continue
		}

		release := newRelease(*app, manifest, version)

		oldVersionSet := map[string]interface{}{}
		for _, filePath := range manifest.Files {
			release.MakeChangeFunc(ctx, client, filePath, fmt.Sprintf(imageRewriteRegexTemplate, app.Image), func(m regexp2.Match) string {
				oldVersionSet[m.GroupByName("version").String()] = nil
				return fmt.Sprintf("%s:%s", app.Image, version)
			})
			release.MakeChangeFunc(ctx, client, filePath, versionRewriteRegex, func(m regexp2.Match) string {
				oldVersionSet[m.GroupByName("version").String()] = nil
				return fmt.Sprintf("version: %s", version)
			})

			for _, key := range app.AdditionalRewriteKeys {
				release.MakeChangeFunc(ctx, client, filePath, fmt.Sprintf(additionalRewriteKeysRegexTemplate, key), func(m regexp2.Match) string {
					oldVersionSet[m.GroupByName("version").String()] = nil
					return fmt.Sprintf("%s: %s", key, version)
				})
			}
			for _, prefix := range app.AdditionalRewritePrefix {
				release.MakeChangeFunc(ctx, client, filePath, fmt.Sprintf(additionalRewritePrefixRegexTemplate, prefix), func(m regexp2.Match) string {
					oldVersionSet[m.GroupByName("version").String()] = nil
					return fmt.Sprintf("%s%s", prefix, version)
				})
			}
		}

		oldVersions := []string{}
		for oldVersion := range oldVersionSet {
			oldVersions = append(oldVersions, oldVersion)
		}
		body := generateBody(*app, manifest, version, oldVersions)
		release.SetBody(body)

		err := release.Commit(ctx, client)
		if err != nil {
			log.Printf("Error Commiting: %s", err)
			continue
		}

		if !manifest.CommitWithoutPR {
			url, err := release.CreatePR(ctx, client)
			if err != nil {
				log.Printf("Error Submitting PR: %s", err)
				continue
			}
			prs = append(prs, PullRequest{
				env: manifest.Env,
				url: *url,
			})
		}
	}
	return prs
}

func shouldProcess(m Manifest, version string) bool {
	if version == "" {
		return false
	}
	// ignore latest tag
	if version == "latest" {
		return false
	}
	for _, prefix := range m.Filters.ExcludePrefixes {
		if strings.HasPrefix(version, prefix) {
			return false
		}
	}

	if len(m.Filters.IncludePrefixes) == 0 {
		return true
	}

	for _, prefix := range m.Filters.IncludePrefixes {
		if strings.HasPrefix(version, prefix) {
			return true
		}
	}

	return false
}

func newRelease(app Application, manifest Manifest, version string) gitbot.Release {
	branchName := getBranchName(app, manifest, version)
	message := getCommitMessage(app, manifest, version)

	// Use base a branch configured in app level
	baseBranch := app.ManifestBaseBranch

	// if baseBranch is not specified in each config use global
	if baseBranch == "" {
		baseBranch = cfg.DefaultBranch
	}

	// if not specified use master
	if baseBranch == "" {
		baseBranch = "master"
	}

	// If a branch is specified in each manifest use it
	if manifest.BaseBranch != "" {
		baseBranch = manifest.BaseBranch
	}

	// Commit in a new branch by default
	commitBranch := branchName
	// If manifest should be commited without a PR, commit to baseBranch
	if manifest.CommitWithoutPR {
		commitBranch = baseBranch
	}

	manifestOwner := cfg.DefaultManifestOwner
	manifestName := cfg.DefaultManifestName

	if app.ManifestOwner != "" {
		manifestOwner = app.ManifestOwner
	}

	if app.ManifestName != "" {
		manifestName = app.ManifestName
	}

	var labels []string
	labels = append(labels, app.SourceName)
	labels = append(labels, manifest.Env)
	labels = append(labels, manifest.Labels...)

	return gitbot.NewRelease(
		gitbot.Repo{
			SourceOwner:  manifestOwner,
			SourceRepo:   manifestName,
			BaseBranch:   baseBranch,
			CommitBranch: commitBranch,
		},
		gitbot.Author{
			Name:  cfg.GitAuthor.Name,
			Email: cfg.GitAuthor.Email,
		},
		message,
		"",
		labels,
	)
}

func getBranchName(a Application, m Manifest, version string) string {
	branch := "rollout/"
	branch += m.Env

	repo := a.SourceName
	if m.ShowSourceOwner {
		repo = fmt.Sprintf("%s-%s", a.SourceOwner, repo)
	}

	if !m.HideSourceName {
		branch += "-" + repo
	}

	branch += "-" + version
	return branch
}

func getCommitMessage(a Application, m Manifest, version string) string {
	message := "Rollout"
	message += " " + m.Env

	repo := a.SourceName
	if m.ShowSourceOwner {
		repo = fmt.Sprintf("%s/%s", a.SourceOwner, repo)
	}

	if !m.HideSourceName {
		message += " " + repo
	}

	message += " " + version
	return message
}

func getApplicationByImage(image string) (*Application, error) {
	for _, app := range cfg.ApplicationList {
		if image == app.Image {
			return &app, nil
		}
	}
	return nil, errors.New("No application found for image " + image)
}

func generateBody(app Application, manifest Manifest, version string, oldVersions []string) string {
	var body string
	if !manifest.HideSourceReleaseDesc {
		body += "## Release\n"
		body += fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s\n", app.SourceOwner, app.SourceName, version)
		body += "\n"

		body += "### Diff from last release\n"
		for _, oldVersion := range oldVersions {
			body += fmt.Sprintf("https://github.com/%s/%s/compare/%s...%s\n", app.SourceOwner, app.SourceName, version, oldVersion)
		}
		body += "\n"
	}

	if manifest.PRBody != "" {
		body += fmt.Sprintf("\n\n%s", manifest.PRBody)
	}

	return body
}
