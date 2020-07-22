package flow

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/sakajunquality/flow/gitbot"
	"github.com/sakajunquality/flow/slackbot"

	retry "github.com/avast/retry-go"
)

const (
	retryCommitTries = 3
)

type PullRequests []PullRequest

type PullRequest struct {
	env string
	url string
	err error
}

func (f *Flow) processImage(ctx context.Context, image, version string) error {
	app, err := getApplicationByImage(image)
	if err != nil {
		return err
	}

	prs := f.process(ctx, app, version)
	return f.notifyReleasePR(image, version, prs, app)
}

func (f *Flow) process(ctx context.Context, app *Application, version string) PullRequests {
	var prs PullRequests
	client := gitbot.NewGitHubClient(ctx, f.githubToken)

	for _, manifest := range app.Manifests {
		if !shouldProcess(manifest, version) {
			continue
		}

		release := newRelease(*app, manifest, version)

		for _, filePath := range manifest.Files {
			release.AddChanges(filePath, fmt.Sprintf("%s:.*", app.Image), fmt.Sprintf("%s:%s", app.Image, version))
			if app.RewriteVersion {
				release.AddChanges(filePath, "version: .*", fmt.Sprintf("version: %s", version))
			}

			if app.RewriteNewTag && strings.Contains(filePath, "kustomization.yaml") {
				release.AddChanges(filePath, "newTag: .*", fmt.Sprintf("newTag: %s", version))
			}
		}

		err := retry.Do(
			func() error {
				return release.Commit(ctx, client)
			},
			retry.DelayType(func(n uint, config *retry.Config) time.Duration {
				return time.Duration(3) * time.Second
			}),
			retry.Attempts(retryCommitTries),
		)

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

func newRelease(app Application, manifest Manifest, version string) *gitbot.Release {
	branchName := getBranchName(app, manifest, version)
	message := getCommitMessage(app, manifest, version)

	// Use base a branch configured in app level
	baseBranch := app.ManifestBaseBranch
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

	body := fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", app.SourceOwner, app.SourceName, version)
	if manifest.PRBody != "" {
		body += fmt.Sprintf("\n\n%s", manifest.PRBody)
	}

	var labels []string
	labels = append(labels, app.SourceName)
	labels = append(labels, manifest.Env)
	labels = append(labels, manifest.Labels...)

	return &gitbot.Release{
		Repo: gitbot.Repo{
			SourceOwner:  app.ManifestOwner,
			SourceRepo:   app.ManifestName,
			BaseBranch:   baseBranch,
			CommitBranch: commitBranch,
		},
		Author: gitbot.Author{
			Name:  cfg.GitAuthor.Name,
			Email: cfg.GitAuthor.Email,
		},
		Message: message,
		Body:    body,
		Labels:  labels,
	}
}

func getBranchName(a Application, m Manifest, version string) string {
	branch := "release/"
	branch += m.Env

	repo := a.SourceName
	if m.ShowSourceOwner {
		repo = fmt.Sprintf("%s-%s", a.SourceOwner, repo)
	}

	if m.ShowSourceName {
		branch += "-" + repo
	}

	branch += "-" + version
	return branch
}

func getCommitMessage(a Application, m Manifest, version string) string {
	message := "Release"
	message += " " + m.Env

	repo := a.SourceName
	if m.ShowSourceOwner {
		repo = fmt.Sprintf("%s/%s", a.SourceOwner, repo)
	}

	if m.ShowSourceName {
		message += " " + repo
	}

	message += " " + version
	return message
}

func (f *Flow) notifyReleasePR(image, version string, prs PullRequests, app *Application) error {
	var prURL string

	for _, pr := range prs {
		if pr.err != nil {
			prURL += fmt.Sprintf("`%s`\n```%s```\n", pr.env, pr.err)
			continue
		}

		prURL += fmt.Sprintf("`%s`\n```%s```\n", pr.env, pr.url)
	}

	d := slackbot.MessageDetail{
		IsSuccess:  true,
		IsPrNotify: true,
		AppName:    app.Name,
		Image:      image,
		Version:    version,
		PrURL:      prURL,
	}

	return slackbot.NewSlackMessage(f.slackBotToken, cfg.SlackNotifiyChannel, d).Post()
}

func getApplicationByImage(image string) (*Application, error) {
	for _, app := range cfg.ApplicationList {
		if image == app.Image {
			return &app, nil
		}
	}
	return nil, errors.New("No application found for image " + image)
}
