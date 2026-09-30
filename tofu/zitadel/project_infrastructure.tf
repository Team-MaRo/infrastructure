# The cluster's admin UIs - Argo CD, Grafana, OpenBao, the Traefik dashboard and Uptime Kuma -
# are applications of this one project, and its role is what they grant admin rights for.
resource "zitadel_project" "infrastructure" {
  org_id = local.org_id
  name   = "Infrastructure"

  # Roles go into the tokens, which is also what makes Zitadel hand the role grants to the
  # groups webhook (groups_claim.tf).
  project_role_assertion = true
  # Zitadel itself refuses a login to these UIs for anyone without a role here.
  project_role_check = true
}

# Role keys become the entries of the flat `groups` claim, which mixes every project's roles,
# so each key carries its project's prefix.
resource "zitadel_project_role" "infra_admin" {
  org_id       = local.org_id
  project_id   = zitadel_project.infrastructure.id
  role_key     = "infra-admin"
  display_name = "Infrastructure admin"
}

# The personal user; human users themselves are created in the console.
data "zitadel_human_users" "d3strukt0r" {
  org_id           = local.org_id
  user_name        = "D3strukt0r"
  user_name_method = "TEXT_QUERY_METHOD_EQUALS"
}

resource "zitadel_user_grant" "d3strukt0r_infrastructure" {
  org_id     = local.org_id
  project_id = zitadel_project.infrastructure.id
  user_id    = one(data.zitadel_human_users.d3strukt0r.user_ids)
  role_keys  = [zitadel_project_role.infra_admin.role_key]
}
