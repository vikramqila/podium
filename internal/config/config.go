// Package config loads and validates GatewayKit YAML configuration.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type File struct {
	Gateway Gateway `yaml:"gateway"`
}

type Gateway struct {
	Port            int        `yaml:"port"`
	GlobalTimeout   string     `yaml:"global_timeout"`
	GlobalRateLimit *RateLimit `yaml:"global_rate_limit"`
	Routes          []Route    `yaml:"routes"`
}

type Route struct {
	Path              string             `yaml:"path"`
	Methods           []string           `yaml:"methods"`
	StripPrefix       bool               `yaml:"strip_prefix"`
	Upstream          Upstream           `yaml:"upstream"`
	Timeout           string             `yaml:"timeout"`
	Retry             *Retry             `yaml:"retry"`
	RateLimit         *RateLimit         `yaml:"rate_limit"`
	HealthCheck       *HealthCheck       `yaml:"health_check"`
	RequestTransform  *RequestTransform  `yaml:"request_transform"`
	ResponseTransform *ResponseTransform `yaml:"response_transform"`
	Auth              *Auth              `yaml:"auth"`
	CircuitBreaker    *CircuitBreaker    `yaml:"circuit_breaker"`
}

type Upstream struct {
	URL     string   `yaml:"url"`
	Targets []Target `yaml:"targets"`
	Balance string   `yaml:"balance"`
}

type Target struct {
	URL    string `yaml:"url"`
	Weight int    `yaml:"weight"`
}

type RateLimit struct {
	Requests int    `yaml:"requests"`
	Window   string `yaml:"window"`
	Strategy string `yaml:"strategy"`
	Per      string `yaml:"per"`
}

type Retry struct {
	Attempts     int    `yaml:"attempts"`
	Backoff      string `yaml:"backoff"`
	InitialDelay string `yaml:"initial_delay"`
	On           []int  `yaml:"on"`
}

type HealthCheck struct {
	Path               string `yaml:"path"`
	Interval           string `yaml:"interval"`
	UnhealthyThreshold int    `yaml:"unhealthy_threshold"`
}

type HeaderTransform struct {
	Add    map[string]string `yaml:"add"`
	Remove []string          `yaml:"remove"`
}

type BodyMapping struct {
	Mapping map[string]string `yaml:"mapping"`
}

type RequestTransform struct {
	Headers *HeaderTransform `yaml:"headers"`
	Body    *BodyMapping     `yaml:"body"`
}

type BodyEnvelope struct {
	Envelope map[string]any `yaml:"envelope"`
}

type ResponseTransform struct {
	Headers *HeaderTransform `yaml:"headers"`
	Body    *BodyEnvelope    `yaml:"body"`
}

type Auth struct {
	Type   string   `yaml:"type"`
	Header string   `yaml:"header"`
	Keys   []string `yaml:"keys"`
}

type CircuitBreaker struct {
	Threshold int    `yaml:"threshold"`
	Window    string `yaml:"window"`
	Cooldown  string `yaml:"cooldown"`
}

func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg File
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (f File) Validate() error {
	var problems []string

	if f.Gateway.Port <= 0 || f.Gateway.Port > 65535 {
		problems = append(problems, "gateway.port must be between 1 and 65535")
	}
	if f.Gateway.GlobalTimeout != "" {
		validateDuration(&problems, "gateway.global_timeout", f.Gateway.GlobalTimeout)
	}
	if f.Gateway.GlobalRateLimit != nil {
		validateRateLimit(&problems, "gateway.global_rate_limit", *f.Gateway.GlobalRateLimit)
	}
	if len(f.Gateway.Routes) == 0 {
		problems = append(problems, "gateway.routes must contain at least one route")
	}

	seenPaths := map[string]struct{}{}
	for i, route := range f.Gateway.Routes {
		prefix := fmt.Sprintf("gateway.routes[%d]", i)
		if route.Path == "" || !strings.HasPrefix(route.Path, "/") {
			problems = append(problems, prefix+".path must start with /")
		}
		if _, ok := seenPaths[route.Path]; route.Path != "" && ok {
			problems = append(problems, prefix+".path must be unique")
		}
		seenPaths[route.Path] = struct{}{}

		if len(route.Methods) == 0 {
			problems = append(problems, prefix+".methods must contain at least one method")
		}
		for _, method := range route.Methods {
			if strings.TrimSpace(method) == "" || strings.ToUpper(method) != method {
				problems = append(problems, prefix+".methods must contain uppercase HTTP methods")
				break
			}
		}

		validateUpstream(&problems, prefix+".upstream", route.Upstream)
		if route.Timeout != "" {
			validateDuration(&problems, prefix+".timeout", route.Timeout)
		}
		if route.Retry != nil {
			validateRetry(&problems, prefix+".retry", *route.Retry)
		}
		if route.RateLimit != nil {
			validateRateLimit(&problems, prefix+".rate_limit", *route.RateLimit)
		}
		if route.HealthCheck != nil {
			validateHealthCheck(&problems, prefix+".health_check", *route.HealthCheck)
		}
		if route.Auth != nil {
			validateAuth(&problems, prefix+".auth", *route.Auth)
		}
		if route.CircuitBreaker != nil {
			validateCircuitBreaker(&problems, prefix+".circuit_breaker", *route.CircuitBreaker)
		}
	}

	if len(problems) > 0 {
		return errors.New("invalid config: " + strings.Join(problems, "; "))
	}
	return nil
}

