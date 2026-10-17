package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DependencyPolicy records reviewed exceptions, not inferred reachability. The
// native audit still runs in full. A devDependency alone is never an exception.
type DependencyPolicy struct {
	Inputs  map[string]string  `json:"inputs"`
	Reviews []DependencyReview `json:"reviews"`
}

type DependencyReview struct {
	Ecosystem       string `json:"ecosystem"`
	Package         string `json:"package"`
	Advisory        string `json:"advisory"`
	Scope           string `json:"scope"`
	ProductExposure string `json:"product_exposure"`
	ToolingRisk     string `json:"tooling_risk"`
	Reason          string `json:"reason"`
}

// LoadDependencyPolicy binds reviews to the current manifest and lockfile. A
// changed dependency graph must be reviewed again. Missing policy means no
// exceptions; malformed or stale policy is an execution error, never a waiver.
func LoadDependencyPolicy(root, ecosystem string) (*DependencyPolicy, error) {
	data, err := os.ReadFile(filepath.Join(root, ".oxguard-dependency-policy.json"))
	if os.IsNotExist(err) {
		return &DependencyPolicy{}, nil
	}
	if err != nil {
		return nil, err
	}
	var policy DependencyPolicy
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&policy); err != nil {
		return nil, err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("dependency policy must contain one JSON value")
	}
	manifests := map[string]string{"npm": "package.json", "pypi": "pyproject.toml"}
	locks := map[string][]string{"npm": {"package-lock.json", "pnpm-lock.yaml", "yarn.lock"}, "pypi": {"uv.lock"}}
	if _, ok := policy.Inputs[manifests[ecosystem]]; !ok {
		return nil, fmt.Errorf("dependency review requires %s hash", manifests[ecosystem])
	}
	locked := false
	for _, name := range locks[ecosystem] {
		_, bound := policy.Inputs[name]
		if bound {
			locked = true
		}
		if _, err := os.Lstat(filepath.Join(root, name)); err == nil && !bound {
			return nil, fmt.Errorf("dependency review requires every existing lockfile hash: %s", name)
		} else if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	if !locked {
		return nil, fmt.Errorf("dependency review requires a supported lockfile hash")
	}
	for name, want := range policy.Inputs {
		if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
			return nil, fmt.Errorf("unsafe dependency review input")
		}
		info, err := os.Lstat(filepath.Join(root, name))
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("dependency review input unavailable: %s", name)
		}
		contents, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(contents)
		if hex.EncodeToString(sum[:]) != want {
			return nil, fmt.Errorf("stale dependency review: %s changed", name)
		}
	}
	seen := map[string]bool{}
	for _, review := range policy.Reviews {
		key := review.Ecosystem + ":" + review.Package + ":" + review.Advisory
		if (review.Ecosystem != "npm" && review.Ecosystem != "pypi") || strings.TrimSpace(review.Package) == "" || strings.TrimSpace(review.Advisory) == "" || (review.Scope != "development_tool" && review.Scope != "ci") || review.ProductExposure != "not_exposed" || review.ToolingRisk != "accepted" || strings.TrimSpace(review.Reason) == "" || seen[key] {
			return nil, fmt.Errorf("invalid or duplicate dependency review: %s", key)
		}
		seen[key] = true
	}
	return &policy, nil
}

func (p *DependencyPolicy) Evidence(ecosystem, pkg, advisory string) string {
	for _, review := range p.Reviews {
		if review.Ecosystem == ecosystem && review.Package == pkg && review.Advisory == advisory {
			return fmt.Sprintf("Reviewed %s dependency; product exposure: %s; tooling risk: %s. %s", review.Scope, review.ProductExposure, review.ToolingRisk, review.Reason)
		}
	}
	return ""
}
