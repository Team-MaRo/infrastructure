# pgAdmin's settings, read after its defaults and the container's own config (it loads
# /etc/pgadmin/config_system.py last). Only what differs from pgAdmin's defaults.

# Logging in only through Zitadel; pgAdmin creates the account on the first login.
AUTHENTICATION_SOURCES = ['oauth2']

# A public client (tofu/zitadel/apps_pgadmin.tf): no secret, authorization code with PKCE.
OAUTH2_CONFIG = [
    {
        'OAUTH2_NAME': 'zitadel',
        'OAUTH2_DISPLAY_NAME': 'Zitadel',
        'OAUTH2_CLIENT_ID': '393358386470584673',
        'OAUTH2_SERVER_METADATA_URL': 'https://auth.d3strukt0r.dev/.well-known/openid-configuration',
        'OAUTH2_AUTHORIZATION_URL': 'https://auth.d3strukt0r.dev/oauth/v2/authorize',
        'OAUTH2_TOKEN_URL': 'https://auth.d3strukt0r.dev/oauth/v2/token',
        'OAUTH2_API_BASE_URL': 'https://auth.d3strukt0r.dev/oidc/v1/',
        'OAUTH2_USERINFO_ENDPOINT': 'userinfo',
        'OAUTH2_SCOPE': 'openid email profile',
        'OAUTH2_CHALLENGE_METHOD': 'S256',
        'OAUTH2_RESPONSE_TYPE': 'code',
        # Only members of infra-admin, from the groups webhook's claim.
        'OAUTH2_ADDITIONAL_CLAIMS': {'groups': ['infra-admin']},
        # Ends the Zitadel session too, not only pgAdmin's.
        'OAUTH2_LOGOUT_URL': 'https://auth.d3strukt0r.dev/oidc/v1/end_session?client_id=393358386470584673&post_logout_redirect_uri={redirect_uri}',
    }
]

# No saved database passwords: PostgreSQL keeps the roles' passwords and 1Password a copy, so
# pgAdmin asks on connecting and holds nothing. Without saved passwords there is nothing for a
# master password to protect, and pgAdmin stops asking for one.
ALLOW_SAVE_PASSWORD = False
MASTER_PASSWORD_REQUIRED = False

# The cookie only over HTTPS; Traefik ends TLS in front of pgAdmin.
SESSION_COOKIE_SECURE = True

# Off: it ties a session to the client's IP address, and behind Cloudflare's proxy pgAdmin sees
# a Cloudflare address that can change between two requests (Traefik does not trust Cloudflare's
# forwarded headers), which would log users out at random.
ENHANCED_COOKIE_PROTECTION = False
