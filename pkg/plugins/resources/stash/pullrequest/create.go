package pullrequest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/drone/go-scm/scm"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/updatecli/pkg/core/reports"
	"github.com/updatecli/updatecli/pkg/core/result"
	utils "github.com/updatecli/updatecli/pkg/plugins/utils/action"
)

// CreateAction opens a Pull Request on the Bitbucket server
func (s *Stash) CreateAction(report *reports.Action, resetDescription bool) error {
	title := report.Title
	if len(s.spec.Title) > 0 {
		title = s.spec.Title
	}

	// Test that both sourceBranch and targetBranch exists on remote before creating a new one
	ok, err := s.isRemoteBranchesExist()
	if err != nil {
		return err
	}

	/*
		Due to the following scenario, Updatecli always tries to open a Pull request
			* A pull request has been "manually" closed via UI
			* A previous Updatecli run failed during a Pull request creation for example due to network issues


		Therefore we always try to open a pull request, we don't consider being an error if all conditions are not met
		such as missing remote branches.
	*/
	if !ok {
		logrus.Debugln("skipping pull request creation")
		return nil
	}

	pullRequestExists, pullRequestDetails, err := s.isPullRequestExist()
	if err != nil {
		return err
	}

	var responseTitle, responseBody, responseLink string
	if pullRequestExists {

		mergedDescription, err := reports.MergeFromMarkdown(pullRequestDetails.Description, report.ToActionsMarkdownString())
		if err != nil {
			return err
		}

		var body string
		if len(s.spec.Body) > 0 {
			body = s.spec.Body
		} else {
			body, err = utils.GeneratePullRequestBodyMarkdown("", mergedDescription)
			if err != nil {
				return err
			}
		}

		logrus.Debugf("Title:\t%q\nBody:\t%q\nSource:\n%q\nTarget:\t%q\n",
			title,
			body,
			s.SourceBranch,
			s.TargetBranch)

		responseTitle, responseBody, responseLink, err = s.updatePullRequest(pullRequestDetails.Number, title, body)
		if err != nil {
			return err
		}

		logrus.Infof("%s Bitbucket Cloud pull request successfully updated %q", result.SUCCESS, responseLink)
	} else {

		var body string
		if len(s.spec.Body) > 0 {
			body = s.spec.Body
		} else {
			body, err = utils.GeneratePullRequestBodyMarkdown("", report.ToActionsMarkdownString())
			if err != nil {
				logrus.Warningf("something went wrong while generating Bitbucket Pull Request body: %s", err)
			}
		}

		logrus.Debugf("Title:\t%q\nBody:\t%q\nSource:\n%q\nTarget:\t%q\n",
			title,
			body,
			s.SourceBranch,
			s.TargetBranch)

		responseTitle, responseBody, responseLink, err = s.createPullRequest(title, body)
		if err != nil {
			return err
		}

		logrus.Infof("%s Bitbucket Cloud pull request successfully opened %q", result.SUCCESS, responseLink)
	}

	report.Link = responseLink
	report.Description = responseBody
	report.Title = responseTitle

	return nil
}

func (s *Stash) createPullRequest(title, body string) (responseTitle string, responseBody string, link string, err error) {
	opts := scm.PullRequestInput{
		Title:  title,
		Body:   body,
		Source: s.SourceBranch,
		Target: s.TargetBranch,
	}

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	pr, resp, err := s.client.PullRequests.Create(
		ctx,
		strings.Join([]string{
			s.Owner,
			s.Repository,
		}, "/"),
		&opts,
	)

	s.logErrorResponse(resp)

	if err != nil {
		if err.Error() == scm.ErrNotFound.Error() {
			logrus.Infof("Bitbucket Cloud pull request not created, skipping")
			return "", "", "", nil
		}
		return "", "", "", err
	}

	return pr.Title, pr.Body, pr.Link, nil
}

func (s *Stash) updatePullRequest(pullRequestNumber int, title, body string) (responseTitle string, responseBody string, link string, err error) {
	type requestInput struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Source      struct {
			Branch struct {
				Name string `json:"name"`
			} `json:"branch"`
		} `json:"source"`
		Destination struct {
			Branch struct {
				Name string `json:"name"`
			} `json:"branch"`
		} `json:"destination"`
	}

	in := new(requestInput)
	in.Title = title
	in.Description = body
	in.Source.Branch.Name = s.SourceBranch
	in.Destination.Branch.Name = s.TargetBranch

	buf := new(bytes.Buffer)
	err = json.NewEncoder(buf).Encode(in)
	if err != nil {
		return "", "", "", err
	}

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := s.client.Do(ctx, &scm.Request{
		Method: "PUT",
		Path:   fmt.Sprintf("2.0/repositories/%s/%s/pullrequests/%d", s.Owner, s.Repository, pullRequestNumber),
		Header: map[string][]string{
			"Content-Type": {"application/json"},
		},
		Body: buf,
	})

	s.logErrorResponse(resp)

	if err != nil {
		return "", "", "", err
	}

	type requestOutput struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Links       struct {
			HTML struct {
				Href string `json:"href"`
			} `json:"html"`
		} `json:"links"`
	}

	defer resp.Body.Close()

	pr := new(requestOutput)

	err = json.NewDecoder(resp.Body).Decode(pr)
	if err != nil {
		return "", "", "", err
	}

	return pr.Title, pr.Description, pr.Links.HTML.Href, nil
}

func (s *Stash) logErrorResponse(resp *scm.Response) {
	if resp != nil {
		if resp.Status > 400 {
			logrus.Debugf("RC: %d\nBody:\n%s", resp.Status, resp.Body)
		}
	}
}
