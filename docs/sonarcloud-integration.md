# SonarCloud Integration

This document describes the SonarCloud integration setup for the integration-service project.

## Overview

SonarCloud provides comprehensive code quality analysis including:

- **Code Coverage**: Test coverage analysis with minimum 70% coverage requirement
- **Code Quality**: Detection of bugs, vulnerabilities, and code smells
- **Security**: Security hotspots and vulnerability scanning
- **Maintainability**: Technical debt analysis and maintainability ratings
- **Reliability**: Bug detection and reliability ratings
- **Duplication**: Code duplication analysis

## Configuration Files

### `.sonarcloud.properties`

The main configuration file that defines:
- Source paths and exclusions
- Coverage settings and exclusions
- Quality gate thresholds
- Security analysis settings
- Project metadata

### `.github/workflows/sonarcloud.yml`

GitHub Actions workflow that:
- Runs on push to main/develop branches
- Runs on pull requests to main/develop branches
- Generates test coverage reports
- Performs SonarCloud analysis
- Comments PRs with analysis results

## Metrics Available

### 1. Code Coverage
- **Coverage Percentage**: Overall code coverage percentage
- **Line Coverage**: Line-by-line coverage analysis
- **Branch Coverage**: Branch coverage analysis
- **Coverage by File**: Coverage breakdown by file
- **Uncovered Lines**: Identification of uncovered code

### 2. Code Quality
- **Bugs**: Potential runtime errors
- **Vulnerabilities**: Security vulnerabilities
- **Code Smells**: Maintainability issues
- **Technical Debt**: Estimated effort to fix issues
- **Reliability Rating**: A-E rating based on bugs
- **Security Rating**: A-E rating based on vulnerabilities
- **Maintainability Rating**: A-E rating based on code smells

### 3. Security Analysis
- **Security Hotspots**: Security-sensitive code areas
- **Vulnerability Detection**: Known security vulnerabilities
- **Dependency Analysis**: Security analysis of dependencies
- **OWASP Top 10**: Coverage of OWASP security risks

### 4. Duplication Analysis
- **Duplicate Code**: Detection of code duplication
- **Duplication Percentage**: Percentage of duplicated code
- **Duplicated Files**: Files with duplicated code

### 5. Complexity Metrics
- **Cyclomatic Complexity**: Code complexity analysis
- **Cognitive Complexity**: Cognitive load analysis
- **Function Complexity**: Function-level complexity

## Setup Requirements

### 1. SonarCloud Account
- Create a SonarCloud account at https://sonarcloud.io
- Create a new project for `konflux-ci-samples/integration-service`

### 2. GitHub Secrets
Add the following secrets to your GitHub repository:
- `SONAR_TOKEN`: Your SonarCloud authentication token

### 3. Quality Gate Configuration
The quality gate is configured to:
- Require minimum 70% code coverage
- Fail on missing coverage
- Wait for quality gate results

## Running Analysis

### Manual Analysis
```bash
# Run tests with coverage
make test

# Run SonarCloud analysis locally (requires SonarCloud CLI)
sonar-scanner
```

### Automated Analysis
The GitHub Actions workflow automatically runs:
1. On every push to main/develop branches
2. On every pull request to main/develop branches

## Coverage Configuration

### Coverage Exclusions
The following are excluded from coverage analysis:
- Test files (`**/*test*`, `**/test/**`)
- Vendor dependencies (`**/vendor/**`)
- Generated files (`**/zz_generated.*`)
- Main entry point (`**/cmd/main.go`)

### Coverage Requirements
- Minimum coverage: 70%
- Fail on missing coverage: true
- Coverage report path: `coverage.out`

## Security Analysis

### Security Hotspots
- Audit mode enabled for security hotspots
- Manual review required for security issues

### Vulnerability Scanning
- Go module analysis (`go.mod`)
- Dependency vulnerability scanning
- Known vulnerability database integration

## Quality Gate

The quality gate ensures:
- Code coverage meets minimum threshold (70%)
- No critical bugs or vulnerabilities
- Maintainability standards are met
- Security standards are maintained

## Integration with Existing Workflows

The SonarCloud analysis integrates with:
- Existing test workflows
- Codecov coverage reporting
- GitHub PR checks
- CI/CD pipelines

## Troubleshooting

### Common Issues

1. **Coverage Not Detected**
   - Ensure `coverage.out` file is generated
   - Check coverage exclusions in `.sonarcloud.properties`

2. **Quality Gate Failing**
   - Review SonarCloud dashboard for specific issues
   - Address code quality issues
   - Improve test coverage

3. **Authentication Issues**
   - Verify `SONAR_TOKEN` secret is correctly set
   - Check token permissions in SonarCloud

### Debugging

1. **Local Analysis**
   ```bash
   # Install SonarCloud CLI
   # Run analysis locally
   sonar-scanner -Dsonar.host.url=https://sonarcloud.io
   ```

2. **Check Logs**
   - Review GitHub Actions logs
   - Check SonarCloud project dashboard

## Best Practices

1. **Regular Reviews**
   - Review SonarCloud dashboard regularly
   - Address quality issues promptly
   - Maintain high coverage standards

2. **Team Integration**
   - Share SonarCloud dashboard with team
   - Use quality gate as part of PR process
   - Regular code quality reviews

3. **Continuous Improvement**
   - Monitor trends in code quality
   - Set improvement goals
   - Regular quality gate adjustments

## Metrics Dashboard

Access your project metrics at:
`https://sonarcloud.io/organizations/konflux-ci-samples/projects`

The dashboard provides:
- Real-time quality metrics
- Historical trends
- Detailed issue analysis
- Coverage reports
- Security analysis 