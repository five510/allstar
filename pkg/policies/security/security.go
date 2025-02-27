// Copyright 2021 Allstar Authors

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

//     http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package security implements the SECURITY.md security policy.
package security

import (
	"context"
	"fmt"
	"path"

	"github.com/ossf/allstar/pkg/config"
	"github.com/ossf/allstar/pkg/config/operator"
	"github.com/ossf/allstar/pkg/policydef"

	"github.com/google/go-github/v39/github"
	"github.com/rs/zerolog/log"
	"github.com/shurcooL/githubv4"
)

const configFile = "security.yaml"
const polName = "SECURITY.md"

const notifyText = `A SECURITY.md file can give users information about what constitutes a vulnerability and how to report one securely so that information about a bug is not publicly visible. Examples of secure reporting methods include using an issue tracker with private issue support, or encrypted email with a published key.

To fix this, add a SECURITY.md file that explains how to handle vulnerabilities found in your repository. Go to https://github.com/%v/%v/security/policy to enable.

For more information, see https://docs.github.com/en/code-security/getting-started/adding-a-security-policy-to-your-repository.`

// OrgConfig is the org-level config definition for Branch Protection.
type OrgConfig struct {
	// OptConfig is the standard org-level opt in/out config, RepoOverride applies to all
	// BP config.
	OptConfig config.OrgOptConfig `yaml:"optConfig"`

	// Action defines which action to take, default log, other: issue...
	Action string `yaml:"action"`

	//TODO add default contents for "fix" action
}

// RepoConfig is the repo-level config for Branch Protection
type RepoConfig struct {
	// OptConfig is the standard repo-level opt in/out config.
	OptConfig config.RepoOptConfig `yaml:"optConfig"`

	// Action overrides the same setting in org-level, only if present.
	Action *string `yaml:"action"`
}

type mergedConfig struct {
	Action string
}

type details struct {
	Enabled bool
	URL     string
}

var configFetchConfig func(context.Context, *github.Client, string, string, string, interface{}) error

func init() {
	configFetchConfig = config.FetchConfig
}

type v4client interface {
	Query(context.Context, interface{}, map[string]interface{}) error
}

// Security is the SECURITY.md policy object, implements policydef.Policy.
type Security bool

// NewSecurity returns a new SECURITY.md policy.
func NewSecurity() policydef.Policy {
	var s Security
	return s
}

// Name returns the name of this policy, implementing policydef.Policy.Name()
func (s Security) Name() string {
	return polName
}

// Check performs the polcy check for SECURITY.md policy based on the
// configuration stored in the org/repo, implementing policydef.Policy.Check()
func (s Security) Check(ctx context.Context, c *github.Client, owner,
	repo string) (*policydef.Result, error) {
	v4c := githubv4.NewClient(c.Client())
	return check(ctx, c, v4c, owner, repo)
}

func check(ctx context.Context, c *github.Client, v4c v4client, owner,
	repo string) (*policydef.Result, error) {
	oc, rc := getConfig(ctx, c, owner, repo)
	enabled := config.IsEnabled(oc.OptConfig, rc.OptConfig, repo)
	log.Info().
		Str("org", owner).
		Str("repo", repo).
		Str("area", polName).
		Bool("enabled", enabled).
		Msg("Check repo enabled")

	var q struct {
		Repository struct {
			SecurityPolicyUrl       string
			IsSecurityPolicyEnabled bool
		} `graphql:"repository(owner: $owner, name: $name)"`
	}
	variables := map[string]interface{}{
		"owner": githubv4.String(owner),
		"name":  githubv4.String(repo),
	}
	if err := v4c.Query(ctx, &q, variables); err != nil {
		return nil, err
	}
	if !q.Repository.IsSecurityPolicyEnabled {
		return &policydef.Result{
			Enabled:    enabled,
			Pass:       false,
			NotifyText: "Security policy not enabled.\n" + fmt.Sprintf(notifyText, owner, repo),
			Details: details{
				Enabled: false,
				URL:     q.Repository.SecurityPolicyUrl,
			},
		}, nil
	}
	return &policydef.Result{
		Enabled:    enabled,
		Pass:       true,
		NotifyText: "",
		Details: details{
			Enabled: true,
			URL:     q.Repository.SecurityPolicyUrl,
		},
	}, nil
}

// Fix implementing policydef.Policy.Fix(). Currently not supported. Plan
// to support this TODO.
func (s Security) Fix(ctx context.Context, c *github.Client, owner, repo string) error {
	log.Warn().
		Str("org", owner).
		Str("repo", repo).
		Str("area", polName).
		Msg("Action fix is configured, but not implemented.")
	return nil
}

// GetAction returns the configured action from SECURITY.md policy's
// configuration stored in the org-level repo, default log. Implementing
// policydef.Policy.GetAction()
func (s Security) GetAction(ctx context.Context, c *github.Client, owner, repo string) string {
	oc, rc := getConfig(ctx, c, owner, repo)
	mc := mergeConfig(oc, rc, repo)
	return mc.Action
}

func getConfig(ctx context.Context, c *github.Client, owner, repo string) (*OrgConfig, *RepoConfig) {
	oc := &OrgConfig{ // Fill out non-zero defaults
		Action: "log",
	}
	if err := configFetchConfig(ctx, c, owner, operator.OrgConfigRepo, configFile, oc); err != nil {
		log.Error().
			Str("org", owner).
			Str("repo", operator.OrgConfigRepo).
			Str("area", polName).
			Str("file", configFile).
			Err(err).
			Msg("Unexpected config error, using defaults.")
	}
	rc := &RepoConfig{}
	if err := configFetchConfig(ctx, c, owner, repo, path.Join(operator.RepoConfigDir, configFile), rc); err != nil {
		log.Error().
			Str("org", owner).
			Str("repo", repo).
			Str("area", polName).
			Str("file", path.Join(operator.RepoConfigDir, configFile)).
			Err(err).
			Msg("Unexpected config error, using defaults.")
	}
	return oc, rc
}

func mergeConfig(oc *OrgConfig, rc *RepoConfig, repo string) *mergedConfig {
	mc := &mergedConfig{
		Action: oc.Action,
	}

	if !oc.OptConfig.DisableRepoOverride {
		if rc.Action != nil {
			mc.Action = *rc.Action
		}
	}
	return mc
}
