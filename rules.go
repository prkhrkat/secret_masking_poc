package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"time"
)

// Reusable regex patterns
const (
	quote       = `["']?`
	connect     = `\s*(:|=>|=)?\s*`
	startSecret = `(^|\s+)`
	endSecret   = `[.,]?(\s+|$)`

	aws = `aws_?`
)

// create rule struct
type Rule struct {
	ID              string
	Severity        string
	Title           string
	Regex           *regexp.Regexp
	SecretGroupName string
	Keywords        []string
}

var BuiltinRules = []Rule{
	{
		ID:              "aws-access-key-id",
		Severity:        "CRITICAL",
		Title:           "AWS Access Key ID",
		Regex:           regexp.MustCompile(fmt.Sprintf(`%s(?P<secret>(A3T[A-Z0-9]|AKIA|AGPA|AIDA|AROA|AIPA|ANPA|ANVA|ASIA)[A-Z0-9]{16})%s%s`, quote, quote, endSecret)),
		SecretGroupName: "secret",
		Keywords:        []string{"AKIA", "AGPA", "AIDA", "AROA", "AIPA", "ANPA", "ANVA", "ASIA"},
	},
	{
		ID:              "aws-secret-access-key",
		Severity:        "CRITICAL",
		Title:           "AWS Secret Access Key",
		Regex:           regexp.MustCompile(fmt.Sprintf(`(?i)%s%s%s(sec(ret)?)?_?(access)?_?key%s%s%s(?P<secret>[A-Za-z0-9\/\+=]{40})%s%s`, startSecret, quote, aws, quote, connect, quote, quote, endSecret)),
		SecretGroupName: "secret",
		Keywords:        []string{"key"},
	},
	{
		ID:       "github-pat",
		Title:    "GitHub Personal Access Token",
		Severity: "CRITICAL",
		Regex:    regexp.MustCompile(`ghp_[0-9a-zA-Z]{36}`),
		Keywords: []string{"ghp_"},
	},
	{
		ID:       "github-oauth",
		Title:    "GitHub OAuth Access Token",
		Severity: "CRITICAL",
		Regex:    regexp.MustCompile(`gho_[0-9a-zA-Z]{36}`),
		Keywords: []string{"gho_"},
	},
	{
		ID:       "github-app-token",
		Title:    "GitHub App Token",
		Severity: "CRITICAL",
		Regex:    regexp.MustCompile(`(ghu|ghs)_[0-9a-zA-Z]{36}`),
		Keywords: []string{"ghu_", "ghs_"},
	},
	{
		ID:       "github-refresh-token",
		Title:    "GitHub Refresh Token",
		Severity: "CRITICAL",
		Regex:    regexp.MustCompile(`ghr_[0-9a-zA-Z]{76}`),
		Keywords: []string{"ghr_"},
	},
	{
		ID:       "github-fine-grained-pat",
		Title:    "GitHub Fine-grained personal access tokens",
		Severity: "CRITICAL",
		Regex:    regexp.MustCompile(`github_pat_[a-zA-Z0-9]{22}_[a-zA-Z0-9]{59}`),
		Keywords: []string{"github_pat_"},
	},
	{
		ID:       "gitlab-pat",
		Title:    "GitLab Personal Access Token",
		Severity: "CRITICAL",
		Regex:    regexp.MustCompile(`glpat-[0-9a-zA-Z\-\_]{20}`),
		Keywords: []string{"glpat-"},
	},
	{
		// cf. https://huggingface.co/docs/hub/en/security-tokens
		ID:       "hugging-face-access-token",
		Severity: "CRITICAL",
		Title:    "Hugging Face Access Token",
		Regex:    regexp.MustCompile(`hf_[A-Za-z0-9]{39}`),
		Keywords: []string{"hf_"},
	},
	{
		ID:              "private-key",
		Title:           "Asymmetric Private Key",
		Severity:        "HIGH",
		Regex:           regexp.MustCompile(`(?i)-----\s*?BEGIN[ A-Z0-9_-]*?PRIVATE KEY( BLOCK)?\s*?-----[\s]*?(?P<secret>[\sA-Za-z0-9=+/\\\r\n]+)[\s]*?-----\s*?END[ A-Z0-9_-]*? PRIVATE KEY( BLOCK)?\s*?-----`),
		SecretGroupName: "secret",
		Keywords:        []string{"-----"},
	},
	{
		ID:       "shopify-token",
		Title:    "Shopify token",
		Severity: "HIGH",
		Regex:    regexp.MustCompile(`shp(ss|at|ca|pa)_[a-fA-F0-9]{32}`),
		Keywords: []string{"shpss_", "shpat_", "shpca_", "shppa_"},
	},
	{
		ID:       "slack-access-token",
		Title:    "Slack token",
		Severity: "HIGH",
		Regex:    regexp.MustCompile(`xox[baprs]-([0-9a-zA-Z]{10,48})`),
		Keywords: []string{"xoxb-", "xoxa-", "xoxp-", "xoxr-", "xoxs-"},
	},
	{
		ID:       "stripe-publishable-token",
		Title:    "Stripe Publishable Key",
		Severity: "LOW",
		Regex:    regexp.MustCompile(`(?i)pk_(test|live)_[0-9a-z]{10,32}`),
		Keywords: []string{"pk_test_", "pk_live_"},
	},
	{
		ID:       "stripe-secret-token",
		Title:    "Stripe Secret Key",
		Severity: "CRITICAL",
		Regex:    regexp.MustCompile(`(?i)sk_(test|live)_[0-9a-z]{10,32}`),
		Keywords: []string{"sk_test_", "sk_live_"},
	},
	{
		ID:       "pypi-upload-token",
		Title:    "PyPI upload token",
		Severity: "HIGH",
		Regex:    regexp.MustCompile(`pypi-AgEIcHlwaS5vcmc[A-Za-z0-9\-_]{50,1000}`),
		Keywords: []string{"pypi-AgEIcHlwaS5vcmc"},
	},
	{
		ID:       "gcp-service-account",
		Title:    "Google (GCP) Service-account",
		Severity: "CRITICAL",
		Regex:    regexp.MustCompile(`\"type\": \"service_account\"`),
		Keywords: []string{"\"type\": \"service_account\""},
	},
	{
		ID:              "heroku-api-key",
		Title:           "Heroku API Key",
		Severity:        "HIGH",
		Regex:           regexp.MustCompile(` (?i)(?P<key>heroku[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[0-9A-F]{8}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{12})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"heroku"},
	},
	{
		ID:       "slack-web-hook",
		Title:    "Slack Webhook",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`https:\/\/hooks.slack.com\/services\/[A-Za-z0-9+\/]{44,48}`),
		Keywords: []string{"hooks.slack.com"},
	},
	{
		ID:       "twilio-api-key",
		Title:    "Twilio API Key",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`SK[0-9a-fA-F]{32}`),
		Keywords: []string{"SK"},
	},
	{
		ID:       "age-secret-key",
		Title:    "Age secret key",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`AGE-SECRET-KEY-1[QPZRY9X8GF2TVDW0S3JN54KHCE6MUA7L]{58}`),
		Keywords: []string{"AGE-SECRET-KEY-1"},
	},
	{
		ID:              "facebook-token",
		Title:           "Facebook token",
		Severity:        "LOW",
		Regex:           regexp.MustCompile(`(?i)(?P<key>facebook[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-f0-9]{32})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"facebook"},
	},
	{
		ID:              "twitter-token",
		Title:           "Twitter token",
		Severity:        "LOW",
		Regex:           regexp.MustCompile(`(?i)(?P<key>twitter[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-f0-9]{35,44})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"twitter"},
	},
	{
		ID:              "adobe-client-id",
		Title:           "Adobe Client ID (Oauth Web)",
		Severity:        "LOW",
		Regex:           regexp.MustCompile(`(?i)(?P<key>adobe[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-f0-9]{32})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"adobe"},
	},
	{
		ID:       "adobe-client-secret",
		Title:    "Adobe Client Secret",
		Severity: "LOW",
		Regex:    regexp.MustCompile(`(p8e-)(?i)[a-z0-9]{32}`),
		Keywords: []string{"p8e-"},
	},
	{
		ID:              "alibaba-access-key-id",
		Title:           "Alibaba AccessKey ID",
		Severity:        "HIGH",
		Regex:           regexp.MustCompile(`([^0-9A-Za-z]|^)(?P<secret>(LTAI)(?i)[a-z0-9]{20})([^0-9A-Za-z]|$)`),
		SecretGroupName: "secret",
		Keywords:        []string{"LTAI"},
	},
	{
		ID:              "alibaba-secret-key",
		Title:           "Alibaba Secret Key",
		Severity:        "HIGH",
		Regex:           regexp.MustCompile(`(?i)(?P<key>alibaba[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-z0-9]{30})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"alibaba"},
	},
	{
		ID:              "asana-client-id",
		Title:           "Asana Client ID",
		Severity:        "MEDIUM",
		Regex:           regexp.MustCompile(`(?i)(?P<key>asana[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[0-9]{16})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"asana"},
	},
	{
		ID:              "asana-client-secret",
		Title:           "Asana Client Secret",
		Severity:        "MEDIUM",
		Regex:           regexp.MustCompile(`(?i)(?P<key>asana[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-z0-9]{32})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"asana"},
	},
	{
		ID:              "atlassian-api-token",
		Title:           "Atlassian API token",
		Severity:        "HIGH",
		Regex:           regexp.MustCompile(`(?i)(?P<key>atlassian[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-z0-9]{24})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"atlassian"},
	},
	{
		ID:              "bitbucket-client-id",
		Title:           "Bitbucket client ID",
		Severity:        "HIGH",
		Regex:           regexp.MustCompile(`(?i)(?P<key>bitbucket[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-z0-9]{32})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"bitbucket"},
	},
	{
		ID:              "bitbucket-client-secret",
		Title:           "Bitbucket client secret",
		Severity:        "HIGH",
		Regex:           regexp.MustCompile(`(?i)(?P<key>bitbucket[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-z0-9_\-]{64})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"bitbucket"},
	},
	{
		ID:              "beamer-api-token",
		Title:           "Beamer API token",
		Severity:        "LOW",
		Regex:           regexp.MustCompile(`(?i)(?P<key>beamer[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>b_[a-z0-9=_\-]{44})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"beamer"},
	},
	{
		ID:       "clojars-api-token",
		Title:    "Clojars API token",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`(CLOJARS_)(?i)[a-z0-9]{60}`),
		Keywords: []string{"CLOJARS_"},
	},
	{
		ID:              "contentful-delivery-api-token",
		Title:           "Contentful delivery API token",
		Severity:        "LOW",
		Regex:           regexp.MustCompile(`(?i)(?P<key>contentful[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-z0-9\-=_]{43})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"contentful"},
	},
	{
		ID:       "databricks-api-token",
		Title:    "Databricks API token",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`dapi[a-h0-9]{32}`),
		Keywords: []string{"dapi"},
	},
	{
		ID:              "discord-api-token",
		Title:           "Discord API key",
		Severity:        "MEDIUM",
		Regex:           regexp.MustCompile(`(?i)(?P<key>discord[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-h0-9]{64})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"discord"},
	},
	{
		ID:              "discord-client-id",
		Title:           "Discord client ID",
		Severity:        "MEDIUM",
		Regex:           regexp.MustCompile(`(?i)(?P<key>discord[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[0-9]{18})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"discord"},
	},
	{
		ID:              "discord-client-secret",
		Title:           "Discord client secret",
		Severity:        "MEDIUM",
		Regex:           regexp.MustCompile(`(?i)(?P<key>discord[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-z0-9=_\-]{32})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"discord"},
	},
	{
		ID:       "doppler-api-token",
		Title:    "Doppler API token",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`['\"](dp\.pt\.)(?i)[a-z0-9]{43}['\"]`),
		Keywords: []string{"dp.pt."},
	},
	{
		ID:       "dropbox-api-secret",
		Title:    "Dropbox API secret/key",
		Severity: "HIGH",
		Regex:    regexp.MustCompile(`(?i)(dropbox[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"]([a-z0-9]{15})['\"]`),
		Keywords: []string{"dropbox"},
	},
	{
		ID:       "dropbox-short-lived-api-token",
		Title:    "Dropbox short lived API token",
		Severity: "HIGH",
		Regex:    regexp.MustCompile(`(?i)(dropbox[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](sl\.[a-z0-9\-=_]{135})['\"]`),
		Keywords: []string{"dropbox"},
	},
	{
		ID:       "dropbox-long-lived-api-token",
		Title:    "Dropbox long lived API token",
		Severity: "HIGH",
		Regex:    regexp.MustCompile(`(?i)(dropbox[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"][a-z0-9]{11}(AAAAAAAAAA)[a-z0-9\-_=]{43}['\"]`),
		Keywords: []string{"dropbox"},
	},
	{
		ID:       "duffel-api-token",
		Title:    "Duffel API token",
		Severity: "LOW",
		Regex:    regexp.MustCompile(`['\"]duffel_(test|live)_(?i)[a-z0-9_-]{43}['\"]`),
		Keywords: []string{"duffel_test_", "duffel_live_"},
	},
	{
		ID:       "dynatrace-api-token",
		Title:    "Dynatrace API token",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`['\"]dt0c01\.(?i)[a-z0-9]{24}\.[a-z0-9]{64}['\"]`),
		Keywords: []string{"dt0c01."},
	},
	{
		ID:       "easypost-api-token",
		Title:    "EasyPost API token",
		Severity: "LOW",
		Regex:    regexp.MustCompile(`['\"]EZ[AT]K(?i)[a-z0-9]{54}['\"]`),
		Keywords: []string{"EZAK", "EZAT"},
	},
	{
		ID:              "fastly-api-token",
		Title:           "Fastly API token",
		Severity:        "MEDIUM",
		Regex:           regexp.MustCompile(`(?i)(?P<key>fastly[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-z0-9\-=_]{32})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"fastly"},
	},
	{
		ID:              "finicity-client-secret",
		Title:           "Finicity client secret",
		Severity:        "MEDIUM",
		Regex:           regexp.MustCompile(`(?i)(?P<key>finicity[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-z0-9]{20})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"finicity"},
	},
	{
		ID:              "finicity-api-token",
		Title:           "Finicity API token",
		Severity:        "MEDIUM",
		Regex:           regexp.MustCompile(`(?i)(?P<key>finicity[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-f0-9]{32})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"finicity"},
	},
	{
		ID:       "flutterwave-public-key",
		Title:    "Flutterwave public/secret key",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`FLW(PUB|SEC)K_TEST-(?i)[a-h0-9]{32}-X`),
		Keywords: []string{"FLWSECK_TEST-", "FLWPUBK_TEST-"},
	},
	{
		ID:       "flutterwave-enc-key",
		Title:    "Flutterwave encrypted key",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`FLWSECK_TEST[a-h0-9]{12}`),
		Keywords: []string{"FLWSECK_TEST"},
	},
	{
		ID:       "frameio-api-token",
		Title:    "Frame.io API token",
		Severity: "LOW",
		Regex:    regexp.MustCompile(`fio-u-(?i)[a-z0-9\-_=]{64}`),
		Keywords: []string{"fio-u-"},
	},
	{
		ID:       "gocardless-api-token",
		Title:    "GoCardless API token",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`['\"]live_(?i)[a-z0-9\-_=]{40}['\"]`),
		Keywords: []string{"live_"},
	},
	{
		ID:       "grafana-api-token",
		Title:    "Grafana API token",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`['\"]eyJrIjoi(?i)[a-z0-9\-_=]{72,92}['\"]`),
		Keywords: []string{"eyJrIjoi"},
	},
	{
		ID:       "hashicorp-tf-api-token",
		Title:    "HashiCorp Terraform user/org API token",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`['\"](?i)[a-z0-9]{14}\.atlasv1\.[a-z0-9\-_=]{60,70}['\"]`),
		Keywords: []string{"atlasv1."},
	},
	{
		ID:              "hubspot-api-token",
		Title:           "HubSpot API token",
		Severity:        "LOW",
		Regex:           regexp.MustCompile(`(?i)(?P<key>hubspot[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-h0-9]{8}-[a-h0-9]{4}-[a-h0-9]{4}-[a-h0-9]{4}-[a-h0-9]{12})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"hubspot"},
	},
	{
		ID:              "intercom-api-token",
		Title:           "Intercom API token",
		Severity:        "LOW",
		Regex:           regexp.MustCompile(`(?i)(?P<key>intercom[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-z0-9=_]{60})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"intercom"},
	},
	{
		ID:              "intercom-client-secret",
		Title:           "Intercom client secret/ID",
		Severity:        "LOW",
		Regex:           regexp.MustCompile(`(?i)(?P<key>intercom[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-h0-9]{8}-[a-h0-9]{4}-[a-h0-9]{4}-[a-h0-9]{4}-[a-h0-9]{12})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"intercom"},
	},
	{
		ID:       "ionic-api-token",
		Title:    "Ionic API token",
		Regex:    regexp.MustCompile(`(?i)(ionic[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](ion_[a-z0-9]{42})['\"]`),
		Keywords: []string{"ionic"},
	},
	{
		ID:       "jwt-token",
		Title:    "JWT token",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`ey[a-zA-Z0-9]{17,}\.ey[a-zA-Z0-9\/\\_-]{17,}\.(?:[a-zA-Z0-9\/\\_-]{10,}={0,2})?`),
		Keywords: []string{"jwt"},
	},
	{
		ID:       "linear-api-token",
		Title:    "Linear API token",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`lin_api_(?i)[a-z0-9]{40}`),
		Keywords: []string{"lin_api_"},
	},
	{
		ID:              "linear-client-secret",
		Title:           "Linear client secret/ID",
		Severity:        "MEDIUM",
		Regex:           regexp.MustCompile(`(?i)(?P<key>linear[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-f0-9]{32})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"linear"},
	},
	{
		ID:              "lob-api-key",
		Title:           "Lob API Key",
		Severity:        "LOW",
		Regex:           regexp.MustCompile(`(?i)(?P<key>lob[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>(live|test)_[a-f0-9]{35})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"lob"},
	},
	{
		ID:              "lob-pub-api-key",
		Title:           "Lob Publishable API Key",
		Severity:        "LOW",
		Regex:           regexp.MustCompile(`(?i)(?P<key>lob[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>(test|live)_pub_[a-f0-9]{31})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"lob"},
	},
	{
		ID:              "mailchimp-api-key",
		Title:           "Mailchimp API key",
		Severity:        "MEDIUM",
		Regex:           regexp.MustCompile(`(?i)(?P<key>mailchimp[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-f0-9]{32}-us20)['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"mailchimp"},
	},
	{
		ID:              "mailgun-token",
		Title:           "Mailgun private API token",
		Severity:        "MEDIUM",
		Regex:           regexp.MustCompile(`(?i)(?P<key>mailgun[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>(pub)?key-[a-f0-9]{32})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"mailgun"},
	},
	{
		ID:              "mailgun-signing-key",
		Title:           "Mailgun webhook signing key",
		Severity:        "MEDIUM",
		Regex:           regexp.MustCompile(`(?i)(?P<key>mailgun[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-h0-9]{32}-[a-h0-9]{8}-[a-h0-9]{8})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"mailgun"},
	},
	{
		ID:       "mapbox-api-token",
		Title:    "Mapbox API token",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`(?i)(pk\.[a-z0-9]{60}\.[a-z0-9]{22})`),
		Keywords: []string{"pk."},
	},
	{
		ID:              "messagebird-api-token",
		Title:           "MessageBird API token",
		Severity:        "MEDIUM",
		Regex:           regexp.MustCompile(`(?i)(?P<key>messagebird[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-z0-9]{25})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"messagebird"},
	},
	{
		ID:              "messagebird-client-id",
		Title:           "MessageBird API client ID",
		Severity:        "MEDIUM",
		Regex:           regexp.MustCompile(`(?i)(?P<key>messagebird[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-h0-9]{8}-[a-h0-9]{4}-[a-h0-9]{4}-[a-h0-9]{4}-[a-h0-9]{12})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"messagebird"},
	},
	{
		ID:       "new-relic-user-api-key",
		Title:    "New Relic user API Key",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`['\"](NRAK-[A-Z0-9]{27})['\"]`),
		Keywords: []string{"NRAK-"},
	},
	{
		ID:              "new-relic-user-api-id",
		Title:           "New Relic user API ID",
		Severity:        "MEDIUM",
		Regex:           regexp.MustCompile(`(?i)(?P<key>newrelic[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[A-Z0-9]{64})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"newrelic"},
	},
	{
		ID:       "new-relic-browser-api-token",
		Title:    "New Relic ingest browser API token",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`['\"](NRJS-[a-f0-9]{19})['\"]`),
		Keywords: []string{"NRJS-"},
	},
	{
		ID:       "npm-access-token",
		Title:    "npm access token",
		Severity: "CRITICAL",
		Regex:    regexp.MustCompile(`['\"](npm_(?i)[a-z0-9]{36})['\"]`),
		Keywords: []string{"npm_"},
	},
	{
		ID:       "planetscale-password",
		Title:    "PlanetScale password",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`pscale_pw_(?i)[a-z0-9\-_\.]{43}`),
		Keywords: []string{"pscale_pw_"},
	},
	{
		ID:       "planetscale-api-token",
		Title:    "PlanetScale API token",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`pscale_tkn_(?i)[a-z0-9\-_\.]{43}`),
		Keywords: []string{"pscale_tkn_"},
	},
	{
		ID:       "postman-api-token",
		Title:    "Postman API token",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`PMAK-(?i)[a-f0-9]{24}\-[a-f0-9]{34}`),
		Keywords: []string{"PMAK-"},
	},
	{
		ID:       "pulumi-api-token",
		Title:    "Pulumi API token",
		Severity: "HIGH",
		Regex:    regexp.MustCompile(`pul-[a-f0-9]{40}`),
		Keywords: []string{"pul-"},
	},
	{
		ID:       "rubygems-api-token",
		Title:    "Rubygem API token",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`rubygems_[a-f0-9]{48}`),
		Keywords: []string{"rubygems_"},
	},
	{
		ID:       "sendgrid-api-token",
		Title:    "SendGrid API token",
		Severity: "MEDIUM",
		Regex:    regexp.MustCompile(`SG\.(?i)[a-z0-9_\-\.]{66}`),
		Keywords: []string{"SG."},
	},
	{
		ID:       "sendinblue-api-token",
		Title:    "Sendinblue API token",
		Severity: "LOW",
		Regex:    regexp.MustCompile(`xkeysib-[a-f0-9]{64}\-(?i)[a-z0-9]{16}`),
		Keywords: []string{"xkeysib-"},
	},
	{
		ID:       "shippo-api-token",
		Title:    "Shippo API token",
		Severity: "LOW",
		Regex:    regexp.MustCompile(`shippo_(live|test)_[a-f0-9]{40}`),
		Keywords: []string{"shippo_live_", "shippo_test_"},
	},
	{
		ID:              "linkedin-client-secret",
		Title:           "LinkedIn Client secret",
		Severity:        "LOW",
		Regex:           regexp.MustCompile(`(?i)(?P<key>linkedin[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-z]{16})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"linkedin"},
	},
	{
		ID:              "linkedin-client-id",
		Title:           "LinkedIn Client ID",
		Severity:        "LOW",
		Regex:           regexp.MustCompile(`(?i)(?P<key>linkedin[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-z0-9]{14})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"linkedin"},
	},
	{
		ID:              "twitch-api-token",
		Title:           "Twitch API token",
		Severity:        "LOW",
		Regex:           regexp.MustCompile(`(?i)(?P<key>twitch[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['\"](?P<secret>[a-z0-9]{30})['\"]`),
		SecretGroupName: "secret",
		Keywords:        []string{"twitch"},
	},
	{
		ID:              "typeform-api-token",
		Title:           "Typeform API token",
		Severity:        "LOW",
		Regex:           regexp.MustCompile(`(?i)(?P<key>typeform[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}(?P<secret>tfp_[a-z0-9\-_\.=]{59})`),
		SecretGroupName: "secret",
		Keywords:        []string{"typeform"},
	},
	{
		ID:              "dockerconfig-secret",
		Title:           "Dockerconfig secret exposed",
		Severity:        "HIGH",
		Regex:           regexp.MustCompile(`(?i)(\.(dockerconfigjson|dockercfg):\s*\|*\s*(?P<secret>(ey|ew)+[A-Za-z0-9\/\+=]+))`),
		SecretGroupName: "secret",
		Keywords:        []string{"dockerc"},
	},
}

// MaskSecrets takes an input string and masks any secrets found based on the provided rules
func MaskSecretsOnString(input string, rules []Rule) string {
	maskedInput := input

	for _, rule := range rules {
		maskedInput = rule.Regex.ReplaceAllString(maskedInput, "******")

	}
	return maskedInput
}

func main() {

	// Create a new command to execute `cat` to read the file
	// cmd := exec.Command("cat", "/Users/prakhar/work/secret_masking_poc/log.log")
	//cmd := exec.Command("echo", "ASIAIQAP7NCOV4IOP6HQ")
	cmd := exec.Command("/bin/sh", "-c", "docker build -t masking -f /Users/prakhar/work/secret_masking_poc/Dockerfile  /Users/prakhar/work/secret_masking_poc --progress plain")

	// Create a buffer to capture the command's stdout
	//var outBuf bytes.Buffer
	//cmd.Stdout = &outBuf
	//cmd.Stderr = os.Stderr

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Command execution failed: %v\n", err)
	}

	// Run the command
	//if err := cmd.Run(); err != nil {
	//	fmt.Printf("Command execution failed: %v\n", err)
	//	os.Exit(1)
	//}

	// Read the file
	// file, err := os.Open("message.txt")
	// if err != nil {
	// 	fmt.Printf("Error opening file: %v\n", err)
	// 	os.Exit(1)
	// }
	// defer file.Close()
	// // timer to measure the time taken to mask the secrets
	start := time.Now()

	// // Create a buffer to capture the file's content
	// var outBuf bytes.Buffer
	// _, err = io.Copy(&outBuf, file)
	// if err != nil {
	// 	fmt.Printf("Error reading file: %v\n", err)
	// 	os.Exit(1)
	// }

	end_read := time.Now()

	outBuf := bytes.NewBuffer(output)

	buf := new(bytes.Buffer)
	maskedStream, err := MaskSecretsStream(outBuf)
	if err != nil {
		fmt.Printf("Error masking secrets: %v\n", err)
		os.Exit(1)
	}

	if _, err := io.Copy(buf, maskedStream); err != nil {
		fmt.Printf("Error reading masked stream: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(buf.String())
	// Call the function to mask secrets and print the masked output

	end_mask := time.Now()

	fmt.Println("Time taken to read the file: ", end_read.Sub(start))
	fmt.Println("Time taken to mask the secrets: ", end_mask.Sub(end_read))
}
func MaskSecretsStream(input *bytes.Buffer) (io.Reader, error) {
	pr, pw := io.Pipe()

	go func() {
		defer func() {
			pw.Close()
		}()
		scanner := bufio.NewScanner(input)
		const maxCapacity int = 256 * 1024 // 256KB
		buf := make([]byte, maxCapacity)
		scanner.Buffer(buf, maxCapacity)

		for scanner.Scan() {
			line := scanner.Text()
			if len(line) == 0 {
				_, err := pw.Write([]byte("\n"))
				if err != nil {
					// handle error appropriately
					return
				}
			} else {
				maskedString := MaskSecretsOnString(line, BuiltinRules)
				_, err := pw.Write([]byte(maskedString + "\n"))
				if err != nil {
					// handle error appropriately
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			if err == bufio.ErrTooLong {
				for {
					n, err := input.Read(buf)
					if err != nil {
						if err == io.EOF {
							break
						}
						// handle error appropriately
						return
					}
					line := string(buf[:n])
					maskedString := MaskSecretsOnString(line, BuiltinRules)
					_, err = pw.Write([]byte(maskedString + "\n"))
					if err != nil {
						// handle error appropriately
						return
					}
				}
			} else {
				// handle other errors appropriately
				return
			}
		}
	}()
	return pr, nil
}
