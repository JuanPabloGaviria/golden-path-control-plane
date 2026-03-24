package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/auth"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/config"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/domain"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "token":
		if err := runToken(os.Args[2:]); err != nil {
			fail(err)
		}
	case "register-service":
		if err := runRegisterService(os.Args[2:]); err != nil {
			fail(err)
		}
	case "scorecard":
		if err := runGet(os.Args[2:], "/v1/services/%s/scorecard", "service-id"); err != nil {
			fail(err)
		}
	case "queue-evaluation":
		if err := runPostEmpty(os.Args[2:], "/v1/services/%s/evaluations", "service-id"); err != nil {
			fail(err)
		}
	case "create-candidate":
		if err := runCreateCandidate(os.Args[2:]); err != nil {
			fail(err)
		}
	case "evaluate-candidate":
		if err := runPostEmpty(os.Args[2:], "/v1/deployment-candidates/%s/evaluate", "candidate-id"); err != nil {
			fail(err)
		}
	default:
		usage()
		os.Exit(1)
	}
}

func runToken(args []string) error {
	fs := flag.NewFlagSet("token", flag.ContinueOnError)
	subject := fs.String("subject", "developer@example.com", "token subject")
	role := fs.String("role", "engineer", "role: engineer|platform-admin")
	ttl := fs.Duration("ttl", time.Hour, "token lifetime")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.LoadTokenIssuerConfig()
	if err != nil {
		return err
	}

	token, err := auth.IssueHMACToken(cfg.Auth, *subject, domain.Role(*role), *ttl)
	if err != nil {
		return err
	}

	fmt.Println(token)
	return nil
}

func runRegisterService(args []string) error {
	fs := flag.NewFlagSet("register-service", flag.ContinueOnError)
	filePath := fs.String("file", "", "path to service JSON payload")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*filePath) == "" {
		return fmt.Errorf("register-service: --file is required")
	}

	body, err := os.ReadFile(*filePath)
	if err != nil {
		return fmt.Errorf("register-service: read file: %w", err)
	}

	return doRequest(http.MethodPost, "/v1/services", bytes.NewReader(body))
}

func runCreateCandidate(args []string) error {
	fs := flag.NewFlagSet("create-candidate", flag.ContinueOnError)
	serviceID := fs.String("service-id", "", "service UUID")
	environment := fs.String("environment", "production", "deployment environment")
	version := fs.String("version", "", "release version")
	commitSHA := fs.String("commit-sha", "", "commit sha")
	requestedBy := fs.String("requested-by", "developer@example.com", "requester email")
	if err := fs.Parse(args); err != nil {
		return err
	}

	id, err := uuid.Parse(*serviceID)
	if err != nil {
		return fmt.Errorf("create-candidate: invalid service-id: %w", err)
	}

	payload := domain.DeploymentCandidateInput{
		ServiceID:   id,
		Environment: *environment,
		Version:     *version,
		CommitSHA:   *commitSHA,
		RequestedBy: *requestedBy,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return doRequest(http.MethodPost, "/v1/deployment-candidates", bytes.NewReader(body))
}

func runGet(args []string, pathTemplate string, idFlag string) error {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	value := fs.String(idFlag, "", "resource UUID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *value == "" {
		return fmt.Errorf("%s: --%s is required", fs.Name(), idFlag)
	}

	return doRequest(http.MethodGet, fmt.Sprintf(pathTemplate, *value), nil)
}

func runPostEmpty(args []string, pathTemplate string, idFlag string) error {
	fs := flag.NewFlagSet("post-empty", flag.ContinueOnError)
	value := fs.String(idFlag, "", "resource UUID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *value == "" {
		return fmt.Errorf("%s: --%s is required", fs.Name(), idFlag)
	}

	return doRequest(http.MethodPost, fmt.Sprintf(pathTemplate, *value), bytes.NewReader([]byte("{}")))
}

func doRequest(method, path string, body io.Reader) error {
	baseURL := strings.TrimSuffix(os.Getenv("CONTROL_PLANE_API_URL"), "/")
	token := strings.TrimSpace(os.Getenv("CONTROL_PLANE_TOKEN"))
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	if token == "" {
		return fmt.Errorf("CONTROL_PLANE_TOKEN must be set")
	}

	request, err := http.NewRequest(method, baseURL+path, body)
	if err != nil {
		return err
	}

	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer func() {
		_ = response.Body.Close()
	}()

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	fmt.Println(string(payload))
	if response.StatusCode >= 400 {
		return fmt.Errorf("cli: request failed with status %d", response.StatusCode)
	}

	return nil
}

func usage() {
	fmt.Println(`usage:
  cli token --subject developer@example.com --role engineer
  cli register-service --file ./service.json
  cli scorecard --service-id <uuid>
  cli queue-evaluation --service-id <uuid>
  cli create-candidate --service-id <uuid> --environment production --version v1.0.0 --commit-sha abc123 --requested-by developer@example.com
  cli evaluate-candidate --candidate-id <uuid>`)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(1)
}