func validateUpstream(problems *[]string, field string, upstream Upstream) {
	hasURL := upstream.URL != ""
	hasTargets := len(upstream.Targets) > 0
	switch {
	case hasURL && hasTargets:
		*problems = append(*problems, field+" must use either url or targets, not both")
	case !hasURL && !hasTargets:
		*problems = append(*problems, field+" must define url or targets")
	case hasURL:
		validateURL(problems, field+".url", upstream.URL)
	case hasTargets:
		for i, target := range upstream.Targets {
			targetField := fmt.Sprintf("%s.targets[%d]", field, i)
			validateURL(problems, targetField+".url", target.URL)
			if target.Weight <= 0 {
				*problems = append(*problems, targetField+".weight must be greater than 0")
			}
		}
		if upstream.Balance != "" && upstream.Balance != "round_robin" && upstream.Balance != "weighted_round_robin" {
			*problems = append(*problems, field+".balance must be round_robin or weighted_round_robin")
		}
	}
}

func validateRateLimit(problems *[]string, field string, rateLimit RateLimit) {
	if rateLimit.Requests <= 0 {
		*problems = append(*problems, field+".requests must be greater than 0")
	}
	validateDuration(problems, field+".window", rateLimit.Window)
	if rateLimit.Strategy != "fixed_window" && rateLimit.Strategy != "sliding_window" {
		*problems = append(*problems, field+".strategy must be fixed_window or sliding_window")
	}
	if rateLimit.Per != "ip" && rateLimit.Per != "global" {
		*problems = append(*problems, field+".per must be ip or global")
	}
}

func validateRetry(problems *[]string, field string, retry Retry) {
	if retry.Attempts <= 0 {
		*problems = append(*problems, field+".attempts must be greater than 0")
	}
	if retry.Backoff != "fixed" && retry.Backoff != "exponential" {
		*problems = append(*problems, field+".backoff must be fixed or exponential")
	}
	validateDuration(problems, field+".initial_delay", retry.InitialDelay)
	for _, statusCode := range retry.On {
		if statusCode < 100 || statusCode > 599 {
			*problems = append(*problems, field+".on must contain valid HTTP status codes")
			break
		}
	}
}

func validateHealthCheck(problems *[]string, field string, healthCheck HealthCheck) {
	if healthCheck.Path == "" || !strings.HasPrefix(healthCheck.Path, "/") {
		*problems = append(*problems, field+".path must start with /")
	}
	validateDuration(problems, field+".interval", healthCheck.Interval)
	if healthCheck.UnhealthyThreshold <= 0 {
		*problems = append(*problems, field+".unhealthy_threshold must be greater than 0")
	}
}

func validateAuth(problems *[]string, field string, auth Auth) {
	if auth.Type != "api_key" {
		*problems = append(*problems, field+".type must be api_key")
	}
	if auth.Header == "" {
		*problems = append(*problems, field+".header is required")
	}
	if len(auth.Keys) == 0 {
		*problems = append(*problems, field+".keys must contain at least one key")
	}
}

func validateCircuitBreaker(problems *[]string, field string, circuitBreaker CircuitBreaker) {
	if circuitBreaker.Threshold <= 0 {
		*problems = append(*problems, field+".threshold must be greater than 0")
	}
	validateDuration(problems, field+".window", circuitBreaker.Window)
	validateDuration(problems, field+".cooldown", circuitBreaker.Cooldown)
}

func validateDuration(problems *[]string, field string, value string) {
	if value == "" {
		*problems = append(*problems, field+" is required")
		return
	}
	if _, err := time.ParseDuration(value); err != nil {
		*problems = append(*problems, field+" must be a valid duration")
	}
}

func validateURL(problems *[]string, field string, value string) {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		*problems = append(*problems, field+" must be an absolute URL")
	}
}
