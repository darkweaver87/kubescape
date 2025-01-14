package resourcehandler

import (
	"errors"
	"fmt"
	nethttp "net/http"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	giturl "github.com/kubescape/go-git-url"
)

// To Check if the given repository is Public(No Authentication needed), send a HTTP GET request to the URL
// If response code is 200, the repository is Public.
func isGitRepoPublic(u string) bool {
	resp, err := nethttp.Get(u) //nolint:gosec
	if err != nil {
		return false
	}

	// if the status code is 200, our get request is successful.
	// It only happens when the repository is public.
	if resp.StatusCode == 200 {
		return true
	}

	return false
}

// Check if the GITHUB_TOKEN is present
func isGitTokenPresent(gitURL giturl.IGitAPI) bool {
	if token := gitURL.GetToken(); token == "" {
		return false
	}
	return true
}

// Get the error message according to the provider
func getProviderError(gitURL giturl.IGitAPI) error {
	switch gitURL.GetProvider(){
	case "github":
		return fmt.Errorf("%w", errors.New("GITHUB_TOKEN is not present"))
	case "gitlab":
		return fmt.Errorf("%w", errors.New("GITLAB_TOKEN is not present"))
	}
	return fmt.Errorf("%w", errors.New("unable to find the host name"))
}

// cloneRepo clones a repository to a local temporary directory and returns the directory
func cloneRepo(gitURL giturl.IGitAPI) (string, error) {

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}

	// Get the URL to clone
	cloneURL := gitURL.GetHttpCloneURL()

	isGitRepoPublic := isGitRepoPublic(cloneURL)

	// Declare the authentication variable required for cloneOptions
	var auth transport.AuthMethod

	if isGitRepoPublic {
		// No authentication needed if repository is public
		auth = nil
	} else {

		// Return Error if the AUTH_TOKEN is not present
		if isGitTokenPresent := isGitTokenPresent(gitURL); !isGitTokenPresent {
			return "", getProviderError(gitURL)
		}
		auth = &http.BasicAuth{
			Username: "anything Except Empty String",
			Password: gitURL.GetToken(),
		}
	}

	// Clone option
	cloneOpts := git.CloneOptions{URL: cloneURL, Auth: auth}
	if gitURL.GetBranchName() != "" {
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(gitURL.GetBranchName())
		cloneOpts.SingleBranch = true
	}

	// Actual clone
	_, err = git.PlainClone(tmpDir, false, &cloneOpts)
	if err != nil {
		return "", fmt.Errorf("failed to clone %s. %w", gitURL.GetRepoName(), err)
	}

	return tmpDir, nil
}
