package pullrequest

import (
	"github.com/sirupsen/logrus"
	"github.com/updatecli/updatecli/pkg/core/reports"
)

// CheckActionExist verifies if an existing Stash pullrequest is already opened.
func (s *Stash) CheckActionExist(report *reports.Action) error {
	pullRequestExists, pullRequestDetails, err := s.isPullRequestExist()
	if err != nil {
		return err
	}

	if pullRequestExists {
		logrus.Debugf("Stash pull request detected")

		report.Title = pullRequestDetails.Title
		report.Description = pullRequestDetails.Description
		report.Link = pullRequestDetails.Link
		return nil
	}

	return nil
}
