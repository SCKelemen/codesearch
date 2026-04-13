// Package vuln scans code and dependencies for known vulnerabilities. It
// composes advisory data with lockfile parsing and code search to produce
// actionable findings. The scanner supports three tiers: fast dependency
// matching, medium pattern-based code search, and slow semantic analysis.
package vuln
