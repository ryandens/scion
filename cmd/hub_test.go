// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"testing"

	"github.com/ptone/scion-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestGetAuthInfo_NoAuth(t *testing.T) {
	settings := &config.Settings{}
	info := getAuthInfo(settings, "https://hub.example.com")
	assert.Equal(t, "none", info.MethodType)
	assert.Equal(t, "none", info.Method)
}

func TestGetAuthInfo_DeprecatedTokenIgnored(t *testing.T) {
	// hub.token is deprecated and should no longer be used for auth
	settings := &config.Settings{
		Hub: &config.HubClientConfig{
			Token: "test-token",
		},
	}
	info := getAuthInfo(settings, "https://hub.example.com")
	// Should NOT return bearer — token is deprecated
	assert.NotEqual(t, "bearer", info.MethodType)
}

func TestGetAuthInfo_DeprecatedAPIKeyIgnored(t *testing.T) {
	// hub.apiKey is deprecated and should no longer be used for auth
	settings := &config.Settings{
		Hub: &config.HubClientConfig{
			APIKey: "test-api-key",
		},
	}
	info := getAuthInfo(settings, "https://hub.example.com")
	// Should NOT return apikey — apiKey is deprecated
	assert.NotEqual(t, "apikey", info.MethodType)
}

func TestGetAuthInfo_EnvTokenTakesPriority(t *testing.T) {
	// SCION_HUB_TOKEN env var should work for bearer auth
	settings := &config.Settings{}
	t.Setenv("SCION_HUB_TOKEN", "env-token")
	info := getAuthInfo(settings, "https://hub.example.com")
	assert.Equal(t, "bearer", info.MethodType)
	assert.Equal(t, "SCION_HUB_TOKEN env", info.Source)
}

func TestGetAuthInfo_NilHub(t *testing.T) {
	settings := &config.Settings{
		Hub: nil,
	}
	info := getAuthInfo(settings, "")
	assert.Equal(t, "none", info.MethodType)
}

func TestParseDefaultBranch_ParsesSymref(t *testing.T) {
	// Real output from `git ls-remote --symref <url> HEAD`
	output := "ref: refs/heads/main\tHEAD\n5f3c6e72abc123def456 HEAD\n"
	result := parseDefaultBranch(output)
	assert.Equal(t, "main", result)
}

func TestParseDefaultBranch_NonMainBranch(t *testing.T) {
	output := "ref: refs/heads/develop\tHEAD\nabc123 HEAD\n"
	result := parseDefaultBranch(output)
	assert.Equal(t, "develop", result)
}

func TestParseDefaultBranch_NoMatch(t *testing.T) {
	// Output that doesn't contain the expected symref line
	output := "abc123def456 HEAD\n"
	result := parseDefaultBranch(output)
	assert.Equal(t, "", result)
}

func TestParseDefaultBranch_EmptyOutput(t *testing.T) {
	result := parseDefaultBranch("")
	assert.Equal(t, "", result)
}
