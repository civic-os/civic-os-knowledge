// Package auth authenticates requests to the knowledge server: OAuth 2.1
// Bearer tokens on the MCP endpoint, validated against Keycloak's JWKS, and an
// OIDC authorization-code flow with role checks for the viz.html viewer.
package auth
