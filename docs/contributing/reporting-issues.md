# Reporting Issues

We use GitHub Issues to track bugs, feature requests, and improvements. Your detailed reports help us improve the project!

## Before Creating an Issue

1.   **Search existing issues:** Check [open issues](https://github.com/OpenNSW/nsw-agency/issues) and [closed issues](https://github.com/OpenNSW/nsw-agency/issues?q=is%3Aissue+is%3Aclosed) to see if your issue has already been reported
2.   **Check documentation:** Review the README and relevant package documentation to see if your question is already answered
3.   **Verify it's a bug:** For behavioral questions, ensure it's actually a bug and not expected behavior

## Reporting a Bug

If you've found a bug, please create a new issue using the bug report template and include:

### Required Information

1.  **Clear and descriptive title**
    - Use a concise summary (e.g., "Agency backend fails to verify expected OU handle for JWT")
    - Avoid vague titles like "Bug" or "Problem"

2.  **Steps to reproduce**
    - Provide step-by-step instructions
    - Include code snippets, config JSON, or request payloads if applicable
    - Be specific about inputs and actions

3.  **Expected behavior**
    - Describe what should happen

4.  **Actual behavior**
    - Describe what actually happens
    - Include error messages or console logs if available

5.  **Environment details**
    - OS and version (e.g., macOS 14.0, Ubuntu 22.04)
    - Go version: `go version`
    - Node / package versions (if applicable)
    - Browser name and version (if frontend related)
    - Commit hashes or tags

### Additional Information

-   **Screenshots/Recordings:** If applicable, include screenshots or screen recordings
-   **Error logs:** Include relevant log output or stack traces from the backend or browser console
-   **Minimal reproduction:** If possible, provide a minimal configuration or dataset that reproduces the issue
-   **Related issues:** Link to related issues or pull requests

### Example Bug Report

```markdown
**Title:** Agency backend fails to verify expected OU for JWT

**Steps to reproduce:**
1. Login to the NPQS Agency Portal.
2. Intercept a request and verify auth headers.
3. Observe that access is granted even if the `ouHandle` claim doesn't match `npqs`.

**Expected behavior:**
Access should be forbidden with a 403 status code if `ouHandle` does not match `AUTH_EXPECTED_OU`.

**Actual behavior:**
Access is permitted, returning a 200 OK status code.

**Environment:**
- OS: macOS 14.0
- Go: 1.25.2
- Node: 22.0.0
- Browser: Chrome 124.0
- Service: nsw-agency-backend v1.1.0
```

## Requesting a Feature

Have an idea for a new feature or improvement? We'd love to hear it! Use the feature request template and include:

### Required Information

1.  **Clear and descriptive title**
    - Summarize the feature (e.g., "Add branding customization options for agency logo height")

2.  **Detailed description**
    - Explain what the feature should do
    - Describe the user experience, config schema additions, or API design

3.  **Problem it solves**
    - What problem does this feature address?
    - What use case does it enable?

4.  **Proposed solution**
    - How should this feature work?
    - Include API design, UI mockups, config JSON structure, or examples if applicable

### Additional Information

-   **Alternatives considered:** What other approaches did you consider?
-   **Impact:** Who would benefit from this feature?
-   **Implementation notes:** Any technical considerations or constraints?

### Example Feature Request

```markdown
**Title:** Add branding customization options for agency logo height

**Description:**
Currently, the frontend only reads custom images from branding configs, but doesn't allow styling/sizing parameters.
Some agencies have wider logos that look squished.

**Proposed Solution:**
Add a `logoHeight` option to the branding config JSON schemas and apply it in the Header components.

**Use Case:**
Allows tailoring the portal's branding to match each agency's unique logo proportions.
```

## Issue Labels

We use labels to categorize issues:

-   `bug` - Something isn't working
-   `enhancement` - New feature or improvement
-   `documentation` - Documentation improvements
-   `good first issue` - Good for newcomers
-   `help wanted` - Extra attention needed
-   `question` - Further information is requested

## After Submitting

-   **Be responsive:** Respond to questions or requests for clarification
-   **Provide updates:** If you find more information, add it to the issue
-   **Close if resolved:** If you resolved the issue yourself, let us know and close it
-   **Be patient:** We'll review your issue as soon as possible

## Security Issues

**Do not** report security vulnerabilities through public GitHub issues. Instead, please contact the maintainers directly or use GitHub's [private vulnerability reporting](https://github.com/OpenNSW/nsw-agency/security/advisories/new) feature.

[Open a new issue](https://github.com/OpenNSW/nsw-agency/issues/new)
